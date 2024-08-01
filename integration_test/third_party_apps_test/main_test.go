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
Test for third-party app integrations. Can be run with Kokoro or "go test".
For instructions, see the top of gce_testing.go.

This test needs the following environment variables to be defined, in addition
to the ones mentioned at the top of gce_testing.go:

IMAGE_SPECS: a comma-separated list of image specs to test, e.g. "suse-cloud:sles-12,ubuntu-os-cloud:ubuntu-2404-amd64".

The following variables are optional:

AGENT_PACKAGES_IN_GCS: If provided, a URL for a directory in GCS containing
    .deb/.rpm/.goo files to install on the testing VMs.
REPO_SUFFIX: If provided, a package repository suffix to install the agent from.
    AGENT_PACKAGES_IN_GCS takes precedence over REPO_SUFFIX.
ARTIFACT_REGISTRY_REGION: If provided, signals to the install scripts that the
    above REPO_SUFFIX is an artifact registry repo and specifies what region it
    is in.
*/

package third_party_apps_test

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	cloudlogging "cloud.google.com/go/logging"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/agents"
	feature_tracking_metadata "github.com/GoogleCloudPlatform/ops-agent/integration_test/feature_tracking"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/gce"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/logging"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/metadata"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/util"
	structpb "google.golang.org/protobuf/types/known/structpb"

	"go.uber.org/multierr"
	"gopkg.in/yaml.v2"
)

var (
	packagesInGCS = os.Getenv("AGENT_PACKAGES_IN_GCS")
)

//go:embed applications
var scriptsDir embed.FS

var validate = metadata.NewIntegrationMetadataValidator()

// removeFromSlice returns a new []string that is a copy of the given []string
// with all occurrences of toRemove removed.
func removeFromSlice(original []string, toRemove string) []string {
	var result []string
	for _, elem := range original {
		if elem != toRemove {
			result = append(result, elem)
		}
	}
	return result
}

// assertFilePresence returns an error if the provided file path doesn't exist on the VM.
func assertFilePresence(ctx context.Context, logger *log.Logger, vm *gce.VM, filePath string) error {
	var fileQuery string
	if gce.IsWindows(vm.ImageSpec) {
		fileQuery = fmt.Sprintf(`Test-Path -Path "%s"`, filePath)
	} else {
		fileQuery = fmt.Sprintf(`sudo test -f %s`, filePath)
	}

	out, err := gce.RunRemotely(ctx, logger, vm, fileQuery)
	if err != nil {
		return fmt.Errorf("error accessing backup file: %v", err)
	}

	// Windows returns False if the path doesn't exist.
	if gce.IsWindows(vm.ImageSpec) && strings.Contains(out.Stdout, "False") {
		return fmt.Errorf("couldn't find file %s. Output response %s. Error response: %s", filePath, out.Stdout, out.Stderr)
	}

	return nil
}

const (
	retryable    = true
	nonRetryable = false
)

// distroFolder returns the distro family name we use in our directory hierarchy
// inside the scripts directory.
func distroFolder(vm *gce.VM) (string, error) {
	if gce.IsWindows(vm.ImageSpec) {
		return "windows", nil
	}
	if gce.IsSUSEVM(vm) {
		return "sles", nil
	}
	switch vm.OS.ID {
	case "centos", "rhel", "rocky":
		return "centos_rhel", nil
	case "debian", "ubuntu":
		return "debian_ubuntu", nil
	default:
		return "", fmt.Errorf("distroFolder() could not find matching folder holding scripts for vm.OS.ID: %s", vm.OS.ID)
	}
}

// runScriptFromScriptsDir runs a script on the given VM.
// The scriptPath should be relative to SCRIPTS_DIR.
// The script should be a shell script for a Linux VM and powershell for a Windows VM.
// env is a map containing environment variables to provide to the script as it runs.
func runScriptFromScriptsDir(ctx context.Context, logger *log.Logger, vm *gce.VM, scriptPath string, env map[string]string) (gce.CommandOutput, error) {
	logger.Printf("Running script with path %s", scriptPath)

	scriptContents, err := scriptsDir.ReadFile(scriptPath)
	if err != nil {
		return gce.CommandOutput{}, err
	}
	return gce.RunScriptRemotely(ctx, logger, vm, string(scriptContents), nil, env)
}

