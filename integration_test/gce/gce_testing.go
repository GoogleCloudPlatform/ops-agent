// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build integration_test

/*
Package gce holds various helpers for testing the agents on GCE.
To run a test based on this library, you can either:

* use Kokoro by triggering automated presubmits on your change, or
* use "go test" directly, after performing the setup steps described
in README.md.

NOTE: When testing Windows VMs without using Kokoro, PROJECT needs to be
a project whose firewall allows ssh connections.
[Kokoro can use stackdriver-test-143416, which does not allow ssh
connections, because our Kokoro workers are also running in that project.]

NOTE: This command does not actually build the Ops Agent. To test the latest

Ops Agent code, first build and upload a package to Rapture. Then look up
the REPO_SUFFIX for that build and add it as an environment variable to the
command below; for example: REPO_SUFFIX=20210805-2. You can also use
AGENT_PACKAGES_IN_GCS, for details see README.md.

	PROJECT=dev_project \
	ZONES=us-central1-b \
	PLATFORMS=debian-10,centos-8,rhel-8-2-sap-ha,sles-15,ubuntu-2004-lts,windows-2016,windows-2019 \
	go test -v ops_agent_test.go \
	  -test.parallel=1000 \
	  -tags=integration_test \
	  -timeout=4h

This library needs the following environment variables to be defined:
PROJECT: What GCP project to use.
ZONES: What GCP zones to run in as a comma-separated list, with optional
integer weights attached to each zone, in the format:
zone1=weight1,zone2=weight2. Any zone with no weight is given a default weight
of 1.

The following variables are optional:

TEST_UNDECLARED_OUTPUTS_DIR: A path to a directory to write log files into.
By default, a new temporary directory is created.

NETWORK_NAME: What GCP network name to use.
KOKORO_BUILD_ID: supplied by Kokoro.
KOKORO_BUILD_ARTIFACTS_SUBDIR: supplied by Kokoro.
LOG_UPLOAD_URL_ROOT: A URL prefix (remember the trailing "/") where the test
logs will be uploaded. If unset, this will point to
ops-agents-public-buckets-test-logs, which should work for all tests
triggered from GitHub.

USE_INTERNAL_IP: If set to "true", pass --no-address to gcloud when creating
VMs. This will not create an external IP address for that VM (because those are
expensive), and instead the VM will use cloud NAT to get to the external
internet. ssh-ing to the VM is done via its internal IP address.
Only useful on Kokoro.

SERVICE_EMAIL: If provided, which service account to use for spawned VMs. The
default is the project's "Compute Engine default service account".
TRANSFERS_BUCKET: A GCS bucket name to use to transfer files to testing VMs.
The default is "stackdriver-test-143416-file-transfers".
INSTANCE_SIZE: What size of VMs to make. Passed in to gcloud as --machine-type.
If provided, this value overrides the selection made by the callers to
this library.

DISABLE_PREPARE_SLES: Hopefully temporary option to disable workarounds for
flaky startup of SLES VMs. Workarounds are disabled by setting this to the
string "true".
*/
package gce

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/integration_test/logging"

	cloudlogging "cloud.google.com/go/logging"
	"cloud.google.com/go/logging/logadmin"
	monitoring "cloud.google.com/go/monitoring/apiv3"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"cloud.google.com/go/storage"
	trace "cloud.google.com/go/trace/apiv1"
	cloudtrace "cloud.google.com/go/trace/apiv1/tracepb"
	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"go.uber.org/multierr"
	"golang.org/x/text/encoding/unicode"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

var (
	storageClient   *storage.Client
	transfersBucket string

	monClient   *monitoring.MetricClient
	logClients  *logClientFactory
	traceClient *trace.Client

	zonePicker *weightedRoundRobin

	// These are paths to files on the local disk that hold the keys needed to
	// ssh to Linux VMs. init() will generate fresh keys for each run. Tests
	// that use this library are advised to call CleanupKeys() at the end of
	// testing so that the keys don't accumulate on disk.
	privateKeyFile string
	publicKeyFile  string
	// The path to the temporary directory that holds privateKeyFile and
	// publicKeyFile.
	keysDir string

	// A prefix to give to all VM names.
	sandboxPrefix string

	// Local filesystem path to a directory to put log files into.
	logRootDir string

	ErrInvalidIteratorLength = errors.New("iterator length is less than the defined minimum length")
)

const (
	// SuggestedTimeout is a recommended limit on how long a test should run before it is cancelled.
	// This cancellation does not happen automatically; each TestFoo() function must explicitly call
	// context.WithTimeout() to enable a timeout. It's a good idea to do this so that if a command
	// hangs, the test still will be cancelled eventually and all its VMs will be cleaned up.
	// This amount needs to be less than 4 hours, which is the limit on how long a Kokoro build can
	// take before it is forcibly killed.
	SuggestedTimeout = 2 * time.Hour

	// QueryMaxAttempts is the default number of retries when calling WaitForLog and WaitForMetricSeries.
	// Retries are spaced by 5 seconds, so 80 retries denotes 6 minutes 40 seconds total.
	QueryMaxAttempts              = 80 // 6 minutes 40 seconds total.
	queryMaxAttemptsMetricMissing = 5  // 25 seconds total.
	queryMaxAttemptsLogMissing    = 5  // 25 seconds total.
	queryBackoffDuration          = 5 * time.Second

	// traceQueryDerate is the number of backoff durations to wait before retrying a trace query.
	// Cloud Trace quota is incredibly low, and each call to ListTraces uses 25 quota tokens.
	traceQueryDerate = 6 // = 30 seconds with above settings

	vmInitTimeout                     = 20 * time.Minute
	vmInitBackoffDuration             = 10 * time.Second
	vmInitPokeSSHTimeout              = 30 * time.Second
	vmWinPasswordResetBackoffDuration = 30 * time.Second

	slesStartupDelay           = 60 * time.Second
	slesStartupSudoDelay       = 5 * time.Second
	slesStartupSudoMaxAttempts = 60

	sshUserName = "test_user"

	exhaustedRetriesSuffix = "exhausted retries"

	DenyEgressTrafficTag = "test-ops-agent-deny-egress-traffic-tag"

	TraceQueryMaxAttempts = QueryMaxAttempts / traceQueryDerate
)

func init() {
	ctx := context.Background()
	var err error
	storageClient, err = storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("storage.NewClient() failed: %v:", err)
	}
	transfersBucket = os.Getenv("TRANSFERS_BUCKET")
	if transfersBucket == "" {
		transfersBucket = "stackdriver-test-143416-file-transfers"
	}
	monClient, err = monitoring.NewMetricClient(ctx)
	if err != nil {
		log.Fatalf("NewMetricClient() failed: %v", err)
	}
	logClients = &logClientFactory{
		clients: make(map[string]*logadmin.Client),
	}
	traceClient, err = trace.NewClient(ctx)
	if err != nil {
		log.Fatalf("trace.NewClient() failed: %v", err)
	}

	zonePicker, err = newZonePicker(os.Getenv("ZONES"))
	if err != nil {
		log.Fatal(err)
	}

	// Some useful options to pass to gcloud.
	os.Setenv("CLOUDSDK_PYTHON", "/usr/bin/python3")
	os.Setenv("CLOUDSDK_CORE_DISABLE_PROMPTS", "1")

	keysDir, err = os.MkdirTemp("", "ssh_keys")
	if err != nil {
		log.Fatalf("init() failed to make a temporary directory for ssh keys: %v", err)
	}
	privateKeyFile = filepath.Join(keysDir, "gce_testing_key")
	if _, err := runCommand(ctx, log.Default(), nil, []string{"ssh-keygen", "-t", "rsa", "-f", privateKeyFile, "-C", sshUserName, "-N", ""}); err != nil {
		log.Fatalf("init() failed to generate new public+private key pair: %v", err)
	}
	publicKeyFile = privateKeyFile + ".pub"

	// Prefixes VM names with today's date in YYYYMMDD format, and a few
	// characters from a UUID. Note that since VM names can't be very long,
	// this prefix needs to be fairly short too.
	// https://cloud.google.com/compute/docs/naming-resources#resource-name-format
	// It's very useful to have today's date in the VM name so that old VMs are
	// easy to identify.
	sandboxPrefix = fmt.Sprintf("test-%s-%s", time.Now().Format("20060102"), uuid.NewString()[:5])

	// This prefix is needed for builds running as build-and-test-external
	// because that service account is only allowed to interact with VMs whose
	// names start with "github-" to isolate them from our release builds.
	// go/sdi-kokoro-security
	if strings.Contains(os.Getenv("SERVICE_EMAIL"), "build-and-test-external@") {
		sandboxPrefix = "github-" + sandboxPrefix
	}

	logRootDir = os.Getenv("TEST_UNDECLARED_OUTPUTS_DIR")
	if logRootDir == "" {
		logRootDir, err = os.MkdirTemp("", "")
		if err != nil {
			log.Fatalf("Couldn't create temporary directory for logs. err=%v", err)
		}
	}
	log.Printf("Detailed logs are in %s\n", logRootDir)
}

// CleanupKeysOrDie deletes ssh key files created in init(). It is intended to
// be called from inside TestMain() after tests have finished running.
func CleanupKeysOrDie() {
	if err := os.RemoveAll(keysDir); err != nil {
		log.Fatalf("CleanupKeysOrDie() failed to remove temporary ssh key dir %v: %v", keysDir, err)
	}
}

type logClientFactory struct {
	mutex sync.Mutex
	// Lazily-initialized map of project to logadmin.Client. Access to this map
	// is guarded by 'mutex'.
	clients map[string]*logadmin.Client
}

// new obtains a logadmin.Client for the given project.
// The Client is cached in a global map so that future calls for the
// same project will return the same Client.
// This function is safe to call concurrently.
func (f *logClientFactory) new(project string) (*logadmin.Client, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if client, ok := f.clients[project]; ok {
		return client, nil
	}

	logClient, err := logadmin.NewClient(context.Background(), project)
	if err != nil {
		return nil, fmt.Errorf("logClientFactory.new() could not construct new logadmin.Client: %v", err)
	}
	f.clients[project] = logClient
	return logClient, nil
}

