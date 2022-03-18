//go:build integration_test

/*
Package gce holds various helpers for testing the agents on GCE.

To run a test based on this library, you can either:

* use Kokoro by triggering automated presubmits on your change, or
* use "go test" directly, after performing the setup steps described
  in README.md.

NOTE: When testing Windows VMs without using Kokoro, PROJECT needs to be
    a project whose firewall allows WinRM connections.
    [Kokoro can use stackdriver-test-143416, which does not allow WinRM
    connections, because our Kokoro workers are also running in that project.]

NOTE: This command does not actually build the Ops Agent. To test the latest
    Ops Agent code, first build and upload a package to Rapture. Then look up
    the REPO_SUFFIX for that build and add it as an environment variable to the
    command below; for example: REPO_SUFFIX=20210805-2. You can also use
	AGENT_PACKAGES_IN_GCS, for details see README.md.

PROJECT=dev_project \
    ZONE=us-central1-b \
    PLATFORMS=debian-10,centos-8,rhel-8-1-sap-ha,sles-15,ubuntu-2004-lts,windows-2012-r2,windows-2019 \
    go test -v ops_agent_test.go \
	-test.parallel=1000 \
	-tags=integration_test \
    -timeout=4h

This library needs the following environment variables to be defined:

PROJECT: What GCP project to use.
ZONE: What GCP zone to run in.
WINRM_PAR_PATH: (required for Windows) Path to winrm.par, used to connect to
    Windows VMs.

The following variables are optional:

TEST_UNDECLARED_OUTPUTS_DIR: A path to a directory to write log files into.
    By default, a new temporary directory is created.
NETWORK_NAME: What GCP network name to use.
KOKORO_BUILD_ARTIFACTS_SUBDIR: supplied by Kokoro.
KOKORO_BUILD_ID: supplied by Kokoro.
USE_INTERNAL_IP: Whether to try to connect to the VMs' internal IP addresses
    (if set to "true"), or external IP addresses (in all other cases).
    Only useful on Kokoro.
SERVICE_EMAIL: If provided, which service account to use for spawned VMs. The
    default is the project's "Compute Engine default service account".
TRANSFERS_BUCKET: A GCS bucket name to use to transfer files to testing VMs.
    The default is "stackdriver-test-143416-file-transfers".
INSTANCE_SIZE: What size of VMs to make. Passed in to gcloud as --machine-type.
    If provided, this value overrides the selection made by the callers to
		this library.
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
	"cloud.google.com/go/storage"
	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"go.uber.org/multierr"
	"golang.org/x/text/encoding/unicode"
	"google.golang.org/api/iterator"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

var (
	storageClient   *storage.Client
	transfersBucket string

	monClient  *monitoring.MetricClient
	logClients *logClientFactory

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
)

const (
	// SuggestedTimeout is a recommended limit on how long a test should run before it is cancelled.
	// This cancellation does not happen automatically; each TestFoo() function must explicitly call
	// context.WithTimeout() to enable a timeout. It's a good idea to do this so that if a command
	// hangs, the test still will be cancelled eventually and all its VMs will be cleaned up.
	// This amount needs to be less than 4 hours, which is the limit on how long a Kokoro build can
	// take before it is forcibly killed.
	SuggestedTimeout = 2 * time.Hour

	// QueryMaxAttempts is the default number of retries when calling WaitForLog.
	// Retries are spaced by 5 seconds, so 80 retries denotes 6 minutes 40 seconds total.
	QueryMaxAttempts              = 80 // 6 minutes 40 seconds total.
	queryMaxAttemptsMetricMissing = 5  // 25 seconds total.
	queryBackoffDuration          = 5 * time.Second

	vmInitTimeout         = 20 * time.Minute
	vmInitBackoffDuration = 10 * time.Second

	sshUserName = "test_user"
)

func init() {
	ctx := context.Background()
	var err error

	if strings.Contains(os.Getenv("PLATFORMS"), "windows") && os.Getenv("WINRM_PAR_PATH") == "" {
		log.Fatal("WINRM_PAR_PATH must be nonempty when testing Windows VMs")
	}

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
	// Some useful options to pass to gcloud.
	os.Setenv("CLOUDSDK_PYTHON", "/usr/bin/python3")
	os.Setenv("CLOUDSDK_CORE_DISABLE_PROMPTS", "1")

	keysDir, err = os.MkdirTemp("", "ssh_keys")
	if err != nil {
		log.Fatalf("init() failed to make a temporary directory for ssh keys: %v", err)
	}
	privateKeyFile = filepath.Join(keysDir, "gce_testing_key")
	if _, err := runCommand(ctx, log.Default(), "", []string{"ssh-keygen", "-t", "rsa", "-f", privateKeyFile, "-C", sshUserName, "-N", ""}); err != nil {
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

// WindowsCredentials is a low-security way to hold login credentials for
// a Windows VM.
type WindowsCredentials struct {
	Username string
	Password string
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
	// The IP address to ssh/WinRM to. This is the external IP address, unless
	// USE_INTERNAL_IP is set to 'true'. See comment on extractIPAddress() for
	// rationale.
	IPAddress string
	// WindowsCredentials is only populated for Windows VMs.
	WindowsCredentials *WindowsCredentials
	AlreadyDeleted     bool
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
		// "sles-15" and "sles-15-sp1-sap".
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

// winRM() returns the path to the winrm.par binary to use to connect to
// Windows VMs.
func winRM() string {
	return os.Getenv("WINRM_PAR_PATH")
}

// IsWindows returns whether the given platform is a version of Windows (including Microsoft SQL Server).
func IsWindows(platform string) bool {
	return strings.HasPrefix(platform, "windows-") || strings.HasPrefix(platform, "sql-")
}

// isRetriableLookupMetricError returns whether the given error, returned from
// lookupMetric() or WaitForMetric(), should be retried.
func isRetriableLookupMetricError(err error) bool {
	myStatus, ok := status.FromError(err)
	// workload.googleapis.com/* domain metrics are created on first write, and may not be immediately queryable.
	// The error doesn't always look the same, hopefully looking for Code() == NotFound will catch all variations.
	// The Internal case catches some transient errors returned by the monitoring API sometimes.
	return ok && (myStatus.Code() == codes.NotFound || myStatus.Code() == codes.Internal)
}

// lookupMetric does a single lookup of the given metric in the backend.
func lookupMetric(ctx context.Context, logger *log.Logger, vm *VM, metric string, window time.Duration, extraFilters []string) *monitoring.TimeSeriesIterator {
	now := time.Now()
	start := timestamppb.New(now.Add(-window))
	end := timestamppb.New(now)
	filters := []string{
		fmt.Sprintf("metric.type = %q", metric),
		fmt.Sprintf(`resource.labels.instance_id = "%d"`, vm.ID),
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

// hasNonEmptySeries examines the given iterator, returning true if the
// lookup succeeded and returned a nonempty time series, and false otherwise.
// Also returns an error if the lookup failed.
// A return value of (false, nil) indicates that the lookup succeeded but
// returned no data.
func hasNonEmptySeries(logger *log.Logger, it *monitoring.TimeSeriesIterator) (bool, error) {
	// Loop through the iterator, looking for at least one nonempty time series.
	for {
		series, err := it.Next()
		logger.Printf("hasNonEmptySeries() iterator supplied err %v and series %v", err, series)
		if err == iterator.Done {
			// Either there were no data series in the iterator or all of them were empty.
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if len(series.Points) == 0 {
			// Look at the next element(s) of the iterator.
			continue
		}
		// Success, we found a timeseries with len(series.Points) > 0.
		return true, nil
	}
}

// WaitForMetric looks for the given metric in the backend and returns an error
// if it does not have data. This function will retry "no data" errors a fixed
// number of times. This is useful because it takes time for monitoring data to
// become visible after it has been uploaded.
func WaitForMetric(ctx context.Context, logger *log.Logger, vm *VM, metric string, window time.Duration, extraFilters []string) error {
	for attempt := 1; attempt <= QueryMaxAttempts; attempt++ {
		it := lookupMetric(ctx, logger, vm, metric, window, extraFilters)
		found, err := hasNonEmptySeries(logger, it)
		if found {
			// Success.
			return nil
		}
		if err != nil && !isRetriableLookupMetricError(err) {
			return fmt.Errorf("WaitForMetric(metric=%q, extraFilters=%v): %v", metric, extraFilters, err)
		}
		// We can get here in two cases:
		// 1. the lookup succeeded but found no data
		// 2. the lookup hit a retriable error. This case happens very rarely.
		logger.Printf("hasNonEmptySeries check(metric=%q, extraFilters=%v): request_error=%v, found-data=%v, retrying (%d/%d)...",
			metric, extraFilters, err, found, attempt, QueryMaxAttempts)
		time.Sleep(queryBackoffDuration)
	}
	return fmt.Errorf("WaitForMetric(metric=%s, extraFilters=%v) failed: exhausted retries", metric, extraFilters)
}

// AssertMetricMissing looks for data of a metric and returns success if
// no data is found. To consider possible transient errors while querying
// the backend we make queryMaxAttemptsMetricMissing query attempts.
func AssertMetricMissing(ctx context.Context, logger *log.Logger, vm *VM, metric string, window time.Duration) error {
	for attempt := 1; attempt <= queryMaxAttemptsMetricMissing; attempt++ {
		it := lookupMetric(ctx, logger, vm, metric, window, nil)
		found, err := hasNonEmptySeries(logger, it)
		logger.Printf("hasNonEmptySeries check(metric=%q): err=%v, found=%v, attempt (%d/%d)",
			metric, err, found, attempt, queryMaxAttemptsMetricMissing)

		if err == nil {
			if found {
				return fmt.Errorf("AssertMetricMissing(metric=%q): %v failed: unexpectedly found data for metric", metric, err)
			}
			// Success
			return nil
		}
		if !isRetriableLookupMetricError(err) {
			return fmt.Errorf("AssertMetricMissing(metric=%q): %v", metric, err)
		}
		time.Sleep(queryBackoffDuration)
	}
	return fmt.Errorf("AssertMetricMissing(metric=%q): failed: no successful queries to the backend", metric)
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
		first = entry
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

// CommandOutput holds the textual output from running a subprocess.
type CommandOutput struct {
	Stdout string
	Stderr string
}

// runCommand invokes a binary and waits until it finishes. Returns the combined stdout
// and stderr in a single string, and an error if the binary had a nonzero
// exit code.
// args is a slice containing the binary to invoke along with all its arguments,
// e.g. {"echo", "hello"}.
func runCommand(ctx context.Context, logger *log.Logger, stdin string, args []string) (CommandOutput, error) {
	var output CommandOutput
	if len(args) < 1 {
		return output, fmt.Errorf("runCommand() needs a nonempty argument slice, got %v", args)
	}
	if !strings.HasSuffix(args[0], "winrm.par") {
		// Print out the command we're running. Skip this for winrm.par commands
		// because they are base64 encoded and the real command is already printed
		// inside runRemotelyWindows() anyway.
		logger.Printf("Running command: %v", args)
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return output, fmt.Errorf("runCommand() failed to open a pipe to stdin: %v", err)
	}

	if _, err = io.WriteString(stdinPipe, stdin); err != nil {
		return output, fmt.Errorf("runCommand() failed to write to stdin: %v", err)
	}

	if err = stdinPipe.Close(); err != nil {
		return output, fmt.Errorf("runCommand() failed to close stdin: %v", err)
	}

	var stdoutBuilder strings.Builder
	var stderrBuilder strings.Builder

	cmd.Stdout = &stdoutBuilder
	cmd.Stderr = &stderrBuilder

	if err = cmd.Run(); err != nil {
		err = fmt.Errorf("Command failed: %v\n%v\nstdout: %s\nstderr: %s", args, err, stdoutBuilder.String(), stderrBuilder.String())
	}

	logger.Printf("exit code: %v", cmd.ProcessState.ExitCode())
	output.Stdout = stdoutBuilder.String()
	output.Stderr = stderrBuilder.String()

	logger.Printf("stdout: %s", output.Stdout)
	logger.Printf("stderr: %s", output.Stderr)

	return output, err
}

// RunGcloud invokes a gcloud binary from runfiles and waits until it finishes.
// Returns the combined stdout and stderr in a single string, and an error if
// the binary had a nonzero exit code.
// args is a slice containing the arguments to pass to gcloud.
//
// Note: most calls to this function could be replaced by calls to the Compute API
// (https://cloud.google.com/compute/docs/reference/rest/v1).
// Various pros/cons of shelling out to gcloud vs using the Compute API are dicussed here:
// http://go/sdi-gcloud-vs-api
func RunGcloud(ctx context.Context, logger *log.Logger, stdin string, args []string) (CommandOutput, error) {
	return runCommand(ctx, logger, stdin, append([]string{gcloudPath}, args...))
}

// runRemotelyWindows runs the provided powershell command on the provided Windows VM.
// The command is base64 encoded in transit because that is an effective way to run
// complex commands, such as commands with nested quoting.
func runRemotelyWindows(ctx context.Context, logger *log.Logger, vm *VM, command string) (CommandOutput, error) {
	logger.Printf("Running command %q", command)

	uni := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	encoded, err := uni.NewEncoder().String(command)
	if err != nil {
		return CommandOutput{}, err
	}
	return runCommand(ctx, logger, "",
		[]string{winRM(),
			"--host=" + vm.IPAddress,
			"--username=" + vm.WindowsCredentials.Username,
			"--password=" + vm.WindowsCredentials.Password,
			fmt.Sprintf("--command=powershell -NonInteractive -encodedcommand %q", base64.StdEncoding.EncodeToString([]byte(encoded))),
			"--stderrthreshold=fatal",
			"--verbosity=-2",
		})
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
	}
)

// RunRemotely runs a command on the provided VM.
// The command should be a shell command if the VM is Linux, or powershell if the VM is Windows.
// Returns the combined stdout+stderr as a string, plus an error if there was
// a problem.
//
// 'command' is what to run on the machine. Example: "cat /tmp/foo; echo hello"
// 'stdin' is what to supply to the command on stdin. It is usually "".
// TODO: Remove the stdin parameter, because it is hardly used and doesn't work
//     on Windows.
func RunRemotely(ctx context.Context, logger *log.Logger, vm *VM, stdin string, command string) (_ CommandOutput, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("Command failed: %v\n%v", command, err)
		}
	}()
	if IsWindows(vm.Platform) {
		if stdin != "" {
			// TODO(martijnvs): Support stdin on Windows, if we see a need for it.
			return CommandOutput{}, errors.New("RunRemotely() does not support stdin when run on Windows")
		}
		return runRemotelyWindows(ctx, logger, vm, command)
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
	args = append(args, command)
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
func UploadContent(ctx context.Context, dirLog *logging.DirectoryLogger, vm *VM, content io.Reader, remotePath string) (err error) {
	defer func() {
		dirLog.ToMainLog().Printf("Uploading file finished. For details see file_uploads.txt. err=%v", err)
	}()
	logger := dirLog.ToFile("file_uploads.txt")
	object := storageClient.Bucket(transfersBucket).Object(path.Join(vm.Name, remotePath))
	writer := object.NewWriter(ctx)
	_, copyErr := io.Copy(writer, content)
	// We have to make sure to call Close() here in order to tell it to finish
	// the upload operation.
	closeErr := writer.Close()
	logger.Printf("Upload to %v finished with copyErr=%v, closeErr=%v", object, copyErr, closeErr)
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
		logger.Printf("Deleting %v finished with deleteErr=%v", object, deleteErr)
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
func RunScriptRemotely(ctx context.Context, logger *logging.DirectoryLogger, vm *VM, scriptContents string, flags []string, env map[string]string) (CommandOutput, error) {
	var quotedFlags []string
	for _, flag := range flags {
		quotedFlags = append(quotedFlags, fmt.Sprintf("'%s'", flag))
	}
	flagsStr := strings.Join(quotedFlags, " ")

	if IsWindows(vm.Platform) {
		if err := UploadContent(ctx, logger, vm, strings.NewReader(scriptContents), "C:\\temp.ps1"); err != nil {
			return CommandOutput{}, err
		}
		return RunRemotely(ctx, logger.ToMainLog(), vm, "", envVarMapToPowershellPrefix(env)+"powershell -File C:\\temp.ps1 "+flagsStr)
	}
	// Write the script contents to script.sh, then tell bash to execute it with -x
	// to print each line as it runs.
	return RunRemotely(ctx, logger.ToMainLog(), vm, scriptContents, "cat - > script.sh && sudo "+envVarMapToBashPrefix(env)+"bash -x script.sh "+flagsStr)
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
		_, err := RunRemotely(ctx, logger, vm, "", "sudo /usr/sbin/registercloudguest")
		return err
	}, backoffPolicy)
	if err != nil {
		RunRemotely(ctx, logger, vm, "", "sudo cat /var/log/cloudregister")
		return fmt.Errorf("error running registercloudguest: %v", err)
	}

	backoffPolicy = backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(5*time.Second), 120), ctx) // 10 minutes max.
	err = backoff.Retry(func() error {
		// timezone-java was selected arbitrarily as a package that:
		// a) can be installed from the default repos, and
		// b) isn't installed already.
		_, zypperErr := RunRemotely(ctx, logger, vm, "", "sudo zypper refresh && sudo zypper -n install timezone-java")
		return zypperErr
	}, backoffPolicy)
	if err != nil {
		RunRemotely(ctx, logger, vm, "", "sudo cat /var/log/zypper.log")
	}
	return err
}

var (
	overriddenImages = map[string]string{
		"opensuse-leap-15-2": "opensuse-leap-15-2-v20200702",
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

	if IsWindows(platform) {
		if _, ok := metadataCopy["windows-startup-script-ps1"]; ok {
			return nil, errors.New("you cannot pass a startup script for Windows instances because the startup script is used to detect that the instance is running. Instead, wait for the instance to be ready and then run things with RunRemotely() or RunScriptRemotely()")
		}
		metadataCopy["windows-startup-script-ps1"] = `
Enable-PSRemoting  # Might help to diagnose b/185923886.

$port = new-Object System.IO.Ports.SerialPort 'COM3'
$port.Open()
$port.WriteLine("STARTUP_SCRIPT_DONE")
$port.Close()
`
	} else {
		if _, ok := metadataCopy["startup-script"]; ok {
			return nil, errors.New("the 'startup-script' metadata key is reserved for future use. Instead, wait for the instance to be ready and then run things with RunRemotely() or RunScriptRemotely()")
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

	// Attach labels to automate cleanup
	labelsCopy["env"] = "test"
	labelsCopy["ttl"] = "180" // minutes

	return labelsCopy, nil
}

// attemptCreateInstance creates a VM instance and waits for it to be ready.
// Returns a VM object or an error (never both). The caller is responsible for
// deleting the VM if (and only if) the returned error is nil.
func attemptCreateInstance(ctx context.Context, logger *log.Logger, options VMOptions) (vmToReturn *VM, errToReturn error) {
	vm := &VM{
		Project:  options.Project,
		Platform: options.Platform,
		Network:  os.Getenv("NETWORK_NAME"),
		Zone:     options.Zone,
	}
	if vm.Project == "" {
		vm.Project = os.Getenv("PROJECT")
	}
	if vm.Network == "" {
		vm.Network = "default"
	}
	if vm.Zone == "" {
		vm.Zone = os.Getenv("ZONE")
	}
	// Note: INSTANCE_SIZE takes precedence over options.MachineType.
	vm.MachineType = os.Getenv("INSTANCE_SIZE")
	if vm.MachineType == "" {
		vm.MachineType = options.MachineType
	}
	if vm.MachineType == "" {
		vm.MachineType = "e2-standard-4"
	}
	// The VM name needs to adhere to these restrictions:
	// https://cloud.google.com/compute/docs/naming-resources#resource-name-format
	vm.Name = fmt.Sprintf("%s-%s", sandboxPrefix, uuid.New())

	imgProject, err := imageProject(vm.Platform)
	if err != nil {
		return nil, fmt.Errorf("attemptCreateInstance() could not find image project: %v", err)
	}
	newMetadata, err := addFrameworkMetadata(vm.Platform, options.Metadata)
	if err != nil {
		return nil, fmt.Errorf("attemptCreateInstance() could not construct valid metadata: %v", err)
	}
	newLabels, err := addFrameworkLabels(options.Labels)
	if err != nil {
		return nil, fmt.Errorf("attemptCreateInstance() could not construct valid labels: %v", err)
	}

	imageOrImageFamilyFlag := "--image-family=" + vm.Platform
	if image, ok := overriddenImages[vm.Platform]; ok {
		imageOrImageFamilyFlag = "--image=" + image
	}
	args := []string{
		"compute", "instances", "create", vm.Name,
		"--project=" + vm.Project,
		"--zone=" + vm.Zone,
		"--machine-type=" + vm.MachineType,
		"--image-project=" + imgProject,
		imageOrImageFamilyFlag,
		"--image-family-scope=global",
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

	if isSUSE(vm.Platform) {
		// Set download.max_silent_tries to 5 (by default, it is commented out in
		// the config file). This should help with issues like b/211003972.
		_, err := RunRemotely(ctx, logger, vm, "", "sudo sed -i -E 's/.*download.max_silent_tries.*/download.max_silent_tries = 5/g' /etc/zypp/zypp.conf")
		if err != nil {
			return nil, fmt.Errorf("attemptCreateInstance() failed to configure retries in zypp.conf: %v", err)
		}
	}

	if strings.HasPrefix(vm.Platform, "sles-") {
		if err := prepareSLES(ctx, logger, vm); err != nil {
			return nil, fmt.Errorf("%s: %v", prepareSLESMessage, err)
		}
	}

	return vm, nil
}