// updateSSHKeysForActiveDirectory alters the ssh-keys metadata value for the
// given VM by prepending the given domain and a backslash onto the username.
func updateSSHKeysForActiveDirectory(ctx context.Context, logger *log.Logger, vm *gce.VM, domain string) error {
	metadata, err := gce.FetchMetadata(ctx, logger, vm)
	if err != nil {
		return err
	}
	if _, err = gce.RunGcloud(ctx, logger, "", []string{
		"compute", "instances", "add-metadata", vm.Name,
		"--project=" + vm.Project,
		"--zone=" + vm.Zone,
		"--metadata=ssh-keys=" + domain + `\` + metadata["ssh-keys"],
	}); err != nil {
		return fmt.Errorf("error setting new ssh keys metadata for vm %v: %w", vm.Name, err)
	}
	return nil
}

// constructQuery converts the given struct of:
//
//	field name => field value regex
//
// into a query filter to pass to the logging API.
func constructQuery(logName string, fields []*metadata.LogFields) string {
	var parts []string
	for _, field := range fields {
		if field.ValueRegex != "" {
			parts = append(parts, fmt.Sprintf(`%s=~"%s"`, field.Name, field.ValueRegex))
		}
	}

	if logName != "syslog" {
		// verify instrumentation_source label
		val := fmt.Sprintf("agent.googleapis.com/%s", logName)
		parts = append(parts, fmt.Sprintf(`%s=%s`, `labels."logging.googleapis.com/instrumentation_source"`, val))
	}

	return strings.Join(parts, " AND ")
}

// logFieldsMapWithPrefix returns a field name => LogField mapping where all the fieldnames have the provided prefix.
// Note that the map will omit the prefix in the returned map.
func logFieldsMapWithPrefix(log *metadata.ExpectedLog, prefix string) map[string]*metadata.LogFields {
	if log == nil {
		return nil
	}

	fieldsMap := make(map[string]*metadata.LogFields)
	for _, entry := range log.Fields {
		if strings.HasPrefix(entry.Name, prefix) {
			fieldsMap[strings.TrimPrefix(entry.Name, prefix)] = entry
		}
	}

	return fieldsMap
}

// verifyLogField verifies that the actual field retrieved from Cloud Logging is as expected.
func verifyLogField(fieldName, actualField string, expectedFields map[string]*metadata.LogFields) error {
	expectedField, ok := expectedFields[fieldName]
	if !ok {
		// Not expecting this field. It could however be populated with some default zero-values when we
		// query it back. Check for zero values based on expectedField.type? Not ideal for sure.
		if actualField != "" && actualField != "0" && actualField != "false" && actualField != "0s" {
			return fmt.Errorf("got %q for unexpected field %s\n", actualField, fieldName)
		}
		return nil
	}

	if len(actualField) == 0 {
		if expectedField.Optional {
			return nil
		} else {
			return fmt.Errorf("expected non-empty value for log field %s\n", fieldName)
		}
	}

	// The (?s) part will make the . match with newline as well. See https://github.com/google/re2/blob/main/doc/syntax.txt#L65,L68
	pattern := "(?s).*"
	if expectedField.ValueRegex != "" {
		pattern = expectedField.ValueRegex
	}
	match, err := regexp.MatchString(pattern, actualField)
	if err != nil {
		return err
	}

	if !match {
		return fmt.Errorf("field %s of the actual log: %s didn't match regex pattern: %s\n", fieldName, actualField, pattern)
	}

	return nil
}

// verifyJsonPayload verifies that the jsonPayload component of the LogEntry is as expected.
// TODO: We don't unpack the jsonPayload and assert that the nested substructure is as expected.
//
//	The way we could do this is flatten the nested payload into a single layer (using something like https://github.com/jeremywohl/flatten)
//	and then verifying the fields against the expected fields.
//
// This should be added if some of the integrations expect to create nested fields.
func verifyJsonPayload(actualPayload interface{}, expectedPayload map[string]*metadata.LogFields) error {
	var multiErr error
	actualPayloadFields := actualPayload.(*structpb.Struct).GetFields()
	for expectedKey, expectedValue := range expectedPayload {
		actualValue, ok := actualPayloadFields[expectedKey]
		if !ok || actualValue == nil {
			if !expectedValue.Optional {
				multiErr = multierr.Append(multiErr, fmt.Errorf("expected values for field jsonPayload.%s but got nil\n", expectedKey))
			}

			continue
		}

		// Sanitize the actualValue string.
		// TODO: Assert that the types are what we expect them to be. Left for anther day.
		var actualValueStr string
		switch v := actualValue.GetKind().(type) {
		case *structpb.Value_NumberValue:
			if v != nil {
				switch {
				case math.IsNaN(v.NumberValue):
					actualValueStr = "NaN"
				case math.IsInf(v.NumberValue, +1):
					actualValueStr = "Infinity"
				case math.IsInf(v.NumberValue, -1):
					actualValueStr = "-Infinity"
				default:
					actualValueStr = strconv.FormatFloat(v.NumberValue, 'E', -1, 32)
				}
			}
		case *structpb.Value_StringValue:
			if v != nil {
				actualValueStr = v.StringValue
			}
		case *structpb.Value_BoolValue:
			if v != nil {
				actualValueStr = strconv.FormatBool(v.BoolValue)
			}
		case *structpb.Value_StructValue:
			if v != nil {
				actualValueStr = fmt.Sprint(v.StructValue.AsMap())
			}
		case *structpb.Value_ListValue:
			if v != nil {
				actualValueStr = fmt.Sprint(v.ListValue.AsSlice())
			}
		}

		if err := verifyLogField(expectedKey, actualValueStr, expectedPayload); err != nil {
			multiErr = multierr.Append(multiErr, err)
		}
	}

	for actualKey, actualValue := range actualPayloadFields {
		if _, ok := expectedPayload[actualKey]; !ok {
			multiErr = multierr.Append(multiErr, fmt.Errorf("expected no value for field jsonPayload.%s but got %s\n", actualKey, actualValue.String()))
		}
	}

	return multiErr
}

// verifyLog returns an error if the actualLog has some fields that weren't expected as specified. Or if it is missing
// some required fields.
func verifyLog(actualLog *cloudlogging.Entry, expectedLog *metadata.ExpectedLog) error {
	var multiErr error
	if expectedLog.LogName == "syslog" {
		// If the application writes to syslog directly (for example: activemq), the log formats are sometimes different
		// per distro.
		return nil
	}

	// Verify all fields in the actualLog match some field in the expectedLog.
	expectedFields := logFieldsMapWithPrefix(expectedLog, "")

	// Severity
	if err := verifyLogField("severity", actualLog.Severity.String(), expectedFields); err != nil {
		multiErr = multierr.Append(multiErr, err)
	}

	// SourceLocation
	if actualLog.SourceLocation == nil {
		_, fileOk := expectedFields["sourceLocation.file"]
		_, lineOk := expectedFields["sourceLocation.line"]
		if fileOk || lineOk {
			multiErr = multierr.Append(multiErr, fmt.Errorf("expected sourceLocation.file and sourceLocation.line but got nil\n"))
		}
	} else {
		if err := verifyLogField("sourceLocation.file", actualLog.SourceLocation.File, expectedFields); err != nil {
			multiErr = multierr.Append(multiErr, err)
		}
		if err := verifyLogField("sourceLocation.line", strconv.FormatInt(actualLog.SourceLocation.Line, 10), expectedFields); err != nil {
			multiErr = multierr.Append(multiErr, err)
		}
	}

	// HTTP Request
	// Taken from https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#HttpRequest
	httpRequestFields := []string{"httpRequest.requestMethod", "httpRequest.requestUrl",
		"httpRequest.requestSize", "httpRequest.status", "httpRequest.responseSize",
		"httpRequest.userAgent", "httpRequest.remoteIp", "httpRequest.serverIp",
		"httpRequest.referer", "httpRequest.latency", "httpRequest.cacheLookup",
		"httpRequest.cacheHit", "httpRequest.cacheValidatedWithOriginServer",
		"httpRequest.cacheFillBytes", "httpRequest.protocol"}
	if actualLog.HTTPRequest == nil {
		for _, field := range httpRequestFields {
			if _, ok := expectedFields[field]; ok {
				multiErr = multierr.Append(multiErr, fmt.Errorf("expected value for field %s but got nil\n", field))
			}
		}
	} else {
		// Validate that the HTTP fields that are present match the expected log.
		testPairs := [][2]string{
			{"httpRequest.requestMethod", actualLog.HTTPRequest.Request.Method},
			{"httpRequest.requestUrl", actualLog.HTTPRequest.Request.URL.String()},
			{"httpRequest.requestSize", strconv.FormatInt(actualLog.HTTPRequest.RequestSize, 10)},
			{"httpRequest.status", strconv.Itoa(actualLog.HTTPRequest.Status)},
			{"httpRequest.responseSize", strconv.FormatInt(actualLog.HTTPRequest.ResponseSize, 10)},
			{"httpRequest.userAgent", actualLog.HTTPRequest.Request.UserAgent()},
			{"httpRequest.remoteIp", actualLog.HTTPRequest.RemoteIP},
			{"httpRequest.serverIp", actualLog.HTTPRequest.LocalIP},
			{"httpRequest.referer", actualLog.HTTPRequest.Request.Referer()},
			{"httpRequest.latency", actualLog.HTTPRequest.Latency.String()},
			{"httpRequest.cacheLookup", strconv.FormatBool(actualLog.HTTPRequest.CacheLookup)},
			{"httpRequest.cacheHit", strconv.FormatBool(actualLog.HTTPRequest.CacheHit)},
			{"httpRequest.cacheValidatedWithOriginServer", strconv.FormatBool(actualLog.HTTPRequest.CacheValidatedWithOriginServer)},
			{"httpRequest.cacheFillBytes", strconv.FormatInt(actualLog.HTTPRequest.CacheFillBytes, 10)},
			{"httpRequest.protocol", actualLog.HTTPRequest.Request.Proto},
		}
		for _, test := range testPairs {
			expectedHTTPField, actualHTTPField := test[0], test[1]
			if err := verifyLogField(expectedHTTPField, actualHTTPField, expectedFields); err != nil {
				multiErr = multierr.Append(multiErr, err)
			}
		}
	}

	// Labels - Untested as of yet, since no application sets LogEntry labels.

	// JSON Payload
	expectedPayloadFields := logFieldsMapWithPrefix(expectedLog, "jsonPayload.")
	if actualLog.Payload == nil {
		if len(expectedPayloadFields) > 0 {
			multiErr = multierr.Append(multiErr, fmt.Errorf("expected values for field jsonPayload but got nil\n"))
		}
	} else {
		if err := verifyJsonPayload(actualLog.Payload, expectedPayloadFields); err != nil {
			multiErr = multierr.Append(multiErr, err)
		}
	}

	if multiErr != nil {
		return fmt.Errorf("%s: %w", expectedLog.LogName, multiErr)
	}

	return nil
}

// stripUnavailableFields removes the fields that are listed as unavailable_on
// the current image spec.
func stripUnavailableFields(fields []*metadata.LogFields, imageSpec string) []*metadata.LogFields {
	var result []*metadata.LogFields
	for _, field := range fields {
		if !metadata.SliceContains(field.UnavailableOn, imageSpec) {
			result = append(result, field)
		}
	}
	return result
}

func runLoggingTestCases(ctx context.Context, logger *log.Logger, vm *gce.VM, logs []*metadata.ExpectedLog) error {
	// Wait for each entry in LogEntries concurrently. This is especially helpful
	// when	the assertions fail: we don't want to wait for each one to time out
	// back-to-back.
	var err error
	c := make(chan error, len(logs))
	for _, entry := range logs {
		// https://golang.org/doc/faq#closures_and_goroutines
		// Plus we need to dereference the pointer to make a copy of the
		// underlying struct.
		entry := *entry
		go func() {
			// Strip out the fields that are not available on this image spec.
			// We do this here so that:
			// 1. the field is never mentioned in the query we send, and
			// 2. verifyLogField treats it as any other unexpected field, which
			//    means it will fail the test ("expected no value for field").
			//    This could result in annoying test failures if the app suddenly
			//    begins reporting a log field on a certain image.
			entry.Fields = stripUnavailableFields(entry.Fields, vm.ImageSpec)

			// Construct query using remaining fields with a nonempty regex.
			query := constructQuery(entry.LogName, entry.Fields)

			// Query logging backend for log matching the query.
			actualLog, err := gce.QueryLog(ctx, logger, vm, entry.LogName, 1*time.Hour, query, gce.QueryMaxAttempts)
			if err != nil {
				c <- err
				return
			}

			// Verify the log is what was expected.
			err = verifyLog(actualLog, &entry)
			if err != nil {
				c <- err
				return
			}

			c <- nil
		}()
	}
	for range logs {
		err = multierr.Append(err, <-c)
	}
	return err
}

func runMetricsTestCases(ctx context.Context, logger *log.Logger, vm *gce.VM, metrics []*metadata.ExpectedMetric, fc *feature_tracking_metadata.FeatureTrackingContainer) error {
	var err error
	logger.Printf("Parsed expectedMetrics: %s", util.DumpPointerArray(metrics, "%+v"))
	// Wait for the representative metric first, which is intended to *always*
	// be sent. If it doesn't exist, we fail fast and skip running the other metrics;
	// if it does exist, we go on to the other metrics in parallel, by which point they
	// have gotten a head start and should end up needing fewer API calls before being found.
	// In both cases we make significantly fewer API calls which helps us stay under quota.
	for _, metric := range metrics {
		if !metric.Representative {
			continue
		}
		err = assertMetric(ctx, logger, vm, metric)
		if gce.IsExhaustedRetriesMetricError(err) {
			return fmt.Errorf("representative metric %s not found, skipping remaining metrics", metric.Type)
		}
		// If err is non-nil here, then the non-representative metric tests later on will
		// pick it up and report it as part of the multierr.
		break
	}
	// Give some catch-up time to the remaining metrics, which tend to be configured
	// for a 60-second interval, plus 10 seconds to let the data propagate in the backend.
	logger.Println("Found representative metric, sleeping before checking remaining metrics")
	time.Sleep(70 * time.Second)
	// Wait for all remaining metrics, skipping the optional ones.
	// TODO: Improve coverage for optional metrics.
	//       See https://github.com/GoogleCloudPlatform/ops-agent/issues/486
	var requiredMetrics []*metadata.ExpectedMetric
	for _, metric := range metrics {
		if metric.Optional || metric.Representative {
			logger.Printf("Skipping optional or representative metric %s", metric.Type)
			continue
		}
		if metadata.SliceContains(metric.UnavailableOn, vm.ImageSpec) {
			logger.Printf("Skipping metric %s due to unavailable_on", metric.Type)
			continue
		}
		requiredMetrics = append(requiredMetrics, metric)
	}
	c := make(chan error, len(requiredMetrics))
	for _, metric := range requiredMetrics {
		metric := metric // https://go.dev/doc/faq#closures_and_goroutines
		go func() {
			c <- assertMetric(ctx, logger, vm, metric)
		}()
	}
	for range requiredMetrics {
		err = multierr.Append(err, <-c)
	}

	if fc == nil {
		logger.Printf("skipping feature tracking integration tests")
		return err
	}

	series, ft_err := gce.WaitForMetricSeries(ctx, logger, vm, "agent.googleapis.com/agent/internal/ops/feature_tracking", 1*time.Hour, nil, false, len(fc.Features))
	if ft_err != nil {
		return multierr.Append(err, ft_err)
	}

	return multierr.Append(err, feature_tracking_metadata.AssertFeatureTrackingMetrics(series, fc.Features))
}

func assertMetric(ctx context.Context, logger *log.Logger, vm *gce.VM, metric *metadata.ExpectedMetric) error {
	series, err := gce.WaitForMetric(ctx, logger, vm, metric.Type, 1*time.Hour, nil, false)
	if err != nil {
		// Optional metrics can be missing
		if metric.Optional && gce.IsExhaustedRetriesMetricError(err) {
			return nil
		}
		return err
	}
	return metadata.AssertMetric(metric, series)
}

// runSingleTest starts with a fresh VM, installs the app and agent on it,
// and ensures that the agent uploads data from the app.
// Returns an error (nil on success), and a boolean indicating whether the error
// is retryable.
func runSingleTest(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM, app string, metadata metadata.IntegrationMetadata) (retry bool, err error) {
	folder, err := distroFolder(vm)
	if err != nil {
		return nonRetryable, err
	}

	installEnv := make(map[string]string)
	if folder == "debian_ubuntu" {
		// Gets us around problematic prompts for user input.
		installEnv["DEBIAN_FRONTEND"] = "noninteractive"
		// Configures sudo to keep the value of DEBIAN_FRONTEND that we set.
		if _, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, `echo 'Defaults env_keep += "DEBIAN_FRONTEND"' | sudo tee -a /etc/sudoers`); err != nil {
			return nonRetryable, err
		}
	}

	if _, err = runScriptFromScriptsDir(
		ctx, logger.ToMainLog(), vm, path.Join("applications", app, folder, "install"), installEnv); err != nil {
		return retryable, fmt.Errorf("error installing %s: %v", app, err)
	}

	if app == "active_directory_ds" {
		// This will allow us to be able to access the machine over ssh after it restarts.
		if err = updateSSHKeysForActiveDirectory(ctx, logger.ToMainLog(), vm, "test"); err != nil {
			return nonRetryable, err
		}
	}

	if metadata.RestartAfterInstall {
		logger.ToMainLog().Printf("Restarting VM instance...")
		restartLogger := logger.ToFile("VM_restart.txt")
		err := gce.RestartInstance(ctx, restartLogger, vm)

		logger.ToMainLog().Printf("Restarting VM instance returned err=%v, see VM_restart.txt for details.", err)
		if err != nil {
			return nonRetryable, err
		}
	}

	if err := agents.InstallOpsAgent(ctx, logger.ToMainLog(), vm, agents.LocationFromEnvVars()); err != nil {
		// InstallOpsAgent does its own retries.
		return nonRetryable, fmt.Errorf("error installing agent: %v", err)
	}

	if _, err = runScriptFromScriptsDir(ctx, logger.ToMainLog(), vm, path.Join("applications", app, "enable"), nil); err != nil {
		return nonRetryable, fmt.Errorf("error enabling %s: %v", app, err)
	}

	backupConfigFilePath := util.GetConfigPath(vm.ImageSpec) + ".bak"
	if err = assertFilePresence(ctx, logger.ToMainLog(), vm, backupConfigFilePath); err != nil {
		return nonRetryable, fmt.Errorf("error when fetching back up config file %s: %v", backupConfigFilePath, err)
	}

	// Check if the exercise script exists, and run it if it does.
	exerciseScript := path.Join("applications", app, "exercise")
	if _, err := scriptsDir.ReadFile(exerciseScript); err == nil {
		logger.ToMainLog().Println("exercise script found, running...")
		if _, err = runScriptFromScriptsDir(ctx, logger.ToMainLog(), vm, exerciseScript, nil); err != nil {
			return nonRetryable, fmt.Errorf("error exercising %s: %v", app, err)
		}
	}

	if metadata.ExpectedLogs != nil {
		logger.ToMainLog().Println("found expectedLogs, running logging test cases...")
		// TODO(b/254325217): bad bad bad, remove this horrible hack once we fix Aerospike on SLES
		if app == AerospikeApp && folder == "sles" {
			logger.ToMainLog().Printf("skipping aerospike logging tests (b/254325217)")
		} else if err = runLoggingTestCases(ctx, logger.ToMainLog(), vm, metadata.ExpectedLogs); err != nil {
			return nonRetryable, err
		}
	}

	if metadata.ExpectedMetrics != nil {
		logger.ToMainLog().Println("found expectedMetrics, running metrics test cases...")

		// All integrations are expected to set the instrumentation_* labels.
		instrumentedApp := app
		// mariadb is a separate test but it uses the same integration as mysql.
		if app == "mariadb" {
			instrumentedApp = "mysql"
		}
		for _, m := range metadata.ExpectedMetrics {
			// The windows metrics that do not target workload.googleapis.com cannot set
			// the instrumentation_* labels
			if strings.HasPrefix(m.Type, "agent.googleapis.com") {
				continue
			}
			if m.Labels == nil {
				m.Labels = map[string]string{}
			}
			if _, ok := m.Labels["instrumentation_source"]; !ok {
				m.Labels["instrumentation_source"] = regexp.QuoteMeta(fmt.Sprintf("agent.googleapis.com/%s", instrumentedApp))
			}
			if _, ok := m.Labels["instrumentation_version"]; !ok {
				m.Labels["instrumentation_version"] = `.*`
			}
		}

		fc, err := getExpectedFeatures(app)

		if err = runMetricsTestCases(ctx, logger.ToMainLog(), vm, metadata.ExpectedMetrics, fc); err != nil {
			return nonRetryable, err
		}
	}

	return nonRetryable, nil
}

func getExpectedFeatures(app string) (*feature_tracking_metadata.FeatureTrackingContainer, error) {
	var fc feature_tracking_metadata.FeatureTrackingContainer

	featuresScript := path.Join("applications", app, "features.yaml")
	featureBytes, err := scriptsDir.ReadFile(featuresScript)
	if err != nil {
		return nil, err
	}

	err = yaml.UnmarshalStrict(featureBytes, &fc)
	if err != nil {
		return nil, err
	}

	return &fc, nil
}

// Returns a map of application name to its parsed and validated metadata.yaml.
// The set of applications returned is authoritative and corresponds to the
// directory names under integration_test/third_party_apps_test/applications.
func fetchAppsAndMetadata(t *testing.T) map[string]metadata.IntegrationMetadata {
	allApps := make(map[string]metadata.IntegrationMetadata)

	files, err := scriptsDir.ReadDir("applications")
	if err != nil {
		t.Fatalf("got error listing files under applications: %v", err)
	}
	for _, file := range files {
		app := file.Name()
		var integrationMetadata metadata.IntegrationMetadata
		testCaseBytes, err := scriptsDir.ReadFile(path.Join("applications", app, "metadata.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		err = metadata.UnmarshalAndValidate(testCaseBytes, &integrationMetadata)
		if err != nil {
			t.Fatalf("could not validate contents of applications/%v/metadata.yaml: %v", app, err)
		}
		allApps[app] = integrationMetadata
	}
	log.Printf("found %v apps", len(allApps))
	if len(allApps) == 0 {
		t.Fatal("Found no applications inside applications")
	}
	return allApps
}

func modifiedFiles(t *testing.T) []string {
	// This command gets the files that have changed since the current branch
	// diverged from official master. See https://stackoverflow.com/a/65166745.
	cmd := exec.Command("git", "diff", "--name-only", "origin/master...")
	out, err := cmd.Output()
	stdout := string(out)
	if err != nil {
		stderr := ""
		if exitError := err.(*exec.ExitError); exitError != nil {
			stderr = string(exitError.Stderr)
		}
		t.Fatalf("got error calling `git diff`: %v\nstderr=%v\nstdout=%v", err, stderr, stdout)
	}
	log.Printf("git diff output:\n\tstdout:%v", stdout)

	return strings.Split(stdout, "\n")
}

// isCriticalFile returns true if the given modified source file
// means we should test all applications.
func isCriticalFile(f string) bool {
	if strings.HasPrefix(f, "submodules/") {
		return true
	}
	for _, criticalFile := range []string{
		"go.mod",
		"integration_test/agents/agents.go",
		"integration_test/gce/gce_testing.go",
		"integration_test/third_party_apps_test/main_test.go",
		"project.yaml",
	} {
		if f == criticalFile {
			return true
		}
	}
	return false
}

// determineImpactedApps determines what apps are impacted by current code
// changes. Some code changes are considered critical, like changing
// submodules.
// For critical code changes, all apps are considered impacted.
// For non-critical code changes, extracts app names as follows:
//
//	apps/<appname>.go
//	integration_test/third_party_apps_test/applications/<appname>/
//
// Checks the extracted app names against the set of all known apps.
// If tests were explicitly selected, or if no app is found as impacted, assume
// all apps are.
func determineImpactedApps(modifiedFiles []string, allApps map[string]metadata.IntegrationMetadata) map[string]bool {
	impactedApps := make(map[string]bool)
	defer log.Printf("impacted apps: %v", impactedApps)

	if flag.Lookup("test.run") != nil {
		// Honor explicit test selectors.
		for app := range allApps {
			impactedApps[app] = true
		}
		return impactedApps
	}

	for _, f := range modifiedFiles {
		if isCriticalFile(f) {
			// Consider all apps as impacted.
			for app := range allApps {
				impactedApps[app] = true
			}
			return impactedApps
		}
	}

	for _, f := range modifiedFiles {
		if strings.HasPrefix(f, "apps/") {
			// File names: apps/<f>.go
			f := strings.TrimPrefix(f, "apps/")
			f = strings.TrimSuffix(f, ".go")

			// To support testing multiple versions of an app, we consider all apps
			// in allApps to be a match if they have <f> as a prefix.
			// For example, consider f = "mongodb". Then all of
			// {mongodb3.6, mongodb} are considered impacted.
			for app := range allApps {
				if strings.HasPrefix(app, f) {
					impactedApps[app] = true
				}
			}
		} else if strings.HasPrefix(f, "integration_test/third_party_apps_test/applications/") {
			// Folder names: integration_test/third_party_apps_test/applications/<app_name>
			f := strings.TrimPrefix(f, "integration_test/third_party_apps_test/applications/")
			f = strings.Split(f, "/")[0]
			// The directories here are already authoritative, no
			// need to check against list.
			impactedApps[f] = true
		}
	}

	if len(impactedApps) == 0 {
		// If none of the apps are impacted, treat all of them as impacted.
		for app := range allApps {
			impactedApps[app] = true
		}
		return impactedApps
	}
	return impactedApps
}

type accelerator struct {
	model         string
	fullName      string
	machineType   string
	availableZone string
}

type test struct {
	imageSpec  string
	app        string
	gpu        *accelerator
	metadata   metadata.IntegrationMetadata
	skipReason string
}

var defaultPlatforms = map[string]bool{
	"debian-cloud:debian-11":     true,
	"windows-cloud:windows-2019": true,
}

var defaultApps = map[string]bool{
	// Chosen because it is relatively popular in the wild.
	// There may be a better choice.
	"postgresql": true,
	// Chosen because it is the most nontrivial Windows app currently
	// implemented.
	"active_directory_ds": true,
}

var gpuModels = map[string]accelerator{
	// This is the A100 40G model; A100 80G is similar so skipping
	"a100": {
		model:         "a100",
		fullName:      "nvidia-tesla-a100",
		machineType:   "a2-highgpu-1g",
		availableZone: "us-central1-a",
	},
	"v100": {
		model:         "v100",
		fullName:      "nvidia-tesla-v100",
		machineType:   "n1-standard-2",
		availableZone: "us-central1-a",
	},
	"t4": {
		model:         "t4",
		fullName:      "nvidia-tesla-t4",
		machineType:   "n1-standard-2",
		availableZone: "us-central1-a",
	},
	"p4": {
		model:         "p4",
		fullName:      "nvidia-tesla-p4",
		machineType:   "n1-standard-2",
		availableZone: "us-central1-a",
	},
	"p100": {
		model:         "p100",
		fullName:      "nvidia-tesla-p100",
		machineType:   "n1-standard-2",
		availableZone: "us-central1-c",
	},
	"l4": {
		model:         "l4",
		fullName:      "nvidia-l4",
		machineType:   "g2-standard-4",
		availableZone: "us-central1-a",
	},
}

const (
	SAPHANAImageSpec = "stackdriver-test-143416:sles-15-sp4-sap-saphana"
	SAPHANAApp       = "saphana"

	OracleDBApp  = "oracledb"
	AerospikeApp = "aerospike"
)

// incompatibleOperatingSystem looks at the supported_operating_systems field
// of metadata.yaml for this app and returns a nonempty skip reason if it
// thinks this app doesn't support the given image.
// supported_operating_systems should only contain "linux", "windows", or
// "linux_and_windows".
func incompatibleOperatingSystem(testCase test) string {
	supported := testCase.metadata.SupportedOperatingSystems
	if !strings.Contains(supported, gce.OSKind(testCase.imageSpec)) {
		return fmt.Sprintf("Skipping test for image spec %v because app %v only supports %v.", testCase.imageSpec, testCase.app, supported)
	}
	return "" // We are testing on a supported image for this app.
}

// When in `-short` test mode, mark some tests for skipping, based on
// test_config and impacted apps.
//   - For all impacted apps, test on all images.
//   - Always test all apps against the default image.
//   - Always test the default app (postgres/active_directory_ds for now)
//     on all images.
//
// `platforms_to_skip` overrides the above.
// Also, restrict `SAPHANAImageSpec` to only test `SAPHANAApp` and skip that
// app on all other images too.
func determineTestsToSkip(tests []test, impactedApps map[string]bool) {
	for i, test := range tests {
		if testing.Short() {
			_, testApp := impactedApps[test.app]
			_, defaultApp := defaultApps[test.app]
			_, defaultPlatform := defaultPlatforms[test.imageSpec]
			if !defaultPlatform && !defaultApp && !testApp {
				tests[i].skipReason = fmt.Sprintf("skipping %v because it's not impacted by pending change", test.app)
			}
		}
		if metadata.SliceContains(test.metadata.PlatformsToSkip, test.imageSpec) {
			tests[i].skipReason = "Skipping test due to 'platforms_to_skip' entry in metadata.yaml"
		}
		for _, gpuPlatform := range test.metadata.GpuPlatforms {
			if test.gpu != nil && test.gpu.model == gpuPlatform.Model && !metadata.SliceContains(gpuPlatform.Platforms, test.imageSpec) {
				tests[i].skipReason = "Skipping test due to 'gpu_platforms.platforms' entry in metadata.yaml"
			}
		}
		if reason := incompatibleOperatingSystem(test); reason != "" {
			tests[i].skipReason = reason
		}
		if test.app == "mssql" && strings.HasPrefix(test.imageSpec, "windows-cloud") {
			tests[i].skipReason = "Skipping MSSQL test because this version of Windows doesn't have MSSQL"
		}
		isSAPHANAImageSpec := test.imageSpec == SAPHANAImageSpec
		isSAPHANAApp := test.app == SAPHANAApp
		if isSAPHANAImageSpec != isSAPHANAApp {
			tests[i].skipReason = fmt.Sprintf("Skipping %v because we only want to test %v on %v", test.app, SAPHANAApp, SAPHANAImageSpec)
		}
	}
}

// This is the entry point for the test. Runs runSingleTest
// for each image in IMAGE_SPECS and each app in linuxApps or windowsApps.
func TestThirdPartyApps(t *testing.T) {
	t.Cleanup(gce.CleanupKeysOrDie)

	tests := []test{}
	allApps := fetchAppsAndMetadata(t)
	imageSpecs := strings.Split(os.Getenv("IMAGE_SPECS"), ",")

	for _, imageSpec := range imageSpecs {
		for app, metadata := range allApps {
			if len(metadata.GpuPlatforms) > 0 {
				for _, gpuPlatform := range metadata.GpuPlatforms {
					if gpu, ok := gpuModels[gpuPlatform.Model]; !ok {
						t.Fatalf("invalid gpu model name %s", gpuPlatform)
					} else {
						tests = append(tests, test{imageSpec: imageSpec, gpu: &gpu, app: app, metadata: metadata, skipReason: ""})
					}
				}
			} else {
				tests = append(tests, test{imageSpec: imageSpec, app: app, metadata: metadata, skipReason: ""})
			}
		}
	}

	// Filter tests
	determineTestsToSkip(tests, determineImpactedApps(modifiedFiles(t), allApps))

	// Execute tests
	for _, tc := range tests {
		tc := tc // https://golang.org/doc/faq#closures_and_goroutines

		testName := tc.imageSpec + "/" + tc.app
		if tc.gpu != nil {
			testName = testName + "/" + tc.gpu.fullName
		}

		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			if tc.skipReason != "" {
				t.Skip(tc.skipReason)
			}

			ctx, cancel := context.WithTimeout(context.Background(), gce.SuggestedTimeout)
			defer cancel()
			ctx = gce.WithGcloudConfigDir(ctx, t.TempDir())

			var err error
			for attempt := 1; attempt <= 4; attempt++ {
				logger := gce.SetupLogger(t)
				logger.ToMainLog().Println("Calling SetupVM(). For details, see VM_initialization.txt.")
				options := gce.VMOptions{
					ImageSpec:            tc.imageSpec,
					TimeToLive:           "3h",
					MachineType:          agents.RecommendedMachineType(tc.imageSpec),
					ExtraCreateArguments: nil,
				}
				if tc.gpu != nil {
					options.ExtraCreateArguments = append(
						options.ExtraCreateArguments,
						fmt.Sprintf("--accelerator=count=1,type=%s", tc.gpu.fullName),
						"--maintenance-policy=TERMINATE")
					options.ExtraCreateArguments = append(options.ExtraCreateArguments, "--boot-disk-size=100GB")
					options.MachineType = tc.gpu.machineType
					options.Zone = tc.gpu.availableZone
				}
				if tc.imageSpec == SAPHANAImageSpec {
					// This image needs an SSD in order to be performant enough.
					options.ExtraCreateArguments = append(options.ExtraCreateArguments, "--boot-disk-type=pd-ssd")
				}
				if tc.app == OracleDBApp {
					options.MachineType = "e2-highmem-8"
					if gce.IsARM(tc.imageSpec) {
						// T2A doesn't have a highmem line, so pick the standard machine that's specced at least
						// as well as e2-highmem-8.
						options.MachineType = "t2a-standard-16"
					}
					options.ExtraCreateArguments = append(options.ExtraCreateArguments, "--boot-disk-size=150GB", "--boot-disk-type=pd-ssd")
				}

				vm := gce.SetupVM(ctx, t, logger.ToFile("VM_initialization.txt"), options)
				logger.ToMainLog().Printf("VM is ready: %#v", vm)

				var retryable bool
				retryable, err = runSingleTest(ctx, logger, vm, tc.app, tc.metadata)
				t.Logf("Attempt %v of %s test of %s finished with err=%v, retryable=%v", attempt, tc.imageSpec, tc.app, err, retryable)
				if err == nil {
					return
				}
				agents.RunOpsAgentDiagnostics(ctx, logger, vm)
				if !retryable {
					t.Fatalf("Non-retryable error: %v", err)
				}
				// If we got here, we're going to retry runSingleTest(). The VM we spawned
				// won't be deleted until the end of t.Run(), (SetupVM() registers it for cleanup
				// at the end of t.Run()), so to avoid accumulating too many idle VMs while we
				// do our retries, we preemptively delete the VM now.
				if deleteErr := gce.DeleteInstance(logger.ToMainLog(), vm); deleteErr != nil {
					t.Errorf("Deleting VM %v failed: %v", vm.Name, deleteErr)
				}
			}
			t.Errorf("Final attempt failed: %v", err)
		})
	}

}
