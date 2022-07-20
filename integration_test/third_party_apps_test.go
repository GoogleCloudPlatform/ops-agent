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

PLATFORMS: a comma-separated list of distros to test, e.g. "centos-7,centos-8".

The following variables are optional:

AGENT_PACKAGES_IN_GCS: If provided, a URL for a directory in GCS containing
    .deb/.rpm/.goo files to install on the testing VMs.
REPO_SUFFIX: If provided, a package repository suffix to install the agent from.
    AGENT_PACKAGES_IN_GCS takes precedence over REPO_SUFFIX.
*/

package integration_test

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/integration_test/agents"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/common"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/gce"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/logging"

	"go.uber.org/multierr"
	"gopkg.in/yaml.v2"

	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

var (
	packagesInGCS = os.Getenv("AGENT_PACKAGES_IN_GCS")
)

//go:embed third_party_apps_data
var scriptsDir embed.FS

var validate = common.NewIntegrationMetadataValidator()

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

// osFolder returns the folder containing OS-specific configuration and
// scripts for the test.
func osFolder(platform string) string {
	if gce.IsWindows(platform) {
		return "windows"
	}
	return "linux"
}

const (
	retryable    = true
	nonRetryable = false
)

// distroFolder returns the distro family name we use in our directory hierarchy
// inside the scripts directory.
func distroFolder(platform string) (string, error) {
	if gce.IsWindows(platform) {
		return "windows", nil
	}
	firstWord := strings.Split(platform, "-")[0]
	switch firstWord {
	case "centos", "rhel", "rocky":
		return "centos_rhel", nil
	case "debian", "ubuntu":
		return "debian_ubuntu", nil
	case "opensuse", "sles":
		return "sles", nil
	}
	return "", fmt.Errorf("distroFolder() could not find matching folder holding scripts for platform %s", platform)
}

func readFileFromScriptsDir(scriptPath string) ([]byte, error) {
	return scriptsDir.ReadFile(path.Join("third_party_apps_data", scriptPath))
}

// runScriptFromScriptsDir runs a script on the given VM.
// The scriptPath should be relative to SCRIPTS_DIR.
// The script should be a shell script for a Linux VM and powershell for a Windows VM.
// env is a map containing environment variables to provide to the script as it runs.
func runScriptFromScriptsDir(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM, scriptPath string, env map[string]string) (gce.CommandOutput, error) {
	logger.ToMainLog().Printf("Running script with path %s", scriptPath)

	scriptContents, err := readFileFromScriptsDir(scriptPath)
	if err != nil {
		return gce.CommandOutput{}, err
	}
	return gce.RunScriptRemotely(ctx, logger, vm, string(scriptContents), nil, env)
}

// Installs the agent according to the instructions in a script
// stored in the scripts directory.
func installUsingScript(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM) (bool, error) {
	environmentVariables := make(map[string]string)
	suffix := os.Getenv("REPO_SUFFIX")
	if suffix != "" {
		environmentVariables["REPO_SUFFIX"] = suffix
	}
	if _, err := runScriptFromScriptsDir(ctx, logger, vm, path.Join("agent", osFolder(vm.Platform), "install"), environmentVariables); err != nil {
		return retryable, fmt.Errorf("error installing agent: %v", err)
	}
	return nonRetryable, nil
}

func installAgent(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM) (bool, error) {
	defer time.Sleep(10 * time.Second)
	if packagesInGCS == "" {
		return installUsingScript(ctx, logger, vm)
	}
	return nonRetryable, agents.InstallPackageFromGCS(ctx, logger, vm, packagesInGCS)
}