func isSUSE(platform string) bool {
	return strings.HasPrefix(platform, "sles-") || strings.HasPrefix(platform, "opensuse-")
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
			// Windows instances sometimes fail to initialize WinRM: b/185923886.
			strings.Contains(err.Error(), winRMDummyCommandMessage) ||
			// SLES instances sometimes fail to be ssh-able: b/186426190
			(isSUSE(options.Platform) && strings.Contains(err.Error(), startupFailedMessage)) ||
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
	defaultEnvironment := "DefaultEnvironment="
	for key, value := range envVariables {
		defaultEnvironment += fmt.Sprintf(`"%s=%s" `, key, value)
	}
	cmd := fmt.Sprintf("echo '%s' | sudo tee -a /etc/systemd/system.conf", defaultEnvironment)
	logger.Println("edit system.conf command: " + cmd)
	if _, err := RunRemotely(ctx, logger, vm, "", cmd); err != nil {
		return err
	}
	// Reload the systemd daemon to pick up the new settings from system.conf edited in the previous command
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	defer cancel()
	_, err := RunGcloud(ctx, logger, "",
		[]string{
			"compute",
			"instances",
			"delete",
			"--project=" + vm.Project,
			"--zone=" + vm.Zone,
			vm.Name,
		})
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
// Also waits for the instance to be reachable over ssh.
func StartInstance(ctx context.Context, logger *log.Logger, vm *VM) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute*20)
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
	if !isSUSE(vm.Platform) {
		return fmt.Errorf("this test does not know how to install gsutil on platform %q", vm.Platform)
	}

	// This is what's used on openSUSE.
	repoSetupCmd := "sudo zypper --non-interactive refresh"

	if strings.HasPrefix(vm.Platform, "sles-") {
		// Use a vendored repo to reduce flakiness of the external repos.
		// See http://go/sdi/releases/build-test-release/vendored for details.
		repo := "google-cloud-monitoring-sles12-x86_64-test-vendor"
		if strings.HasPrefix(vm.Platform, "sles-15") {
			repo = "google-cloud-monitoring-sles15-x86_64-test-vendor"
		}
		repoSetupCmd = `sudo zypper --non-interactive addrepo -g -t YUM https://packages.cloud.google.com/yum/repos/` + repo + ` test-vendor
sudo rpm --import https://packages.cloud.google.com/yum/doc/yum-key.gpg https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg

sudo zypper --non-interactive refresh test-vendor`
	}

	installCmd := `set -ex

` + repoSetupCmd + `
sudo zypper --non-interactive install --capability 'python>=3.6'
sudo zypper --non-interactive install python3-certifi

# On SLES 12, python3 is Python 3.4. Tell gsutil/gcloud to use python3.6.
export CLOUDSDK_PYTHON=/usr/bin/python3.6

# Install gcloud (https://cloud.google.com/sdk/docs/downloads-interactive).
curl -o install.sh https://sdk.cloud.google.com
INSTALL_DIR="$(readlink --canonicalize .)"
(
		INSTALL_LOG="$(mktemp)"
    # This command produces a lot of console spam, so we only display that
    # output if there is a problem.
    sudo --preserve-env bash install.sh --disable-prompts --install-dir="${INSTALL_DIR}" &>"${INSTALL_LOG}" || \
      EXIT_CODE=$?
    if [[ "${EXIT_CODE-}" ]]; then
      cat "${INSTALL_LOG}"
      exit "${EXIT_CODE}"
    fi
)

# Make a "gsutil" bash script in /usr/bin that runs the copy of gsutil that
# was installed into $INSTALL_DIR with CLOUDSDK_PYTHON set.
sudo tee /usr/bin/gsutil > /dev/null << EOF
#!/usr/bin/env bash
CLOUDSDK_PYTHON=/usr/bin/python3.6 ${INSTALL_DIR}/google-cloud-sdk/bin/gsutil "\$@"
EOF
sudo chmod a+x /usr/bin/gsutil
`
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