// VM represents an individual virtual machine.
type VM struct {
	Name        string
	Project     string
	Network     string
	Platform    string
	Zone        string
	MachineType string
	ID          int64
	// The IP address to ssh to. This is the external IP address, unless
	// USE_INTERNAL_IP is set to 'true'. See comment on extractIPAddress() for
	// rationale.
	IPAddress      string
	AlreadyDeleted bool
}

// imageProject returns the image project providing the given image family.
func imageProject(family string) (string, error) {
	firstWord := strings.Split(family, "-")[0]
	switch firstWord {
	case "windows":
		return "windows-cloud", nil
	case "sql":
		return "windows-sql-cloud", nil
	case "centos":
		return "centos-cloud", nil
	case "debian":
		return "debian-cloud", nil
	case "ubuntu":
		return "ubuntu-os-cloud", nil
	case "rhel":
		// There are a few different cases:
		// "rhel-7", "rhel-7-4-sap", and "rhel-7-6-sap-ha".
		if strings.Contains(family, "-sap") {
			return "rhel-sap-cloud", nil
		}
		return "rhel-cloud", nil
	case "rocky":
		return "rocky-linux-cloud", nil
	case "opensuse":
		return "opensuse-cloud", nil
	case "sles":
		// There are a few different cases:
		// "sles-15" and "sles-15-sp*-sap".
		if strings.Contains(family, "-sap") {
			return "suse-sap-cloud", nil
		}
		return "suse-cloud", nil
	default:
		return "", fmt.Errorf("could not find match for family %s", family)
	}
}

// SyslogLocation returns a filesystem path to the system log. This function
// assumes the platform is some kind of Linux.
func SyslogLocation(platform string) string {
	if strings.Contains(platform, "debian") || strings.Contains(platform, "ubuntu") {
		return "/var/log/syslog"
	}
	return "/var/log/messages"
}

// defaultGcloudPath returns "gcloud", unless the environment variable
// GCLOUD_BIN is set, in which case it returns that.
func defaultGcloudPath() string {
	path := os.Getenv("GCLOUD_BIN")
	if path != "" {
		return path
	}
	return "gcloud"
}

var (
	// The path to the gcloud binary to use for all commands run by this test.
	gcloudPath = defaultGcloudPath()
)

// SetGcloudPath configures this library to use the passed-in gcloud binary
// instead of the default gcloud installed on the system.
func SetGcloudPath(path string) {
	gcloudPath = path
}

// IsWindows returns whether the given platform is a version of Windows (including Microsoft SQL Server).
func IsWindows(platform string) bool {
	return strings.HasPrefix(platform, "windows-") || strings.HasPrefix(platform, "sql-")
}

// IsWindowsCore returns whether the given platform is a version of Windows core.
func IsWindowsCore(platform string) bool {
	return strings.HasPrefix(platform, "windows-") && strings.HasSuffix(platform, "-core")
}

// PlatformKind returns "linux" or "windows" based on the given platform.
func PlatformKind(platform string) string {
	if IsWindows(platform) {
		return "windows"
	}
	return "linux"
}

// isRetriableLookupError returns whether the given error, returned from
// lookup[Metric|Trace]() or WaitFor[Metric|Trace](), should be retried.
func isRetriableLookupError(err error) bool {
	if errors.Is(err, ErrInvalidIteratorLength) {
		return true
	}
	myStatus, ok := status.FromError(err)
	// workload.googleapis.com/* domain metrics are created on first write, and may not be immediately queryable.
	// The error doesn't always look the same, hopefully looking for Code() == NotFound will catch all variations.
	// The Internal case catches some transient errors returned by the monitoring API sometimes.
	// The ResourceExhausted case catches API quota errors like:
	// https://source.cloud.google.com/results/invocations/863be0a0-fa7c-4dab-a8df-7689d91513a7.
	return ok && (myStatus.Code() == codes.NotFound || myStatus.Code() == codes.Internal || myStatus.Code() == codes.ResourceExhausted)
}

// lookupMetric does a single lookup of the given metric in the backend.
func lookupMetric(ctx context.Context, logger *log.Logger, vm *VM, metric string, window time.Duration, extraFilters []string, isPrometheus bool) *monitoring.TimeSeriesIterator {
	now := time.Now()
	start := timestamppb.New(now.Add(-window))
	end := timestamppb.New(now)
	filters := []string{
		fmt.Sprintf("metric.type = %q", metric),
	}

	if isPrometheus {
		filters = append(filters, fmt.Sprintf(`resource.labels.namespace = "%d/%s"`, vm.ID, vm.Name))
	} else {
		filters = append(filters, fmt.Sprintf(`resource.labels.instance_id = "%d"`, vm.ID))
	}

	req := &monitoringpb.ListTimeSeriesRequest{
		Name:   "projects/" + vm.Project,
		Filter: strings.Join(append(filters, extraFilters...), " AND "),
		Interval: &monitoringpb.TimeInterval{
			EndTime:   end,
			StartTime: start,
		},
		View: monitoringpb.ListTimeSeriesRequest_FULL,
	}
	return monClient.ListTimeSeries(ctx, req)
}

// lookupTrace does a single lookup of any trace from the given VM in the backend.
func lookupTrace(ctx context.Context, vm *VM, window time.Duration) *trace.TraceIterator {
	now := time.Now()
	start := timestamppb.New(now.Add(-window))
	end := timestamppb.New(now)
	filter := fmt.Sprintf("+g.co/r/gce_instance/instance_id:%d", vm.ID)
	req := &cloudtrace.ListTracesRequest{
		ProjectId: vm.Project,
		Filter:    filter,
		StartTime: start,
		EndTime:   end,
	}
	return traceClient.ListTraces(ctx, req)
}

// nonEmptySeriesList evaluates the given iterator, returning a non-empty slice of
// time series, the length of the slice is guaranteed to be of size minimumRequiredSeries or greater.
// A panic is issued if minimumRequiredSeries is zero or negative.
// An error is returned if the evaluation fails or produces a non-empty slice with length less than minimumRequiredSeries.
// A return value of (nil, nil) indicates that the evaluation succeeded but returned no data.
func nonEmptySeriesList(logger *log.Logger, it *monitoring.TimeSeriesIterator, minimumRequiredSeries int) ([]*monitoringpb.TimeSeries, error) {
	if minimumRequiredSeries < 1 {
		panic("minimumRequiredSeries cannot be negative or 0")
	}
	// Loop through the iterator, looking for at least one non-empty time series.
	tsList := make([]*monitoringpb.TimeSeries, 0)
	for {
		series, err := it.Next()
		logger.Printf("nonEmptySeriesList() iterator supplied err %v and series %v", err, series)
		if err == iterator.Done {
			if len(tsList) == 0 {
				return nil, nil
			}
			if len(tsList) < minimumRequiredSeries {
				return nil, ErrInvalidIteratorLength
			}
			// Success
			return tsList, nil
		}
		if err != nil {
			return nil, err
		}
		if len(series.Points) == 0 {
			// Look at the next element(s) of the iterator.
			continue
		}
		tsList = append(tsList, series)
	}
}