// constructQuery converts the given struct of:
//   field name => field value regex
// into a query filter to pass to the logging API.
func constructQuery(logName string, fields []*common.LogFields) string {
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

func runLoggingTestCases(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM, logs []*common.ExpectedLog) error {

	// Wait for each entry in LogEntries concurrently. This is especially helpful
	// when	the assertions fail: we don't want to wait for each one to time out
	// back-to-back.
	var err error
	c := make(chan error, len(logs))
	for _, entry := range logs {
		entry := entry // https://golang.org/doc/faq#closures_and_goroutines
		go func() {
			c <- gce.WaitForLog(ctx, logger.ToMainLog(), vm, entry.LogName, 1*time.Hour, constructQuery(entry.LogName, entry.Fields))
		}()
	}
	for range logs {
		err = multierr.Append(err, <-c)
	}
	return err
}

func runMetricsTestCases(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM, metrics []*common.ExpectedMetric) error {
	var err error
	logger.ToMainLog().Printf("Parsed expectedMetrics: %+v", metrics)
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
	logger.ToMainLog().Println("Found representative metric, sleeping before checking remaining metrics")
	time.Sleep(70 * time.Second)
	// Wait for all remaining metrics, skipping the optional ones.
	// TODO: Improve coverage for optional metrics.
	//       See https://github.com/GoogleCloudPlatform/ops-agent/issues/486
	var requiredMetrics []*common.ExpectedMetric
	for _, metric := range metrics {
		if metric.Optional || metric.Representative {
			logger.ToMainLog().Printf("Skipping optional or representative metric %s", metric.Type)
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
	return err
}

func assertMetric(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM, metric *common.ExpectedMetric) error {
	series, err := gce.WaitForMetric(ctx, logger.ToMainLog(), vm, metric.Type, 1*time.Hour, nil)
	if err != nil {
		// Optional metrics can be missing
		if metric.Optional && gce.IsExhaustedRetriesMetricError(err) {
			return nil
		}
		return err
	}
	if series.ValueType.String() != metric.ValueType {
		err = multierr.Append(err, fmt.Errorf("valueType: expected %s but got %s", metric.ValueType, series.ValueType.String()))
	}
	if series.MetricKind.String() != metric.Kind {
		err = multierr.Append(err, fmt.Errorf("kind: expected %s but got %s", metric.Kind, series.MetricKind.String()))
	}
	if series.Resource.Type != metric.MonitoredResource {
		err = multierr.Append(err, fmt.Errorf("monitored_resource: expected %s but got %s", metric.MonitoredResource, series.Resource.Type))
	}
	err = multierr.Append(err, assertMetricLabels(metric, series))
	if err != nil {
		return fmt.Errorf("%s: %w", metric.Type, err)
	}
	return nil
}

func assertMetricLabels(metric *common.ExpectedMetric, series *monitoringpb.TimeSeries) error {
	// All present labels must be expected
	var err error
	for actualLabel := range series.Metric.Labels {
		if _, ok := metric.Labels[actualLabel]; !ok {
			err = multierr.Append(err, fmt.Errorf("unexpected label: %s", actualLabel))
		}
	}
	// All expected labels must be present and match the given pattern
	for expectedLabel, expectedPattern := range metric.Labels {
		actualValue, ok := series.Metric.Labels[expectedLabel]
		if !ok {
			err = multierr.Append(err, fmt.Errorf("expected label not found: %s", expectedLabel))
			continue
		}
		match, matchErr := regexp.MatchString(expectedPattern, actualValue)
		if matchErr != nil {
			err = multierr.Append(err, fmt.Errorf("error parsing pattern. label=%s, pattern=%s, err=%v",
				expectedLabel,
				expectedPattern,
				matchErr,
			))
		} else if !match {
			err = multierr.Append(err, fmt.Errorf("error: label value does not match pattern. label=%s, pattern=%s, value=%s",
				expectedLabel,
				expectedPattern,
				actualValue,
			))
		}
	}
	return err
}

type testConfig struct {
	// Note on tags: the "yaml" tag specifies the name of this field in the
	// .yaml file.

	// per_application_overrides is a map from application to specific settings
	// for that application.
	PerApplicationOverrides map[string]struct {
		// platforms_to_skip is a list of platforms that need to be skipped for
		// this application. Ideally this will be empty or nearly empty most of
		// the time.
		PlatformsToSkip []string `yaml:"platforms_to_skip"`
	} `yaml:"per_application_overrides"`
}

// parseTestConfigFile looks for test_config.yaml, and if it exists, merges
// any options in it into the default test config and returns the result.
func parseTestConfigFile() (testConfig, error) {
	config := testConfig{}

	bytes, err := readFileFromScriptsDir("test_config.yaml")
	if err != nil {
		log.Printf("Reading test_config.yaml failed with err=%v, proceeding...", err)
		// Probably the file is just missing, return the defaults.
		return config, nil
	}

	if err = yaml.UnmarshalStrict(bytes, &config); err != nil {
		return testConfig{}, err
	}
	return config, nil
}

// runSingleTest starts with a fresh VM, installs the app and agent on it,
// and ensures that the agent uploads data from the app.
// Returns an error (nil on success), and a boolean indicating whether the error
// is retryable.
func runSingleTest(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM, app string, metadata common.IntegrationMetadata) (retry bool, err error) {
	folder, err := distroFolder(vm.Platform)
	if err != nil {
		return nonRetryable, err
	}

	if _, err = runScriptFromScriptsDir(
		ctx, logger, vm, path.Join("applications", app, folder, "install"), nil); err != nil {
		return retryable, fmt.Errorf("error installing %s: %v", app, err)
	}

	if metadata.RestartAfterInstall {
		logger.ToMainLog().Printf("Restarting vm instance...")
		err := gce.RestartInstance(ctx, logger, vm)
		if err != nil {
			return nonRetryable, err
		}
		logger.ToMainLog().Printf("vm instance restarted")
	}

	if shouldRetry, err := installAgent(ctx, logger, vm); err != nil {
		return shouldRetry, fmt.Errorf("error installing agent: %v", err)
	}

	if _, err = runScriptFromScriptsDir(ctx, logger, vm, path.Join("applications", app, "enable"), nil); err != nil {
		return nonRetryable, fmt.Errorf("error enabling %s: %v", app, err)
	}

	// Check if the exercise script exists, and run it if it does.
	exerciseScript := path.Join("applications", app, "exercise")
	if _, err := readFileFromScriptsDir(exerciseScript); err == nil {
		logger.ToMainLog().Println("exercise script found, running...")
		if _, err = runScriptFromScriptsDir(ctx, logger, vm, exerciseScript, nil); err != nil {
			return nonRetryable, fmt.Errorf("error exercising %s: %v", app, err)
		}
	}

	if metadata.ExpectedLogs != nil {
		logger.ToMainLog().Println("found expectedLogs, running logging test cases...")
		if err = runLoggingTestCases(ctx, logger, vm, metadata.ExpectedLogs); err != nil {
			return nonRetryable, err
		}
	}

	if metadata.ExpectedMetrics != nil {
		logger.ToMainLog().Println("found expectedMetrics, running metrics test cases...")
		if err = runMetricsTestCases(ctx, logger, vm, metadata.ExpectedMetrics); err != nil {
			return nonRetryable, err
		}
	}

	return nonRetryable, nil
}

// Returns a map of application name to its parsed and validated metadata.yaml.
// The set of applications returned is authoritative and corresponds to the
// directory names under integration_test/third_party_apps_data/applications.
func fetchAppsAndMetadata(t *testing.T) map[string]common.IntegrationMetadata {
	allApps := make(map[string]common.IntegrationMetadata)

	files, err := scriptsDir.ReadDir(path.Join("third_party_apps_data", "applications"))
	if err != nil {
		t.Fatalf("got error listing files under third_party_apps_data/applications: %v", err)
	}
	for _, file := range files {
		app := file.Name()
		var metadata common.IntegrationMetadata
		testCaseBytes, err := readFileFromScriptsDir(path.Join("applications", app, "metadata.yaml"))
		if err != nil {
			t.Fatalf("could not read applications/%v/metadata.yaml: %v", app, err)
		}
		err = yaml.UnmarshalStrict(testCaseBytes, &metadata)
		if err != nil {
			t.Fatalf("could not unmarshal contents of applications/%v/metadata.yaml: %v", app, err)
		}
		if err = validate.Struct(&metadata); err != nil {
			t.Fatalf("could not validate contents of applications/%v/metadata.yaml: %v", app, err)
		}
		allApps[app] = metadata
	}
	log.Printf("found %v apps", len(allApps))
	if len(allApps) == 0 {
		t.Fatal("Found no applications inside third_party_apps_data/applications")
	}
	return allApps
}

func modifiedFiles(t *testing.T) []string {
	cmd := exec.Command("git", "diff", "--name-only", "origin/master")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("got error calling `git diff`: %v", err)
	}
	stdout := string(out)
	log.Printf("git diff output:\n\tstdout:%v", stdout)

	return strings.Split(stdout, "\n")
}

// Determine what apps are impacted by current code changes.
// Extracts app names as follows:
//   apps/<appname>.go
//   integration_test/third_party_apps_data/<appname>/
// Checks the extracted app names against the set of all known apps.
func determineImpactedApps(mf []string, allApps map[string]common.IntegrationMetadata) map[string]bool {
	impactedApps := make(map[string]bool)
	for _, f := range mf {
		if strings.HasPrefix(f, "apps/") {

			// File names: apps/<appname>.go
			f := strings.TrimPrefix(f, "apps/")
			f = strings.TrimSuffix(f, ".go")

			if _, ok := allApps[f]; ok {
				impactedApps[f] = true
			}
		} else if strings.HasPrefix(f, "integration_test/third_party_apps_data/applications/") {
			// Folder names: integration_test/third_party_apps_data/applications/<app_name>
			f := strings.TrimPrefix(f, "integration_test/third_party_apps_data/applications/")
			f = strings.Split(f, "/")[0]
			// The directories here are already authoritative, no
			// need to check against list.
			impactedApps[f] = true

		}
	}
	log.Printf("impacted apps: %v", impactedApps)
	return impactedApps
}

type test struct {
	platform   string
	app        string
	metadata   common.IntegrationMetadata
	skipReason string
}

var defaultPlatforms = map[string]bool{
	"debian-10":    true,
	"windows-2019": true,
}

const (
	SAPHANAPlatform = "sles-15-sp3-sap-saphana"
	SAPHANAApp      = "saphana"
)

// incompatibleOperatingSystem looks at the supported_operating_systems field
// of metadata.yaml for this app and returns a nonempty skip reason if it
// thinks this app doesn't support the given platform.
// supported_operating_systems should only contain "linux", "windows", or
// "linux_and_windows".
func incompatibleOperatingSystem(testCase test) string {
    supported := testCase.metadata.SupportedOperatingSystems
    if !strings.Contains(supported, gce.PlatformKind(testCase.platform)) {
        return fmt.Sprintf("Skipping test for platform %v because app %v only supports %v.", testCase.platform, testCase.app, supported)
    }
    return "" // We are testing on a supported platform for this app.
}

// When in `-short` test mode, mark some tests for skipping, based on
// test_config and impacted apps.  Always test all apps against the default
// platform.  If a subset of apps is determined to be impacted, also test all
// platforms for those apps.
// `platforms_to_skip` overrides the above.
// Also, restrict `SAPHANAPlatform` to only test `SAPHANAApp` and skip that
// app on all other platforms too.
func determineTestsToSkip(tests []test, impactedApps map[string]bool, testConfig testConfig) {
	for i, test := range tests {
		if testing.Short() {
			_, testApp := impactedApps[test.app]
			_, defaultPlatform := defaultPlatforms[test.platform]
			if !defaultPlatform && !testApp {
				tests[i].skipReason = fmt.Sprintf("skipping %v because it's not impacted by pending change", test.app)
			}
		}
		if common.SliceContains(testConfig.PerApplicationOverrides[test.app].PlatformsToSkip, test.platform) {
			tests[i].skipReason = "Skipping test due to 'platforms_to_skip' entry in test_config.yaml"
		}
		if reason := incompatibleOperatingSystem(test); reason != "" {
			tests[i].skipReason = reason
		}
		if test.app == "mssql" && gce.IsWindows(test.platform) && !strings.HasPrefix(test.platform, "sql-") {
			tests[i].skipReason = "Skipping MSSQL test because this version of Windows doesn't have MSSQL"
		}
		isSAPHANAPlatform := test.platform == SAPHANAPlatform
		isSAPHANAApp := test.app == SAPHANAApp
		if isSAPHANAPlatform != isSAPHANAApp {
			tests[i].skipReason = fmt.Sprintf("Skipping %v because we only want to test %v on %v", test.app, SAPHANAApp, SAPHANAPlatform)
		}
	}
}

// This is the entry point for the test. Runs runSingleTest
// for each platform in PLATFORMS and each app in linuxApps or windowsApps.
func TestThirdPartyApps(t *testing.T) {
	t.Cleanup(gce.CleanupKeysOrDie)

	testConfig, err := parseTestConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	tests := []test{}
	allApps := fetchAppsAndMetadata(t)
	platforms := strings.Split(os.Getenv("PLATFORMS"), ",")
	for _, platform := range platforms {
		for app, metadata := range allApps {
			tests = append(tests, test{platform: platform, app: app, metadata: metadata, skipReason: ""})
		}
	}

	// Filter tests
	determineTestsToSkip(tests, determineImpactedApps(modifiedFiles(t), allApps), testConfig)

	// Execute tests
	for _, tc := range tests {
		tc := tc // https://golang.org/doc/faq#closures_and_goroutines
		t.Run(tc.platform+"/"+tc.app, func(t *testing.T) {
			t.Parallel()

			if tc.skipReason != "" {
				t.Skip(tc.skipReason)
			}

			ctx, cancel := context.WithTimeout(context.Background(), gce.SuggestedTimeout)
			defer cancel()

			var err error
			for attempt := 1; attempt <= 4; attempt++ {
				logger := gce.SetupLogger(t)
				logger.ToMainLog().Println("Calling SetupVM(). For details, see VM_initialization.txt.")
				options := gce.VMOptions{
					Platform:             tc.platform,
					MachineType:          agents.RecommendedMachineType(tc.platform),
					ExtraCreateArguments: nil,
				}
				if tc.platform == SAPHANAPlatform {
					// This image needs an SSD in order to be performant enough.
					options.ExtraCreateArguments = append(options.ExtraCreateArguments, "--boot-disk-type=pd-ssd")
					options.ImageProject = "stackdriver-test-143416"
				}

				vm := gce.SetupVM(ctx, t, logger.ToFile("VM_initialization.txt"), options)
				logger.ToMainLog().Printf("VM is ready: %#v", vm)

				var retryable bool
				retryable, err = runSingleTest(ctx, logger, vm, tc.app, tc.metadata)
				log.Printf("Attempt %v of %s test of %s finished with err=%v, retryable=%v", attempt, tc.platform, tc.app, err, retryable)
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