func resetAndFetchWindowsCredentials(ctx context.Context, logger *log.Logger, vm *VM) (*WindowsCredentials, error) {
	output, err := RunGcloud(ctx, logger, "",
		[]string{"compute", "reset-windows-password", vm.Name,
			// The username can be anything; it just has to comply with the requirements here:
			// https://docs.microsoft.com/en-us/windows/win32/api/lmaccess/nf-lmaccess-netuseradd
			"--user=windows_user",
			"--project=" + vm.Project,
			"--zone=" + vm.Zone,
			"--format=json",
		})
	if err != nil {
		return nil, fmt.Errorf("failed to reset Windows password: %v", err)
	}
	var creds WindowsCredentials
	if err := json.Unmarshal([]byte(output.Stdout), &creds); err != nil {
		return nil, fmt.Errorf("could not parse JSON for %q: %v", output.Stdout, err)
	}
	if creds.Username == "" || creds.Password == "" {
		return nil, fmt.Errorf("username or password was empty when parsing %q. Parsed result: %#v", output.Stdout, creds)
	}
	return &creds, nil
}

const (
	// Retry errors that look like b/185923886.
	winRMDummyCommandMessage = "waitForStartWindows() failed: dummy command could not run over WinRM"

	// Retry errors that look like b/186426190.
	startupFailedMessage = "waitForStartLinux() failed: waiting for startup timed out"
)