// firstTrace evaluates the given iterator, returning its first trace.
// An error is returned if the evaluation fails.
// A return value of (nil, nil) indicates that the evaluation succeeded
// but returned no data.
func firstTrace(it *trace.TraceIterator) (*cloudtrace.Trace, error) {
	trace, err := it.Next()
	if err == iterator.Done {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return trace, nil
}

// WaitForMetric looks for the given metrics in the backend and returns it if it
// exists. An error is returned otherwise. This function will retry "no data"
// errors a fixed number of times. This is useful because it takes time for
// monitoring data to become visible after it has been uploaded.
func WaitForMetric(ctx context.Context, logger *log.Logger, vm *VM, metric string, window time.Duration, extraFilters []string, isPrometheus bool) (*monitoringpb.TimeSeries, error) {
	series, err := WaitForMetricSeries(ctx, logger, vm, metric, window, extraFilters, isPrometheus, 1)
	if err != nil {
		return nil, err
	}
	logger.Printf("WaitForMetric metric=%v, series=%v", metric, series)
	return series[0], nil
}

// WaitForMetricSeries looks for the given metrics in the backend and returns a slice if it
// exists. An error is returned otherwise. This function will retry "no data"
// errors a fixed number of times. This is useful because it takes time for
// monitoring data to become visible after it has been uploaded.
func WaitForMetricSeries(ctx context.Context, logger *log.Logger, vm *VM, metric string, window time.Duration, extraFilters []string, isPrometheus bool, minimumRequiredSeries int) ([]*monitoringpb.TimeSeries, error) {
	for attempt := 1; attempt <= QueryMaxAttempts; attempt++ {
		it := lookupMetric(ctx, logger, vm, metric, window, extraFilters, isPrometheus)
		tsList, err := nonEmptySeriesList(logger, it, minimumRequiredSeries)

		if tsList != nil && err == nil {
			// Success.
			logger.Printf("Successfully found series=%v", tsList)
			return tsList, nil
		}
		if err != nil && !isRetriableLookupError(err) {
			return nil, fmt.Errorf("WaitForMetric(metric=%q, extraFilters=%v): %v", metric, extraFilters, err)
		}
		// We can get here in two cases:
		// 1. the lookup succeeded but found no data
		// 2. the lookup hit a retriable error. This case happens very rarely.
		logger.Printf("nonEmptySeriesList check(metric=%q, extraFilters=%v): request_error=%v, retrying (%d/%d)...",
			metric, extraFilters, err, attempt, QueryMaxAttempts)

		time.Sleep(queryBackoffDuration)
	}

	return nil, fmt.Errorf("WaitForMetricSeries(metric=%s, extraFilters=%v) failed: %s", metric, extraFilters, exhaustedRetriesSuffix)
}

// WaitForTrace looks for any trace from the given VM in the backend and returns
// it if it exists. An error is returned otherwise. This function will retry
// "no data" errors a fixed number of times. This is useful because it takes
// time for trace data to become visible after it has been uploaded.
//
// Only the ProjectId and TraceId fields are populated. To get other fields,
// including spans, call traceClient.GetTrace with the TraceID returned from
// this function.
func WaitForTrace(ctx context.Context, logger *log.Logger, vm *VM, window time.Duration) (*cloudtrace.Trace, error) {
	for attempt := 1; attempt <= TraceQueryMaxAttempts; attempt++ {
		it := lookupTrace(ctx, vm, window)
		trace, err := firstTrace(it)
		if trace != nil && err == nil {
			return trace, nil
		}
		if err != nil && !isRetriableLookupError(err) {
			return nil, fmt.Errorf("WaitForTrace() failed: %v", err)
		}
		logger.Printf("firstTrace check(): empty, retrying (%d/%d)...",
			attempt, TraceQueryMaxAttempts)
		time.Sleep(time.Duration(traceQueryDerate) * queryBackoffDuration)
	}
	return nil, fmt.Errorf("WaitForTrace() failed: %s", exhaustedRetriesSuffix)
}

// IsExhaustedRetriesMetricError returns true if the given error is an
// "exhausted retries" error returned from WaitForMetric.
func IsExhaustedRetriesMetricError(err error) bool {
	return err != nil && strings.HasSuffix(err.Error(), exhaustedRetriesSuffix)
}

// AssertMetricMissing looks for data of a metric and returns success if
// no data is found. To consider possible transient errors while querying
// the backend we make queryMaxAttemptsMetricMissing query attempts.
func AssertMetricMissing(ctx context.Context, logger *log.Logger, vm *VM, metric string, isPrometheus bool, window time.Duration) error {
	descriptorNotFoundErrCount := 0
	for attempt := 1; attempt <= queryMaxAttemptsMetricMissing; attempt++ {
		it := lookupMetric(ctx, logger, vm, metric, window, nil, isPrometheus)
		series, err := nonEmptySeriesList(logger, it, 1)
		found := len(series) > 0
		logger.Printf("nonEmptySeriesList check(metric=%q): err=%v, found=%v, attempt (%d/%d)",
			metric, err, found, attempt, queryMaxAttemptsMetricMissing)

		if err == nil {
			if found {
				return fmt.Errorf("AssertMetricMissing(metric=%q): %v failed: unexpectedly found data for metric", metric, err)
			}
			// Success
			return nil
		}
		if !isRetriableLookupError(err) {
			return fmt.Errorf("AssertMetricMissing(metric=%q): %v", metric, err)
		}

		// prometheus.googleapis.com/* domain metrics are created on first write, and may not be immediately queryable.
		// The error doesn't always look the same, hopefully looking for Code() == NotFound will catch all variations.
		myStatus, ok := status.FromError(err)
		if ok && isPrometheus && myStatus.Code() == codes.NotFound {
			descriptorNotFoundErrCount += 1
		}
		time.Sleep(queryBackoffDuration)
	}
	if !isPrometheus {
		return fmt.Errorf("AssertMetricMissing(metric=%q): failed: no successful queries to the backend", metric)
	}

	if descriptorNotFoundErrCount != queryMaxAttemptsMetricMissing {
		return fmt.Errorf("AssertMetricMissing(metric=%q): failed: atleast one query failed with something other than a NOT_FOUND error", metric)
	}

	// Success
	return nil
}

// hasMatchingLog looks in the logging backend for a log matching the given query,
// over the trailing time interval specified by the given window.
// Returns a boolean indicating whether the log was present in the backend,
// plus the first log entry found, or an error if the lookup failed.
func hasMatchingLog(ctx context.Context, logger *log.Logger, vm *VM, logNameRegex string, window time.Duration, query string) (bool, *cloudlogging.Entry, error) {
	start := time.Now().Add(-window)

	t := start.Format(time.RFC3339)
	filter := fmt.Sprintf(`logName=~"projects/%s/logs/%s" AND resource.labels.instance_id="%d" AND timestamp > "%s"`, vm.Project, logNameRegex, vm.ID, t)
	if query != "" {
		filter += fmt.Sprintf(` AND %s`, query)
	}
	logger.Println(filter)

	logClient, err := logClients.new(vm.Project)
	if err != nil {
		return false, nil, fmt.Errorf("hasMatchingLog() failed to obtain logClient for project %v: %v", vm.Project, err)
	}
	it := logClient.Entries(ctx, logadmin.Filter(filter))
	found := false

	var first *cloudlogging.Entry
	// Loop through the iterator printing out each matching log entry. We could return true on the
	// first match, but it's nice for debugging to print out all matches into the logs.
	for {
		entry, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return false, nil, err
		}
		logger.Printf("Found matching log entry: %v", entry)
		found = true
		if first == nil {
			first = entry
		}
	}
	return found, first, nil
}

// WaitForLog looks in the logging backend for a log matching the given query,
// over the trailing time interval specified by the given window.
// Returns an error if the log could not be found after QueryMaxAttempts retries.
func WaitForLog(ctx context.Context, logger *log.Logger, vm *VM, logNameRegex string, window time.Duration, query string) error {
	_, err := QueryLog(ctx, logger, vm, logNameRegex, window, query, QueryMaxAttempts)
	return err
}

// QueryLog looks in the logging backend for a log matching the given query,
// over the trailing time interval specified by the given window.
// Returns the first log entry found, or an error if the log could not be
// found after some retries.
func QueryLog(ctx context.Context, logger *log.Logger, vm *VM, logNameRegex string, window time.Duration, query string, maxAttempts int) (*cloudlogging.Entry, error) {
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		found, first, err := hasMatchingLog(ctx, logger, vm, logNameRegex, window, query)
		if found {
			// Success.
			return first, nil
		}
		logger.Printf("Query returned found=%v, err=%v, attempt=%d", found, err, attempt)
		if err != nil && !strings.Contains(err.Error(), "Internal error encountered") {
			// A non-retryable error.
			return nil, fmt.Errorf("QueryLog() failed: %v", err)
		}
		// found was false, or we hit a retryable error.
		time.Sleep(queryBackoffDuration)
	}
	return nil, fmt.Errorf("QueryLog() failed: %s not found, exhausted retries", logNameRegex)
}

// AssertLogMissing looks in the logging backend for a log matching the given query
// and returns success if no data is found. To consider possible transient errors
// while querying the backend we make queryMaxAttemptsMetricMissing query attempts.
func AssertLogMissing(ctx context.Context, logger *log.Logger, vm *VM, logNameRegex string, window time.Duration, query string) error {
	for attempt := 1; attempt <= queryMaxAttemptsLogMissing; attempt++ {
		found, _, err := hasMatchingLog(ctx, logger, vm, logNameRegex, window, query)
		if err == nil {
			if found {
				return fmt.Errorf("AssertLogMissing(log=%q): %v failed: unexpectedly found data for log", query, err)
			}
			// Success
			return nil
		}
		logger.Printf("Query returned found=%v, err=%v, attempt=%d", found, err, attempt)
		if err != nil && !strings.Contains(err.Error(), "Internal error encountered") {
			// A non-retryable error.
			return fmt.Errorf("AssertLogMissing() failed: %v", err)
		}
		// found was false, or we hit a retryable error.
		time.Sleep(queryBackoffDuration)
	}

	// Success
	return nil
}

// CommandOutput holds the textual output from running a subprocess.
type CommandOutput struct {
	Stdout string
	Stderr string
}

type ThreadSafeWriter struct {
	mu      sync.Mutex
	guarded io.Writer
}

func (writer *ThreadSafeWriter) Write(p []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.guarded.Write(p)
}

// runCommand invokes a binary and waits until it finishes. Returns the stdout
// and stderr, and an error if the binary had a nonzero exit code.
// args is a slice containing the binary to invoke along with all its arguments,
// e.g. {"echo", "hello"}.
func runCommand(ctx context.Context, logger *log.Logger, stdin io.Reader, args []string) (CommandOutput, error) {
	var output CommandOutput
	if len(args) < 1 {
		return output, fmt.Errorf("runCommand() needs a nonempty argument slice, got %v", args)
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)

	cmd.Stdin = stdin

	var stdoutBuilder strings.Builder
	var stderrBuilder strings.Builder
	var interleavedBuilder strings.Builder

	interleavedWriter := &ThreadSafeWriter{guarded: &interleavedBuilder}
	cmd.Stdout = io.MultiWriter(&stdoutBuilder, interleavedWriter)
	cmd.Stderr = io.MultiWriter(&stderrBuilder, interleavedWriter)

	err := cmd.Run()

	if err != nil {
		err = fmt.Errorf("Command failed: %v\n%v\nstdout+stderr: %s", args, err, interleavedBuilder.String())
	}

	logger.Printf("exit code: %v", cmd.ProcessState.ExitCode())
	logger.Printf("stdout+stderr: %s", interleavedBuilder.String())

	output.Stdout = stdoutBuilder.String()
	output.Stderr = stderrBuilder.String()

	return output, err
}

// RunGcloud invokes a gcloud binary from runfiles and waits until it finishes.
// Returns the stdout and stderr and an error if the binary had a nonzero exit
// code. args is a slice containing the arguments to pass to gcloud.
//
// Note: most calls to this function could be replaced by calls to the Compute API
// (https://cloud.google.com/compute/docs/reference/rest/v1).
// Various pros/cons of shelling out to gcloud vs using the Compute API are discussed here:
// http://go/sdi-gcloud-vs-api
func RunGcloud(ctx context.Context, logger *log.Logger, stdin string, args []string) (CommandOutput, error) {
	logger.Printf("Running command: gcloud %v", args)
	return runCommand(ctx, logger, strings.NewReader(stdin), append([]string{gcloudPath}, args...))
}

var (
	sshOptions = []string{
		// In some situations, ssh will hang when connecting to a new VM unless
		// it has an explicit connection timeout set.
		"-oConnectTimeout=120",
		// StrictHostKeyChecking is disabled because the host keys are unknown
		// to us at the start of the test.
		"-oStrictHostKeyChecking=no",
		// UserKnownHostsFile is set to /dev/null to avoid a rare logspam problem
		// where ssh sees that a host key has changed (I'm not sure why this happens)
		// and prints a big warning banner each time it is invoked.
		"-oUserKnownHostsFile=/dev/null",
		// LogLevel is set to ERROR to hide a warning that ssh prints on every invocation
		// like "Warning: Permanently added <IP address> (ECDSA) to the list of known hosts."
		// (even though UserKnownHostsFile is /dev/null).
		// If you are debugging ssh problems, you'll probably want to remove this option.
		"-oLogLevel=ERROR",
		// Sometimes you can be prompted to auth with a password if OpenSSH isn't
		// ready yet on Windows, which hangs the test. We only ever auth with keys so
		// let's disable password auth.
		"-oPreferredAuthentications=publickey",
	}
)

func wrapPowershellCommand(command string) (string, error) {
	uni := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	encoded, err := uni.NewEncoder().String(command)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("powershell -NonInteractive -EncodedCommand %q", base64.StdEncoding.EncodeToString([]byte(encoded))), nil
}

// RunRemotely runs a command on the provided VM.
// The command should be a shell command if the VM is Linux, or powershell if the VM is Windows.
// Returns the stdout and stderr, plus an error if there was a problem.
//
// 'command' is what to run on the machine. Example: "cat /tmp/foo; echo hello"
// For extremely long commands, use RunScriptRemotely instead.
// 'stdin' is what to supply to the command on stdin. It is usually "".
// TODO: Remove the stdin parameter. Any callsite that needs to pass
// data over standard input should use RunRemotelyStdin.
//
// When making changes to this function, please test them by running
// gce_testing_test.go (manually).
func RunRemotely(ctx context.Context, logger *log.Logger, vm *VM, stdin string, command string) (_ CommandOutput, err error) {
	return RunRemotelyStdin(ctx, logger, vm, strings.NewReader(stdin), command)
}

// RunRemotelyStdin is just like RunRemotely but it accepts an io.Reader
// for what data to pass in over standard input to the command.
func RunRemotelyStdin(ctx context.Context, logger *log.Logger, vm *VM, stdin io.Reader, command string) (_ CommandOutput, err error) {
	logger.Printf("Running command remotely: %v", command)
	defer func() {
		if err != nil {
			err = fmt.Errorf("Command failed: %v\n%v", command, err)
		}
	}()
	wrappedCommand := command
	if IsWindows(vm.Platform) {
		wrappedCommand, err = wrapPowershellCommand(command)
		if err != nil {
			return CommandOutput{}, err
		}
	}
	// Raw ssh is used instead of "gcloud compute ssh" with OS Login because:
	// 1. OS Login will generate new ssh keys for each kokoro run and they don't carry over.
	//    This means that they pile up and need to be deleted periodically.
	// 2. We saw a variety of flaky issues when using gcloud, see b/171810719#comment6.
	//    "gcloud compute ssh" does not work reliably when run concurrently with itself.
	args := []string{"ssh"}
	args = append(args, sshUserName+"@"+vm.IPAddress)
	args = append(args, "-oIdentityFile="+privateKeyFile)
	args = append(args, sshOptions...)
	args = append(args, wrappedCommand)
	return runCommand(ctx, logger, stdin, args)
}

// UploadContent takes an io.Reader and uploads its contents as a file to a
// given path on the given VM.
//
// In order for this function to work, the currently active application default
// credentials (GOOGLE_APPLICATION_CREDENTIALS) need to be able to upload to
// fileTransferBucket, and also the role running on the remote VM needs to be
// given permission to read from that bucket. This was accomplished by adding
// the "Compute Engine default service account" for PROJECT as
// a "Storage Object Viewer" and "Storage Object Creator" on the bucket.
func UploadContent(ctx context.Context, logger *log.Logger, vm *VM, content io.Reader, remotePath string) (err error) {
	defer func() {
		if err != nil {
			logger.Printf("Uploading file finished with err=%v", err)
		}
	}()
	object := storageClient.Bucket(transfersBucket).Object(path.Join(vm.Name, remotePath))
	writer := object.NewWriter(ctx)
	_, copyErr := io.Copy(writer, content)
	// We have to make sure to call Close() here in order to tell it to finish
	// the upload operation.
	closeErr := writer.Close()
	err = multierr.Combine(copyErr, closeErr)
	if err != nil {
		return fmt.Errorf("UploadContent() could not write data into storage object: %v", err)
	}
	// Make sure to clean up the object once we're done with it.
	// Note: if the preceding io.Copy() or writer.Close() fails, the object will
	// not be uploaded and there is no need to delete it:
	// https://cloud.google.com/storage/docs/resumable-uploads#introduction
	// (note that the go client libraries use resumable uploads).
	defer func() {
		deleteErr := object.Delete(ctx)
		if deleteErr != nil {
			err = fmt.Errorf("UploadContent() finished with err=%v, then cleanup of %v finished with err=%v", err, object.ObjectName(), deleteErr)
		}
	}()

	if IsWindows(vm.Platform) {
		_, err = RunRemotely(ctx, logger, vm, "", fmt.Sprintf(`Read-GcsObject -Force -Bucket "%s" -ObjectName "%s" -OutFile "%s"`, object.BucketName(), object.ObjectName(), remotePath))
		return err
	}
	if err := InstallGsutilIfNeeded(ctx, logger, vm); err != nil {
		return err
	}
	objectPath := fmt.Sprintf("gs://%s/%s", object.BucketName(), object.ObjectName())
	_, err = RunRemotely(ctx, logger, vm, "", fmt.Sprintf("sudo gsutil cp '%s' '%s'", objectPath, remotePath))
	return err
}

// envVarMapToBashPrefix converts a map of env variable name to value into a string
// suitable for passing to bash as a way to set those variables. The environment values
// are wrapped in quotes. Example output: `VAR1='foo' VAR2='bar' `
func envVarMapToBashPrefix(env map[string]string) string {
	var builder strings.Builder
	for key, value := range env {
		fmt.Fprintf(&builder, "%s='%s' ", key, value)
	}
	return builder.String()
}

// envVarMapToPowershellPrefix converts a map of env variable name to value into a string
// suitable for prepending onto a powershell command as a way to set those variables.
// Example output: "$env:VAR1='foo'\n$env:VAR2='bar'\n"
func envVarMapToPowershellPrefix(env map[string]string) string {
	var builder strings.Builder
	for key, value := range env {
		fmt.Fprintf(&builder, "$env:%s='%s'\n", key, value)
	}
	return builder.String()
}

// RunScriptRemotely runs a script on the given VM.
// The script should be a shell script for a Linux VM and powershell for a Windows VM.
// env is a map containing environment variables to provide to the script as it runs.
// The environment variables and the flags will be wrapped in quotes.
// This function is necessary to handle long commands, particularly on Windows,
// since there is a length limit on the commands you can pass to RunRemotely:
// powershell will complain if its -EncodedCommand parameter is too long.
// It is highly recommended that any powershell script passed in here start with:
// $ErrorActionPreference = 'Stop'
// This will cause a broader class of errors to be reported as an error (nonzero exit code)
// by powershell.
func RunScriptRemotely(ctx context.Context, logger *log.Logger, vm *VM, scriptContents string, flags []string, env map[string]string) (CommandOutput, error) {
	var quotedFlags []string
	for _, flag := range flags {
		quotedFlags = append(quotedFlags, fmt.Sprintf("'%s'", flag))
	}
	flagsStr := strings.Join(quotedFlags, " ")

	if IsWindows(vm.Platform) {
		// Use a UUID for the script name in case RunScriptRemotely is being
		// called concurrently on the same VM.
		scriptPath := "C:\\" + uuid.NewString() + ".ps1"
		if err := UploadContent(ctx, logger, vm, strings.NewReader(scriptContents), scriptPath); err != nil {
			return CommandOutput{}, err
		}
		// powershell -File seems to drop certain kinds of errors:
		// https://stackoverflow.com/a/15779295
		// In testing, adding $ErrorActionPreference = 'Stop' to the start of each
		// script seems to work around this completely.
		//
		// To test changes to this command, please run gce_testing_test.go (manually).
		return RunRemotely(ctx, logger, vm, "", envVarMapToPowershellPrefix(env)+"powershell -File "+scriptPath+" "+flagsStr)
	}
	scriptPath := uuid.NewString() + ".sh"
	// Write the script contents to <UUID>.sh, then tell bash to execute it with -x
	// to print each line as it runs.
	// Use a UUID for the script name in case RunScriptRemotely is being called
	// concurrently on the same VM.
	//
	// Note: if we ever decide to support a stdin parameter to this function, we can
	// accomplish that by splitting the below command into two RunRemotely() calls:
	// one to put scriptContents into a file and another to execute the script.
	//
	// To test changes to this command, please run gce_testing_test.go (manually).
	return RunRemotely(ctx, logger, vm, scriptContents, "cat - > "+scriptPath+" && sudo "+envVarMapToBashPrefix(env)+"bash -x "+scriptPath+" "+flagsStr)
}

// MapToCommaSeparatedList converts a map of key-value pairs into a form that
// gcloud will accept, which is a comma separated list with "=" between each
// key-value pair. For example: "KEY1=VALUE1,KEY2=VALUE2"
func MapToCommaSeparatedList(mapping map[string]string) string {
	var elems []string
	for k, v := range mapping {
		elems = append(elems, k+"="+v)
	}
	return strings.Join(elems, ",")
}

func instanceLogURL(vm *VM) string {
	return fmt.Sprintf("https://console.cloud.google.com/logs/viewer?resource=gce_instance%%2Finstance_id%%2F%d&project=%s", vm.ID, vm.Project)
}

const (
	prepareSLESMessage = "prepareSLES() failed"
)