func waitForStartWindows(ctx context.Context, logger *log.Logger, vm *VM) error {
	lookForReadyMessages := func() error {
		output, err := RunGcloud(ctx, logger, "", []string{
			"compute", "instances", "get-serial-port-output",
			"--port=3",
			"--project=" + vm.Project,
			"--zone=" + vm.Zone,
			vm.Name})
		if err != nil {
			return fmt.Errorf("error getting COM3 serial port output: %v", err)
		}
		if strings.Contains(output.Stdout, "STARTUP_SCRIPT_DONE") {
			// Success.
			return nil
		}
		return fmt.Errorf("STARTUP_SCRIPT_DONE not found in serial port output: %v", output)
	}
	backoffPolicy := backoff.WithContext(backoff.NewConstantBackOff(vmInitBackoffDuration), ctx)
	if err := backoff.Retry(lookForReadyMessages, backoffPolicy); err != nil {
		return fmt.Errorf("ran out of attempts waiting for VM to initialize: %v", err)
	}

	creds, err := resetAndFetchWindowsCredentials(ctx, logger, vm)
	if err != nil {
		return fmt.Errorf("resetAndFetchWindowsCredentials() failed: %v", err)
	}
	vm.WindowsCredentials = creds

	// Now, make sure the server is really ready to run remote commands by
	// sending it a dummy command repeatedly until it works.
	attempt := 0
	printFoo := func() error {
		attempt++
		output, err := RunRemotely(ctx, logger, vm, "", "'foo'")
		logger.Printf("Printing 'foo' finished with err=%v, attempt #%d\noutput: %v",
			err, attempt, output)
		return err
	}

	gracePeriod := 3 * time.Minute // I'm not sure what's a good value here.
	maxAttempts := uint64(gracePeriod / vmInitBackoffDuration)
	backoffPolicy = backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(vmInitBackoffDuration), maxAttempts), ctx)
	if err := backoff.Retry(printFoo, backoffPolicy); err != nil {
		return fmt.Errorf("%v, even after %v of attempts. err=%v", winRMDummyCommandMessage, gracePeriod, err)
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
	if isSUSE(vm.Platform) {
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
		// "sudo" is needed for debian-9, which doesn't have dbus, so systemctl
		// needs to talk directly to systemd.
		output, err := RunRemotely(ctx, logger, vm, "", "sudo systemctl is-system-running")

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
	return "https://console.cloud.google.com/storage/browser/ops-agents-public-buckets-test-logs/" +
		path.Join(subdir, "logs", testName)
}

// SetupLogger creates a new DirectoryLogger that will write to a directory based on
// t.Name() inside the directory TEST_UNDECLARED_OUTPUTS_DIR.
// If creating the logger fails, it will abort the test.
// At the end of the test, the logger will be cleaned up.
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
	t.Logf("Test logs: %s", logLocation(logRootDir, name))
	return logger
}

// VMOptions specifies settings when creating a VM via CreateInstance() or SetupVM().
type VMOptions struct {
	// Required.
	Platform string
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
func RunForEachPlatform(t *testing.T, f func(t *testing.T, platform string)) {
	platforms := strings.Split(os.Getenv("PLATFORMS"), ",")
	for _, platform := range platforms {
		platform := platform // https://golang.org/doc/faq#closures_and_goroutines
		t.Run(platform, func(t *testing.T) {
			f(t, platform)
		})
	}
}

// ArbitraryPlatform picks an arbitrary element from PLATFORMS and returns it.
func ArbitraryPlatform() string {
	return strings.Split(os.Getenv("PLATFORMS"), ",")[0]
}