// prepareSLES runs some preliminary steps that get a SLES VM ready to install packages.
// First it repeatedly runs registercloudguest, then it repeatedly tries installing a dummy package until it succeeds.
// When that happens, the VM is ready to install packages.
// See b/148612123 and b/196246592 for some history about this.
func prepareSLES(ctx context.Context, logger *log.Logger, vm *VM) error {
	backoffPolicy := backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(5*time.Second), 5), ctx) // 5 attempts.
	err := backoff.Retry(func() error {
		_, err := RunRemotely(ctx, logger, vm, "", "sudo /usr/sbin/registercloudguest --force")
		return err
	}, backoffPolicy)
	if err != nil {
		RunRemotely(ctx, logger, vm, "", "sudo cat /var/log/cloudregister")
		return fmt.Errorf("error running registercloudguest: %v", err)
	}

	backoffPolicy = backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(5*time.Second), 120), ctx) // 10 minutes max.
	err = backoff.Retry(func() error {
		// --gpg-auto-import-keys is included to fix a rare flake where (due to
		// a policy being installed already) there is a new key that needs to
		// be imported.
		// timezone-java was selected arbitrarily as a package that:
		// a) can be installed from the default repos, and
		// b) isn't installed already.
		_, zypperErr := RunRemotely(ctx, logger, vm, "", "sudo zypper --non-interactive --gpg-auto-import-keys refresh && sudo zypper --non-interactive install timezone-java")
		return zypperErr
	}, backoffPolicy)
	if err != nil {
		RunRemotely(ctx, logger, vm, "", "sudo cat /var/log/zypper.log")
	}
	return err
}

var (
	overriddenImageFamilies = map[string]string{
		"opensuse-leap-15-4": "opensuse-leap-15-4-v20231208-x86-64",
	}
)

func addFrameworkMetadata(platform string, inputMetadata map[string]string) (map[string]string, error) {
	metadataCopy := make(map[string]string)

	// Set serial-port-logging-enable to true by default to help diagnose startup
	// issues. inputMetadata can override this setting.
	metadataCopy["serial-port-logging-enable"] = "true"

	for k, v := range inputMetadata {
		metadataCopy[k] = v
	}

	if _, ok := metadataCopy["enable-oslogin"]; ok {
		return nil, errors.New("the 'enable-oslogin' metadata key is reserved for framework use")
	}
	// We manage our own ssh keys, so we don't need OS Login. For a while, it
	// worked to leave it enabled anyway, but one day that broke (b/181867249).
	// Disabling OS Login fixed the issue.
	metadataCopy["enable-oslogin"] = "false"

	if _, ok := metadataCopy["ssh-keys"]; ok {
		return nil, errors.New("the 'ssh-keys' metadata key is reserved for framework use")
	}
	publicKey, err := os.ReadFile(publicKeyFile)
	if err != nil {
		return nil, fmt.Errorf("could not read local public key file %v: %v", publicKeyFile, err)
	}
	metadataCopy["ssh-keys"] = fmt.Sprintf("%s:%s", sshUserName, string(publicKey))

	if IsWindows(platform) {
		// From https://cloud.google.com/compute/docs/connect/windows-ssh#create_vm
		if _, ok := metadataCopy["sysprep-specialize-script-cmd"]; ok {
			return nil, errors.New("you cannot pass a sysprep script for Windows instances because they are needed to enable ssh-ing. Instead, wait for the instance to be ready and then run things with RunRemotely() or RunScriptRemotely()")
		}
		metadataCopy["sysprep-specialize-script-cmd"] = "googet -noconfirm=true install google-compute-engine-ssh"

		if _, ok := metadataCopy["enable-windows-ssh"]; ok {
			return nil, errors.New("the 'enable-windows-ssh' metadata key is reserved for framework use")
		}
		metadataCopy["enable-windows-ssh"] = "TRUE"
	} else {
		if _, ok := metadataCopy["startup-script"]; ok {
			return nil, errors.New("the 'startup-script' metadata key is reserved for future use. Instead, wait for the instance to be ready and then run things with RunRemotely() or RunScriptRemotely()")
		}
	}
	return metadataCopy, nil
}

func addFrameworkLabels(inputLabels map[string]string) (map[string]string, error) {
	labelsCopy := make(map[string]string)
	for k, v := range inputLabels {
		labelsCopy[k] = v
	}

	// Attach the Kokoro ID to the instance to aid in debugging.
	if buildID := os.Getenv("KOKORO_BUILD_ID"); buildID != "" {
		labelsCopy["kokoro_build_id"] = buildID
	}

	return labelsCopy, nil
}

func getVMPlatform(image string, platform string) (string, error) {
	if image != "" && platform != "" {
		return "", fmt.Errorf("Both platform and image cannot be specified in VMOptions.")
	}

	if image != "" {
		return image, nil
	}

	if platform != "" {
		return platform, nil
	}

	return "", errors.New("at least one of image or platform must be specified")
}

// attemptCreateInstance creates a VM instance and waits for it to be ready.
// Returns a VM object or an error (never both). The caller is responsible for
// deleting the VM if (and only if) the returned error is nil.
func attemptCreateInstance(ctx context.Context, logger *log.Logger, options VMOptions) (vmToReturn *VM, errToReturn error) {

	platform, err := getVMPlatform(options.Image, options.Platform)
	if err != nil {
		return nil, err
	}
	vm := &VM{
		Project:  options.Project,
		Platform: platform,
		Name:     options.Name,
		Network:  os.Getenv("NETWORK_NAME"),
		Zone:     options.Zone,
	}
	if vm.Name == "" {
		// The VM name needs to adhere to these restrictions:
		// https://cloud.google.com/compute/docs/naming-resources#resource-name-format
		vm.Name = fmt.Sprintf("%s-%s", sandboxPrefix, uuid.New())
	}
	if vm.Project == "" {
		vm.Project = os.Getenv("PROJECT")
	}
	if vm.Network == "" {
		vm.Network = "default"
	}
	if vm.Zone == "" {
		// Chooses the next zone from ZONES.
		vm.Zone = zonePicker.Next()
	}

	// Note: INSTANCE_SIZE takes precedence over options.MachineType.
	vm.MachineType = os.Getenv("INSTANCE_SIZE")
	if vm.MachineType == "" {
		vm.MachineType = options.MachineType
	}
	if vm.MachineType == "" {
		vm.MachineType = "e2-standard-4"
		if IsARM(vm.Platform) {
			vm.MachineType = "t2a-standard-4"
		}
	}

	imgProject := options.ImageProject
	if imgProject == "" {
		var err error
		imgProject, err = imageProject(vm.Platform)
		if err != nil {
			return nil, fmt.Errorf("attemptCreateInstance() could not find image project: %v", err)
		}
	}
	newMetadata, err := addFrameworkMetadata(vm.Platform, options.Metadata)
	if err != nil {
		return nil, fmt.Errorf("attemptCreateInstance() could not construct valid metadata: %v", err)
	}
	newLabels, err := addFrameworkLabels(options.Labels)
	if err != nil {
		return nil, fmt.Errorf("attemptCreateInstance() could not construct valid labels: %v", err)
	}

	imageOrImageFamilyFlag := "--image=" + options.Image

	if options.Platform != "" {
		imageOrImageFamilyFlag = "--image-family=" + options.Platform

		if image, ok := overriddenImageFamilies[options.Platform]; ok {
			imageOrImageFamilyFlag = "--image=" + image
		}

	}

	imageFamilyScope := options.ImageFamilyScope

	if imageFamilyScope == "" {
		imageFamilyScope = "global"
	}

	args := []string{
		// "beta" is needed for --max-run-duration below.
		"beta", "compute", "instances", "create", vm.Name,
		"--project=" + vm.Project,
		"--zone=" + vm.Zone,
		"--machine-type=" + vm.MachineType,
		"--image-project=" + imgProject,
		imageOrImageFamilyFlag,
		"--image-family-scope=" + imageFamilyScope,
		"--network=" + vm.Network,
		"--format=json",
	}
	if len(newMetadata) > 0 {
		// The --metadata flag can't be empty, so we have to have a special case
		// to omit the flag completely when the newMetadata map is empty.
		args = append(args, "--metadata="+MapToCommaSeparatedList(newMetadata))
	}
	if len(newLabels) > 0 {
		args = append(args, "--labels="+MapToCommaSeparatedList(newLabels))
	}
	if email := os.Getenv("SERVICE_EMAIL"); email != "" {
		args = append(args, "--service-account="+email)
	}
	if internalIP := os.Getenv("USE_INTERNAL_IP"); internalIP == "true" {
		// Don't assign an external IP address. This is to avoid using up
		// a very limited budget of external IPv4 addresses. The instances
		// will talk to the external internet by routing through a Cloud NAT
		// gateway that is configured in our testing project.
		args = append(args, "--no-address")
	}
	if options.TimeToLive != "" {
		args = append(args, "--max-run-duration="+options.TimeToLive, "--instance-termination-action=DELETE", "--provisioning-model=STANDARD")
	}
	args = append(args, options.ExtraCreateArguments...)

	output, err := RunGcloud(ctx, logger, "", args)
	if err != nil {
		// Note: we don't try and delete the VM in this case because there is
		// nothing to delete.
		return nil, err
	}

	defer func() {
		if errToReturn != nil {
			// This function is responsible for deleting the VM in all error cases.
			errToReturn = multierr.Append(errToReturn, DeleteInstance(logger, vm))
			// Make sure to never return both a valid VM object and an error.
			vmToReturn = nil
		}
		if errToReturn == nil && vm == nil {
			errToReturn = errors.New("programming error: attemptCreateInstance() returned nil VM and nil error")
		}
	}()

	// Pull the instance ID and external IP address out of the output.
	id, err := extractID(output.Stdout)
	if err != nil {
		return nil, err
	}
	vm.ID = id

	logger.Printf("Instance Log: %v", instanceLogURL(vm))

	ipAddress, err := extractIPAddress(output.Stdout)
	if err != nil {
		return nil, err
	}
	vm.IPAddress = ipAddress

	// RunGcloud will log the output of the command, so we don't need to.
	if _, err = RunGcloud(ctx, logger, "", []string{
		"compute", "disks", "describe", vm.Name,
		"--project=" + vm.Project,
		"--zone=" + vm.Zone,
		"--format=json",
	}); err != nil {
		// This is just informational, so it's ok if it fails. Just warn and proceed.
		logger.Printf("Unable to retrieve information about the VM's boot disk: %v", err)
	}

	if err := waitForStart(ctx, logger, vm); err != nil {
		return nil, err
	}

	if IsSUSE(vm.Platform) {
		// Set download.max_silent_tries to 5 (by default, it is commented out in
		// the config file). This should help with issues like b/211003972.
		if _, err := RunRemotely(ctx, logger, vm, "", "sudo sed -i -E 's/.*download.max_silent_tries.*/download.max_silent_tries = 5/g' /etc/zypp/zypp.conf"); err != nil {
			return nil, fmt.Errorf("attemptCreateInstance() failed to configure retries in zypp.conf: %v", err)
		}
	}

	if strings.HasPrefix(vm.Platform, "sles-") && os.Getenv("DISABLE_PREPARE_SLES") != "true" {
		if err := prepareSLES(ctx, logger, vm); err != nil {
			return nil, fmt.Errorf("%s: %v", prepareSLESMessage, err)
		}
	}

	if IsSUSE(vm.Platform) {
		// Set ZYPP_LOCK_TIMEOUT so tests that use zypper don't randomly fail
		// because some background process happened to be using zypper at the same time.
		if _, err := RunRemotely(ctx, logger, vm, "", `echo 'ZYPP_LOCK_TIMEOUT=300' | sudo tee -a /etc/environment`); err != nil {
			return nil, err
		}
	}

	// Removing flaky rhel-7 repositories due to b/265341502
	if isRHEL7SAPHA(vm.Platform) {
		if _, err := RunRemotely(ctx,
			logger, vm, "", `sudo yum -y --disablerepo=rhui-rhel*-7-* install yum-utils && sudo yum-config-manager --disable "rhui-rhel*-7-*"`); err != nil {
			return nil, fmt.Errorf("disabling flaky repos failed : %w", err)
		}
	}

	return vm, nil
}

func IsSUSE(platform string) bool {
	return strings.HasPrefix(platform, "sles-") || strings.HasPrefix(platform, "opensuse-")
}

func IsCentOS(platform string) bool {
	return strings.HasPrefix(platform, "centos-")
}

func IsRHEL(platform string) bool {
	return strings.HasPrefix(platform, "rhel-")
}

func isRHEL7SAPHA(platform string) bool {
	return strings.HasPrefix(platform, "rhel-7") && strings.HasSuffix(platform, "-sap-ha")
}

func IsARM(platform string) bool {
	// At the time of writing, all ARM images and image families on GCE
	// contain "arm64" (and none contain "aarch" nor "arm" without the "64").
	return strings.Contains(platform, "arm64")
}

// CreateInstance launches a new VM instance based on the given options.
// Also waits for the instance to be reachable over ssh.
// Returns a VM object or an error (never both). The caller is responsible for
// deleting the VM if (and only if) the returned error is nil.
func CreateInstance(origCtx context.Context, logger *log.Logger, options VMOptions) (*VM, error) {
	// Give enough time for at least 3 consecutive attempts to start a VM.
	// If an attempt returns a non-retriable error, it will be returned
	// immediately.
	// If retriable errors happen quickly, there will be more than 3 attempts.
	// If retriable errors happen slowly, there will still be at least 3 attempts.
	ctx, cancel := context.WithTimeout(origCtx, 3*vmInitTimeout)
	defer cancel()

	shouldRetry := func(err error) bool {
		// VM creation can hit quota, especially when re-running presubmits,
		// or when multple people are running tests.
		return strings.Contains(err.Error(), "Quota") ||
			// Rarely, instance creation fails due to internal errors in the compute API.
			strings.Contains(err.Error(), "Internal error") ||
			// Instance creation can also fail due to service unavailability.
			strings.Contains(err.Error(), "currently unavailable") ||
			// This error is a consequence of running gcloud concurrently, which is actually
			// unsupported. In the absence of a better fix, just retry such errors.
		        strings.Contains(err.Error(), "database is locked") ||
			// windows-*-core instances sometimes fail to be ssh-able: b/305721001
			(IsWindowsCore(options.Platform) && strings.Contains(err.Error(), windowsStartupFailedMessage)) ||
			// SLES instances sometimes fail to be ssh-able: b/186426190
			(IsSUSE(options.Platform) && strings.Contains(err.Error(), startupFailedMessage)) ||
			strings.Contains(err.Error(), prepareSLESMessage)
	}

	var vm *VM
	createFunc := func() error {
		attemptCtx, cancel := context.WithTimeout(ctx, vmInitTimeout)
		defer cancel()

		var err error
		vm, err = attemptCreateInstance(attemptCtx, logger, options)

		if err != nil && !shouldRetry(err) {
			err = backoff.Permanent(err)
		}
		// Returning a non-permanent error triggers retries.
		return err
	}
	backoffPolicy := backoff.WithContext(backoff.NewConstantBackOff(time.Minute), ctx)
	if err := backoff.Retry(createFunc, backoffPolicy); err != nil {
		return nil, err
	}
	logger.Printf("VM is ready: %#v", vm)
	return vm, nil
}

// RemoveExternalIP deletes the external ip for an instance.
func RemoveExternalIP(ctx context.Context, logger *log.Logger, vm *VM) error {
	_, err := RunGcloud(ctx, logger, "",
		[]string{
			"compute", "instances", "delete-access-config",
			"--project=" + vm.Project,
			"--zone=" + vm.Zone,
			vm.Name,
			"--access-config-name=external-nat",
		})
	return err
}

// SetEnvironmentVariables sets the environment variables in the envVariables map on the given vm in a platform-dependent way.
// On Windows platforms, variables set this way are visible to all processes.
// On Linux platforms, variables set this way are visible to the Ops Agent services only.
func SetEnvironmentVariables(ctx context.Context, logger *log.Logger, vm *VM, envVariables map[string]string) error {
	if IsWindows(vm.Platform) {
		for key, value := range envVariables {
			envVariableCmd := fmt.Sprintf(`setx %s "%s" /M`, key, value)
			logger.Println("envVariableCmd " + envVariableCmd)
			if _, err := RunRemotely(ctx, logger, vm, "", envVariableCmd); err != nil {
				return err
			}
		}
		return nil
	}
	// https://serverfault.com/a/413408
	// Escaping newlines here because we'll be using `echo -e` later
	override := `[Service]\n`
	for key, value := range envVariables {
		override += fmt.Sprintf(`Environment="%s=%s"\n`, key, value)
	}
	for _, service := range []string{
		"google-cloud-ops-agent",
		"google-cloud-ops-agent-diagnostics",
		"google-cloud-ops-agent-fluent-bit",
		"google-cloud-ops-agent-opentelemetry-collector",
	} {
		dir := fmt.Sprintf("/etc/systemd/system/%s.service.d", service)
		cmd := fmt.Sprintf(`sudo mkdir -p %s && echo -e '%s' | sudo tee %s/override.conf`, dir, override, dir)
		if _, err := RunRemotely(ctx, logger, vm, "", cmd); err != nil {
			return err
		}
	}
	// Reload the systemd daemon to pick up the new settings edited in the previous command
	daemonReload := "sudo systemctl daemon-reload"
	_, err := RunRemotely(ctx, logger, vm, "", daemonReload)
	return err
}

// DeleteInstance deletes the given VM instance synchronously.
// Does nothing if the VM was already deleted.
// Doesn't take a Context argument because even if the test has timed out or is
// cancelled, we still want to delete the VMs.
func DeleteInstance(logger *log.Logger, vm *VM) error {
	if vm.AlreadyDeleted {
		logger.Printf("VM %v was already deleted, skipping delete.", vm.Name)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	backoffPolicy := backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 10), ctx)
	attempt := 0
	tryDelete := func() error {
		attempt++
		_, err := RunGcloud(ctx, logger, "",
			[]string{
				"compute", "instances", "delete",
				"--project=" + vm.Project,
				"--zone=" + vm.Zone,
				vm.Name,
			})
		if err == nil {
			return nil
		}
		// GCE sometimes responds with 502 or 503 errors. Retry these errors
		// (and other 50x errors for good measure), by returning them directly.
		if strings.Contains(err.Error(), "Error 50") {
			return err
		}
		// "not found" can happen when a previous attempt actually did delete
		// the VM but there was some communication problem along the way.
		// Consider that a successful deletion. Only do this when there has
		// been a previous attempt.
		if strings.Contains(err.Error(), "not found") && attempt > 1 {
			return nil
		}
		// Wrap other errors in backoff.Permanent() to avoid retrying those.
		return backoff.Permanent(err)
	}
	err := backoff.Retry(tryDelete, backoffPolicy)
	if err == nil {
		vm.AlreadyDeleted = true
	}
	return err
}

// StopInstance shuts down a VM instance.
func StopInstance(ctx context.Context, logger *log.Logger, vm *VM) error {
	_, err := RunGcloud(ctx, logger, "",
		[]string{
			"compute", "instances", "stop",
			"--project=" + vm.Project,
			"--zone=" + vm.Zone,
			vm.Name,
		})
	return err
}

// StartInstance boots a previously-stopped VM instance.
// Also waits for the instance to be started up.
func StartInstance(ctx context.Context, logger *log.Logger, vm *VM) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()

	var output CommandOutput
	tryStart := func() error {
		var err error
		output, err = RunGcloud(ctx, logger, "",
			[]string{
				"compute", "instances", "start",
				"--project=" + vm.Project,
				"--zone=" + vm.Zone,
				vm.Name,
				"--format=json",
			})
		// Sometimes we see errors about running out of CPU quota or IP addresses,
		// Back off and retry in these cases, just like CreateInstance().
		if err != nil && !strings.Contains(err.Error(), "Quota") {
			err = backoff.Permanent(err)
		}
		// Returning a non-permanent error triggers retries.
		return err
	}
	backoffPolicy := backoff.WithContext(backoff.NewConstantBackOff(time.Minute), ctx)
	if err := backoff.Retry(tryStart, backoffPolicy); err != nil {
		return err
	}

	ipAddress, err := extractIPAddress(output.Stdout)
	if err != nil {
		return err
	}
	vm.IPAddress = ipAddress

	return waitForStart(ctx, logger, vm)
}

// RestartInstance stops and starts the instance.
// It also waits for the instance to be started up post-restart.
func RestartInstance(ctx context.Context, logger *log.Logger, vm *VM) error {
	if err := StopInstance(ctx, logger, vm); err != nil {
		return fmt.Errorf("failed to stop instance: %w", err)
	}

	return StartInstance(ctx, logger, vm)
}

// InstallGsutilIfNeeded installs gsutil on instances that don't already have
// it installed. This is only currently the case for some old versions of SUSE.
func InstallGsutilIfNeeded(ctx context.Context, logger *log.Logger, vm *VM) error {
	if IsWindows(vm.Platform) {
		return nil
	}
	if _, err := RunRemotely(ctx, logger, vm, "", "sudo gsutil --version"); err == nil {
		// Success, no need to install gsutil.
		return nil
	}
	logger.Printf("gsutil not found, installing it...")

	// SUSE seems to be the only distro without gsutil, so what follows is all
	// very SUSE-specific.
	if !IsSUSE(vm.Platform) {
		return fmt.Errorf("this test does not know how to install gsutil on platform %q", vm.Platform)
	}

	gcloudArch := "x86_64"
	if IsARM(vm.Platform) {
		gcloudArch = "arm"
	}
	gcloudPkg := "google-cloud-cli-453.0.0-linux-" + gcloudArch + ".tar.gz"
	installFromTarball := `
curl -O https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/` + gcloudPkg + `
INSTALL_DIR="$(readlink --canonicalize .)"
(
	INSTALL_LOG="$(mktemp)"
	# This command produces a lot of console spam, so we only display that
	# output if there is a problem.
	sudo tar -xf ` + gcloudPkg + ` -C ${INSTALL_DIR} 
	sudo --preserve-env ${INSTALL_DIR}/google-cloud-sdk/install.sh -q &>"${INSTALL_LOG}" || \
		EXIT_CODE=$?
	if [[ "${EXIT_CODE-}" ]]; then
		cat "${INSTALL_LOG}"
		exit "${EXIT_CODE}"
	fi
)`
	installCmd := `set -ex
` + installFromTarball + `

# Upgrade to the latest version
sudo ${INSTALL_DIR}/google-cloud-sdk/bin/gcloud components update --quiet

sudo ln -s ${INSTALL_DIR}/google-cloud-sdk/bin/gsutil /usr/bin/gsutil 
`
	// b/308962066: The GCloud CLI ARM Linux tarballs do not have bundled Python
	// and the GCloud CLI requires Python >= 3.8. Install Python311 for ARM VMs
	if IsARM(vm.Platform) {
		// This is what's used on openSUSE.
		repoSetupCmd := "sudo zypper --non-interactive refresh"
		if strings.HasPrefix(vm.Platform, "sles-12") {
			return fmt.Errorf("this test does not know how to install gsutil on platform %q", vm.Platform)
		}
		// For SLES 15 ARM: use a vendored repo to reduce flakiness of the
		// external repos. See http://go/sdi/releases/build-test-release/vendored
		// for details.
		if strings.HasPrefix(vm.Platform, "sles-15") {
			repoSetupCmd = `sudo zypper --non-interactive addrepo -g -t YUM https://us-yum.pkg.dev/projects/cloud-ops-agents-artifacts-dev/google-cloud-monitoring-sles15-aarch64-test-vendor test-vendor
sudo rpm --import https://packages.cloud.google.com/yum/doc/yum-key.gpg https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg
sudo zypper --non-interactive refresh test-vendor`
		}

		installCmd = `set -ex
` + repoSetupCmd + `
sudo zypper --non-interactive install python311 python3-certifi

# On SLES 15 and OpenSUSE Leap arm, python3 is Python 3.6. Tell gsutil/gcloud to use python3.11.
export CLOUDSDK_PYTHON=/usr/bin/python3.11

` + installFromTarball + `

# Upgrade to the latest version
sudo CLOUDSDK_PYTHON=/usr/bin/python3.11 ${INSTALL_DIR}/google-cloud-sdk/bin/gcloud components update --quiet

# Make a "gsutil" bash script in /usr/bin that runs the copy of gsutil that
# was installed into $INSTALL_DIR with CLOUDSDK_PYTHON set.
sudo tee /usr/bin/gsutil > /dev/null << EOF
#!/usr/bin/env bash
CLOUDSDK_PYTHON=/usr/bin/python3.11 ${INSTALL_DIR}/google-cloud-sdk/bin/gsutil "\$@"
EOF
sudo chmod a+x /usr/bin/gsutil
`
	}

	_, err := RunRemotely(ctx, logger, vm, "", installCmd)
	return err
}

// instance is a subset of the official instance type from the GCE compute API
// documented here:
// http://cloud/compute/docs/reference/rest/v1/instances
type instance struct {
	ID                string
	NetworkInterfaces []struct {
		// This is the internal IP address.
		NetworkIP     string
		AccessConfigs []struct {
			// This is the external IP address.
			NatIP string
		}
	}
	Metadata struct {
		Items []struct {
			Key   string
			Value string
		}
	}
}

// extractSingleInstances parses the input serialized JSON description of a
// list of instances, and returns the only instance in the list. If the JSON
// parse fails or if there isn't exactly one instance in the list once it's
// been parsed, extractSingleInstance returns an error.
func extractSingleInstance(stdout string) (instance, error) {
	var instances []instance
	if err := json.Unmarshal([]byte(stdout), &instances); err != nil {
		return instance{}, fmt.Errorf("could not parse JSON from %q: %v", stdout, err)
	}
	if len(instances) != 1 {
		return instance{}, fmt.Errorf("should be exactly one instance in list. stdout: %q. Parsed result: %#v", stdout, instances)
	}
	return instances[0], nil
}

// extractIPAddress pulls the IP address out of the stdout from a gcloud
// create/start command with --format=json. By default it returns the external
// IP address, which is visible to entities outside the project the VM is
// running in. When USE_INTERNAL_IP is "true", this returns the internal IP
// address instead, which is what we need to use on Kokoro to satisfy the
// firewall settings set up for the project we use on Kokoro. Here is a
// drawing of my best understanding of the situation when trying to connect
// to VMs in various ways: http://go/sdi-testing-network-drawing
func extractIPAddress(stdout string) (string, error) {
	instance, err := extractSingleInstance(stdout)
	if err != nil {
		return "", err
	}

	if len(instance.NetworkInterfaces) == 0 {
		return "", fmt.Errorf("empty NetworkInterfaces list in %#v", instance)
	}

	if os.Getenv("USE_INTERNAL_IP") == "true" {
		internalIP := instance.NetworkInterfaces[0].NetworkIP
		if internalIP == "" {
			return "", fmt.Errorf("empty internal IP (networkInterfaces[0].NetworkIP) in instance %#v", instance)
		}
		return internalIP, nil
	}

	if len(instance.NetworkInterfaces[0].AccessConfigs) == 0 {
		return "", fmt.Errorf("empty NetworkInterfaces[0].AccessConfigs list in %#v", instance)
	}
	externalIP := instance.NetworkInterfaces[0].AccessConfigs[0].NatIP
	if externalIP == "" {
		return "", fmt.Errorf("empty external IP (networkInterfaces[0].AccessConfigs[0].NatIP) in instance %#v", instance)
	}
	return externalIP, nil
}

// ExtractID pulls the instance ID out of the stdout from a gcloud create/start
// command with --format=json.
func extractID(stdout string) (int64, error) {
	instance, err := extractSingleInstance(stdout)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(instance.ID, 10, 64)
}

// FetchMetadata retrieves the instance metadata for the given VM.
func FetchMetadata(ctx context.Context, logger *log.Logger, vm *VM) (map[string]string, error) {
	output, err := RunGcloud(ctx, logger, "", []string{
		"compute", "instances", "describe", vm.Name,
		"--project=" + vm.Project,
		"--zone=" + vm.Zone,
		"--format=json(metadata)",
	})
	if err != nil {
		return nil, fmt.Errorf("error fetching metadata for VM %v: %w", vm.Name, err)
	}
	var inst instance
	if err := json.Unmarshal([]byte(output.Stdout), &inst); err != nil {
		return nil, fmt.Errorf("could not parse JSON from %q: %v", output.Stdout, err)
	}
	metadata := make(map[string]string)
	for _, item := range inst.Metadata.Items {
		metadata[item.Key] = item.Value
	}
	return metadata, nil
}

const (
	// Retry errors that look like b/186426190.
	startupFailedMessage = "waitForStartLinux() failed: waiting for startup timed out"
	// Retry errors that look like b/305721001.
	windowsStartupFailedMessage = "waitForStartWindows() failed: ran out of attempts waiting for dummy command to run."
)

func waitForStartWindows(ctx context.Context, logger *log.Logger, vm *VM) error {
	// Make sure the server is really ready to run remote commands by
	// sending it a dummy command repeatedly until it works.
	attempt := 0
	printFoo := func() error {
		attempt++
		ctx, cancel := context.WithTimeout(ctx, vmInitPokeSSHTimeout)
		defer cancel()
		output, err := RunRemotely(ctx, logger, vm, "", "'foo'")
		logger.Printf("Printing 'foo' finished with err=%v, attempt #%d\noutput: %v",
			err, attempt, output)
		return err
	}

	backoffPolicy := backoff.WithContext(backoff.NewConstantBackOff(vmInitBackoffDuration), ctx)
	if err := backoff.Retry(printFoo, backoffPolicy); err != nil {
		return fmt.Errorf("%s err=%v", windowsStartupFailedMessage, err)
	}
	return nil
}

// waitForStartLinux waits for "systemctl is-system-running" to run over ssh and
// for it to reach "running", which indicates successful startup, or "degraded",
// which indicates startup has finished although with one or more services not
// initialized correctly. Historically "degraded" is usually still good enough
// to continue running the test.
func waitForStartLinux(ctx context.Context, logger *log.Logger, vm *VM) error {
	var backoffPolicy backoff.BackOff
	backoffPolicy = backoff.NewConstantBackOff(vmInitBackoffDuration)
	if IsSUSE(vm.Platform) {
		// Give up early on SUSE due to b/186426190. If this step times out, the
		// error will be retried with a fresh VM.
		backoffPolicy = backoff.WithMaxRetries(backoffPolicy, uint64((5*time.Minute)/vmInitBackoffDuration))
	}
	backoffPolicy = backoff.WithContext(backoffPolicy, ctx)

	// Returns an error if system startup is still ongoing.
	// Hopefully, waiting for system startup to finish will avoid some
	// hard-to-debug flaky issues like:
	// * b/180518814 (ubuntu, sles)
	// * b/148612123 (sles)
	isStartupDone := func() error {
		ctx, cancel := context.WithTimeout(ctx, vmInitPokeSSHTimeout)
		defer cancel()
		output, err := RunRemotely(ctx, logger, vm, "", "systemctl is-system-running")

		// There are a few cases for what is-system-running returns:
		// https://www.freedesktop.org/software/systemd/man/systemctl.html#is-system-running
		// If the command failed due to SSH issues, the stdout should be "".
		state := strings.TrimSpace(output.Stdout)
		if state == "running" {
			return nil
		}
		if state == "degraded" {
			// Even though some services failed to start, it's worth continuing
			// to run the test. There are various unnecessary services that could be
			// failing, see b/185473981 and b/185182238 for some examples.
			// But let's at least print out which services failed into the logs.
			RunRemotely(ctx, logger, vm, "", "systemctl --failed")
			return nil
		}
		// There are several reasons this could be failing, but usually if we get
		// here, that just means that ssh is not ready yet or the VM is in some
		// kind of non-ready state, like "starting".
		return err
	}

	if err := backoff.Retry(isStartupDone, backoffPolicy); err != nil {
		return fmt.Errorf("%v. Last err=%v", startupFailedMessage, err)
	}

	if IsSUSE(vm.Platform) {
		// TODO(b/259122953): SUSE needs additional startup time. Remove once we have more
		// sensible/deterministic workarounds for each of the individual problems.
		time.Sleep(slesStartupDelay)
		// TODO(b/259122953): wait until sudo is ready
		backoffPolicy := backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(slesStartupSudoDelay), slesStartupSudoMaxAttempts), ctx)
		err := backoff.Retry(func() error {
			_, err := RunRemotely(ctx, logger, vm, "", "sudo ls /root")
			return err
		}, backoffPolicy)
		if err != nil {
			return fmt.Errorf("exceeded retries trying to get sudo: %v", err)
		}
	}

	return nil
}

// waitForStart waits for the given VM to be ready to accept remote commands.
//
// Note that this does not mean that the VM is fully initialized. We don't have
// a good way to tell when the VM is fully initialized.
func waitForStart(ctx context.Context, logger *log.Logger, vm *VM) error {
	if IsWindows(vm.Platform) {
		return waitForStartWindows(ctx, logger, vm)
	}
	return waitForStartLinux(ctx, logger, vm)
}

// logLocation returns a string pointing to the test log. When this test is run
// on Kokoro, we can point at a URL in the cloud console, but if it's being run
// outside of Kokoro, we don't get to have a URL and have to point to a local
// path instead.
func logLocation(logRootDir, testName string) string {
	subdir := os.Getenv("KOKORO_BUILD_ARTIFACTS_SUBDIR")
	if subdir == "" {
		return path.Join(logRootDir, testName)
	}
	uploadLocation := os.Getenv("LOG_UPLOAD_URL_ROOT")
	if uploadLocation == "" {
		uploadLocation = "https://console.cloud.google.com/storage/browser/ops-agents-public-buckets-test-logs/"
	}
	return uploadLocation + path.Join(subdir, "logs", testName)
}

// SetupLogger creates a new DirectoryLogger that will write to a directory based on
// t.Name() inside the directory TEST_UNDECLARED_OUTPUTS_DIR.
// If creating the logger fails, it will abort the test.
// At the end of the test, the logger will be cleaned up.
// TODO: Move this function along with logLocation() into the agents package,
// since nothing else in this file depends on DirectoryLogger anymore.
func SetupLogger(t *testing.T) *logging.DirectoryLogger {
	t.Helper()
	name := strings.Replace(t.Name(), "/", "_", -1)

	logger, err := logging.NewDirectoryLogger(path.Join(logRootDir, name))
	if err != nil {
		t.Fatalf("SetupLogger() error creating DirectoryLogger: %v", err)
	}
	t.Cleanup(func() {
		if err := logger.Close(); err != nil {
			t.Errorf("SetupLogger() error closing DirectoryLogger: %v", err)
		}
	})
	logger.ToMainLog().Printf("Starting test %s", name)
	t.Logf("Test logs: %s ", logLocation(logRootDir, name))
	return logger
}

// VMOptions specifies settings when creating a VM via CreateInstance() or SetupVM().
type VMOptions struct {
	// Required. Normally passed as --image-family to
	// "gcloud compute images create".
	Platform string
	// Optional. If unspecified, 'Platform' must be specified.
	// Normally passed as --image to gcloud compute images create.
	Image string
	// Optional. Passed as --image-project to "gcloud compute images create".
	// If not supplied, the framework will attempt to guess the right project
	// to use based on Platform.
	ImageProject string
	// Optional. Set this to a duration like "3h" or "1d" to configure the VM to
	// be automatically deleted after the specified amount of time. This is
	// a recommended setting for short-lived VMs even if your code calls
	// DeleteInstance(), because this setting will take effect even if your code
	// crashes before calling DeleteInstance(), and besides DeleteInstance() can
	// fail. Calling DeleteInstance() is still recommended even if your code sets
	// a TimeToLive to free up VM resources as soon as possible.
	TimeToLive string
	// Optional. If missing, a random name will be generated.
	Name string
	// Optional. If missing, the environment variable PROJECT will be used.
	Project string
	// Optional. If missing, the environment variable ZONE will be used.
	Zone string
	// Optional.
	Metadata map[string]string
	// Optional.
	Labels map[string]string
	// Optional. If missing, the default is e2-standard-4.
	// Overridden by INSTANCE_SIZE if that environment variable is set.
	MachineType string
	// Optional. If missing, the default is 'global'.
	ImageFamilyScope string
	// Optional. If provided, these arguments are appended on to the end
	// of the "gcloud compute instances create" command.
	ExtraCreateArguments []string
}

// SetupVM creates a new VM according to the given options.
// If VM creation fails, it will abort the test.
// At the end of the test, the VM will be cleaned up.
func SetupVM(ctx context.Context, t *testing.T, logger *log.Logger, options VMOptions) *VM {
	t.Helper()

	vm, err := CreateInstance(ctx, logger, options)
	if err != nil {
		t.Fatalf("SetupVM() error creating instance: %v", err)
	}
	t.Cleanup(func() {
		if err := DeleteInstance(logger, vm); err != nil {
			t.Errorf("SetupVM() error deleting instance: %v", err)
		}
	})

	t.Logf("Instance Log: %v", instanceLogURL(vm))
	return vm
}

// RunForEachPlatform runs a subtest for each platform defined in PLATFORMS.
func RunForEachPlatform(t *testing.T, testBody func(t *testing.T, platform string)) {
	platformsEnv := os.Getenv("PLATFORMS")
	if platformsEnv == "" {
		t.Fatal("PLATFORMS env variable must be nonempty for RunForEachPlatform.")
	}
	platforms := strings.Split(platformsEnv, ",")
	for _, platform := range platforms {
		platform := platform // https://golang.org/doc/faq#closures_and_goroutines
		t.Run(platform, func(t *testing.T) {
			testBody(t, platform)
		})
	}
}

// ArbitraryPlatform picks an arbitrary element from PLATFORMS and returns it.
func ArbitraryPlatform() string {
	return strings.Split(os.Getenv("PLATFORMS"), ",")[0]
}

func areTagsValid(tags []string) (bool, error) {
	for _, tag := range tags {
		if strings.Contains(tag, ",") {
			return false, fmt.Errorf("Tag %v cannot contain comma.", tag)
		}
	}
	return true, nil
}

func AddTagToVm(ctx context.Context, logger *log.Logger, vm *VM, tags []string) (CommandOutput, error) {
	var output CommandOutput
	if valid, err := areTagsValid(tags); !valid {
		logger.Printf("Unable to add tag to VM: %v", err)
		return output, err
	}
	args := []string{
		"compute", "instances", "add-tags", vm.Name,
		"--zone=" + vm.Zone,
		"--project=" + vm.Project,
		"--tags=" + strings.Join(tags, ","),
	}
	output, err := RunGcloud(ctx, logger, "", args)
	if err != nil {
		logger.Printf("Unable to add tag to VM: %v", err)
		return output, err
	}
	return output, nil
}

func RemoveTagFromVm(ctx context.Context, logger *log.Logger, vm *VM, tags []string) (CommandOutput, error) {
	var output CommandOutput
	if valid, err := areTagsValid(tags); !valid {
		logger.Printf("Unable to remove tag from VM: %v", err)
		return output, err
	}
	args := []string{
		"compute", "instances", "remove-tags", vm.Name,
		"--zone=" + vm.Zone,
		"--project=" + vm.Project,
		"--tags=" + strings.Join(tags, ","),
	}

	output, err := RunGcloud(ctx, logger, "", args)
	if err != nil {
		logger.Printf("Unable remove tag from VM: %v", err)
		return output, err
	}
	return output, nil
}
