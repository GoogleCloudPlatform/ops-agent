//go:build integration_test

/*
Test for third-party app integrations. Can be run with Kokoro or "go test".
For instructions, see the top of gce_testing.go.

This test needs the following environment variables to be defined, in addition
to the ones mentioned at the top of gce_testing.go:

SCRIPTS_DIR: a path containing scripts for installing/configuring the various
    applications and agents. Also has some files that aren't technically
    scripts that tell the test what to do, such as supported_applications.txt.

PLATFORMS: a comma-separated list of distros to test, e.g. "centos-7,centos-8".

The following variables are optional:

AGENT_PACKAGES_IN_GCS: If provided, a URL for a directory in GCS containing
    .deb/.rpm/.goo files to install on the testing VMs. They must be inside
    a directory called ops-agent. For example, this would be a valid structure
    inside AGENT_PACKAGES_IN_GCS:
    └── ops-agent
        ├── ops-agent-google-cloud-1.2.3.deb
        ├── ops-agent-google-cloud-1.2.3.rpm
        └── ops-agent-google-cloud-1.2.3.goo
REPO_SUFFIX: If provided, a package repository suffix to install the agent from.
    AGENT_PACKAGES_IN_GCS takes precedence over REPO_SUFFIX.
*/

package integration_test

import (
	"context"
	"errors"
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
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/gce"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/logging"

	"github.com/go-playground/validator/v10"
	"go.uber.org/multierr"
	"gopkg.in/yaml.v2"

	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

var (
	scriptsDir    = os.Getenv("SCRIPTS_DIR")
	packagesInGCS = os.Getenv("AGENT_PACKAGES_IN_GCS")
	allowedEnums  = map[string][]string{
		"metric_kind":               {"GAUGE", "DELTA", "CUMULATIVE"},
		"metric_value_type":         {"BOOL", "INT64", "DOUBLE", "STRING", "DISTRIBUTION"},
		"metric_monitored_resource": {"gce_instance"},
	}
)

type thirdPartyValidator struct {
	v *validator.Validate
}

func newValidator() *validator.Validate {
	v := validator.New()
	for enumKey, enumValues := range allowedEnums {
		enumKey := enumKey
		enumValues := enumValues
		v.RegisterValidation(enumKey, func(fl validator.FieldLevel) bool {
			return sliceContains(enumValues, fl.Field().String())
		})
	}
	return v
}

// rewriteEnumErrors rewrites enum validation errors to be more informative.
// After rewriting:
//     Kind: invalid value CUMULATIV (must be one of [GAUGE DELTA CUMULATIVE])
// Before rewriting:
//     Key: 'expectedMetric.Kind' Error:Field validation for 'Kind' failed on the 'metric_kind' tag
func rewriteEnumErrors(err error) error {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return err
	}
	err = nil
	for _, v := range ve {
		allowedEnumValues, ok := allowedEnums[v.Tag()]
		if !ok {
			err = multierr.Append(err, ve)
			continue
		}
		err = multierr.Append(err, fmt.Errorf("%s: invalid value %v (must be one of %v)",
			v.Field(),
			v.Value(),
			allowedEnumValues,
		))
	}
	return err
}

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

// appsToTest reads which applications to test for the given agent+platform
// combination from the appropriate supported_applications.txt file.
func appsToTest(agentType, platform string) ([]string, error) {
	contents, err := os.ReadFile(
		path.Join(scriptsDir, "agent", agentType, osFolder(platform), "supported_applications.txt"))
	if err != nil {
		return nil, fmt.Errorf("could not read supported_applications.txt: %v", err)
	}

	apps := strings.Split(strings.TrimSpace(string(contents)), "\n")
	if gce.IsWindows(platform) && !strings.HasPrefix(platform, "sql-") {
		apps = removeFromSlice(apps, "mssql")
	}
	return apps, nil
}

// findMetricName reads which metric to query from the metric_name.txt file
// corresponding to the given application. The file is allowed to be empty,
// and if so, the test is skipped.
func findMetricName(app string) (string, error) {
	contents, err := os.ReadFile(path.Join(scriptsDir, "applications", app, "metric_name.txt"))
	if err != nil {
		return "", fmt.Errorf("could not read metric_name.txt: %v", err)
	}
	return strings.TrimSpace(string(contents)), nil
}

func sliceContains(slice []string, toFind string) bool {
	for _, entry := range slice {
		if entry == toFind {
			return true
		}
	}
	return false
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
	return os.ReadFile(path.Join(scriptsDir, scriptPath))
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
func installUsingScript(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM, agentType string) (bool, error) {
	environmentVariables := make(map[string]string)
	suffix := os.Getenv("REPO_SUFFIX")
	if suffix != "" {
		environmentVariables["REPO_SUFFIX"] = suffix
	}
	if _, err := runScriptFromScriptsDir(ctx, logger, vm, path.Join("agent", agentType, osFolder(vm.Platform), "install"), environmentVariables); err != nil {
		return retryable, fmt.Errorf("error installing agent: %v", err)
	}
	return nonRetryable, nil
}

func installAgent(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM, agentType string) (bool, error) {
	defer time.Sleep(10 * time.Second)
	if packagesInGCS == "" {
		return installUsingScript(ctx, logger, vm, agentType)
	}
	return nonRetryable, agents.InstallPackageFromGCS(ctx, logger, vm, agentType, packagesInGCS)
}

// expectedLogs encodes a series of assertions about what data we expect
// to see in the logging backend.
type expectedLogs struct {
	// Note on tags: the "yaml" tag specifies the name of this field in the
	// .yaml file.
	LogEntries []expectedLog `yaml:"log_entries"`
}
type expectedLog struct {
	LogName string `yaml:"log_name"`
	// Map of field name to a regex that is expected to match the field value.
	// For example, {"jsonPayload.message": ".*access denied.*"}.
	FieldMatchers map[string]string `yaml:"field_matchers"`
}

// expectedMetric encodes a series of assertions about what data we expect
// to see in the metrics backend.
type expectedMetric struct {
	// The metric type, for example workload.googleapis.com/apache.current_connections.
	Type string `yaml:"type"`
	// The value type, for example INT64.
	ValueType string `yaml:"value_type" validate:"metric_value_type"`
	// The kind, for example GAUGE.
	Kind string `yaml:"kind" validate:"metric_kind"`
	// The monitored resource, for example gce_instance.
	// Currently we only test with gce_instance, so we expect
	// all expectedMetricsEntries to have gce_instance.
	MonitoredResource string `yaml:"monitored_resource" validate:"metric_monitored_resource"`
	// Mapping of expected label keys to value patterns.
	// Patterns are RE2 regular expressions.
	Labels map[string]string `yaml:"labels"`
	// If Optional is true, the test will not fail if this metric
	// is missing.
	Optional bool `yaml:"optional,omitempty"`
	// Exactly one metric in each expected_metrics.yaml must
	// have Representative set to true. This metric can be used
	// to test that the integration is enabled.
	Representative bool `yaml:"representative,omitempty"`
}

// constructQuery converts the given map of:
//   field name => field value regex
// into a query filter to pass to the logging API.
func constructQuery(fieldMatchers map[string]string) string {
	var parts []string
	for field, matcher := range fieldMatchers {
		parts = append(parts, fmt.Sprintf("%s=~%q", field, matcher))
	}
	return strings.Join(parts, " AND ")
}

func runLoggingTestCases(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM, testCaseBytes []byte) error {
	var entries expectedLogs
	err := yaml.UnmarshalStrict(testCaseBytes, &entries)
	if err != nil {
		return fmt.Errorf("could not unmarshal contents of expected_logs.yaml: %v", err)
	}
	logger.ToMainLog().Printf("Parsed expected_logs.yaml: %+v", entries)

	// Wait for each entry in LogEntries concurrently. This is especially helpful
	// when	the assertions fail: we don't want to wait for each one to time out
	// back-to-back.
	c := make(chan error, len(entries.LogEntries))
	for _, entry := range entries.LogEntries {
		entry := entry // https://golang.org/doc/faq#closures_and_goroutines
		go func() {
			c <- gce.WaitForLog(ctx, logger.ToMainLog(), vm, entry.LogName, 1*time.Hour, constructQuery(entry.FieldMatchers))
		}()
	}
	for range entries.LogEntries {
		err = multierr.Append(err, <-c)
	}
	return err
}

func runMetricsTestCases(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM, testCaseBytes []byte) error {
	var metrics []expectedMetric
	var err error
	if err = yaml.UnmarshalStrict(testCaseBytes, &metrics); err != nil {
		return fmt.Errorf("could not unmarshal contents of expected_metrics.yaml: %v", err)
	}
	if err = validateMetrics(metrics); err != nil {
		return fmt.Errorf("expected_metrics.yaml failed validation: %v", err)
	}
	logger.ToMainLog().Printf("Parsed expected_metrics.yaml: %+v", metrics)
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
		break
	}
	// Give some catch-up time to the remaining metrics, which tend to be configured
	// for a 60-second interval and sometimes require two data points.
	logger.ToMainLog().Println("Found representative metric, sleeping before checking remaining metrics")
	time.Sleep(120 * time.Second)
	c := make(chan error, len(metrics))
	for _, metric := range metrics {
		if metric.Representative {
			c <- nil
			continue
		}
		metric := metric
		go func() {
			c <- assertMetric(ctx, logger, vm, metric)
		}()
	}
	for range metrics {
		err = multierr.Append(err, <-c)
	}
	return err
}

// validateMetrics checks that all enum fields have valid values and that
// there is exactly one representative metric in the slice
func validateMetrics(metrics []expectedMetric) error {
	var err error
	// Field validation
	v := newValidator()
	for _, metric := range metrics {
		if validatorErr := rewriteEnumErrors(v.Struct(metric)); validatorErr != nil {
			err = multierr.Append(err, fmt.Errorf("%s: %v", metric.Type, validatorErr))
		}
	}
	// Slice validation
	representativeCount := 0
	for _, metric := range metrics {
		if metric.Representative {
			representativeCount += 1
			if metric.Optional {
				err = multierr.Append(err, fmt.Errorf("%s: metric cannot be both representative and optional", metric.Type))
			}
		}
	}
	if representativeCount != 1 {
		err = multierr.Append(err, fmt.Errorf("there must be exactly one metric with representative: true, but %v were found", representativeCount))
	}
	return err
}

func assertMetric(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM, metric expectedMetric) error {
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

func assertMetricLabels(metric expectedMetric, series *monitoringpb.TimeSeries) error {
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
func runSingleTest(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM, agentType, app string) (retry bool, err error) {
	folder, err := distroFolder(vm.Platform)
	if err != nil {
		return nonRetryable, err
	}
	if _, err = runScriptFromScriptsDir(
		ctx, logger, vm, path.Join("applications", app, folder, "install"), nil); err != nil {
		return retryable, fmt.Errorf("error installing %s: %v", app, err)
	}

	if shouldRetry, err := installAgent(ctx, logger, vm, agentType); err != nil {
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

	// Check if expected_logs.yaml exists, and run the test cases if it does.
	if testCaseBytes, err := readFileFromScriptsDir(path.Join("applications", app, "expected_logs.yaml")); err == nil {
		logger.ToMainLog().Println("found expected_logs.yaml, running logging test cases...")
		if err = runLoggingTestCases(ctx, logger, vm, testCaseBytes); err != nil {
			return nonRetryable, err
		}
	}

	// Check if expected_metrics.yaml exists, and run the test cases if it does.
	if testCaseBytes, err := readFileFromScriptsDir(path.Join("applications", app, "expected_metrics.yaml")); err == nil {
		logger.ToMainLog().Println("found expected_metrics.yaml, running metrics test cases...")
		if err = runMetricsTestCases(ctx, logger, vm, testCaseBytes); err != nil {
			return nonRetryable, err
		}
	}

	return nonRetryable, nil
}

// Returns the authoritative set of all apps available for testing.
// The authoritative list corresponds to the directory names under
// integration_test/third_party_apps_data/applications
func determineAllApps(t *testing.T) map[string]bool {
	allApps := make(map[string]bool)

	files, err := os.ReadDir("third_party_apps_data/applications")
	if err != nil {
		t.Fatalf("got error listing files under third_party_apps_data/applications: %v", err)
	}
	for _, file := range files {
		if file.IsDir() {
			allApps[file.Name()] = true
		}
	}
	log.Printf("all apps: %v", allApps)
	return allApps
}

func modifiedFiles(t *testing.T) []string {
	cmd := exec.Command("git", "diff", "--name-only", "origin/master")
	out, err := cmd.Output()
	log.Printf("git diff output:\n\tstdout:%v\n\tstderr:%v\n", out, err)
	if err != nil {
		t.Fatalf("got error calling `git diff`: %v", err)
	}

	return strings.Split(string(out), "\n")
}

// Determine what apps are impacted by current code changes.
// Extracts app names as follows:
//   apps/<appname>.go
//   integration_test/third_party_apps_data/<appname>/
// Checks the extracted app names against the set of all known apps.
func determineImpactedApps(mf []string, allApps map[string]bool) map[string]bool {
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
	skipReason string
}

var defaultPlatforms = map[string]bool{
	"debian-10":    true,
	"windows-2019": true,
}

// When in `-short` test mode, mark some tests for skipping, based on
// test_config and impacted apps.  Always test all apps against the default
// platform.  If a subset of apps is determined to be impacted, also test all
// platforms for those apps.
func determineTestsToSkip(tests []test, impactedApps map[string]bool, testConfig testConfig) {
	for i, test := range tests {
		if testing.Short() {
			_, testApp := impactedApps[test.app]
			_, defaultPlatform := defaultPlatforms[test.platform]
			if !defaultPlatform && !testApp {
				tests[i].skipReason = fmt.Sprintf("skipping %v because it's not impacted by pending change", test.app)
			}
		}
		if test.app == "mysql" {
			// TODO(b/215197805): Reenable this test once the repos are fixed.
			tests[i].skipReason = "mysql repos seem to be totally broken, see b/215197805"
		}
		if sliceContains(testConfig.PerApplicationOverrides[test.app].PlatformsToSkip, test.platform) {
			tests[i].skipReason = "Skipping test due to 'platforms_to_skip' entry in test_config.yaml"
		}
	}
}

// This is the entry point for the test. Runs runSingleTest
// for each platform in PLATFORMS and each app in linuxApps or windowsApps.
func TestThirdPartyApps(t *testing.T) {
	t.Cleanup(gce.CleanupKeysOrDie)

	if scriptsDir == "" {
		t.Fatalf("Cannot run test with empty value of SCRIPTS_DIR.")
	}
	agentType := agents.OpsAgentType

	testConfig, err := parseTestConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	tests := []test{}
	platforms := strings.Split(os.Getenv("PLATFORMS"), ",")
	for _, platform := range platforms {
		apps, err := appsToTest(agentType, platform)
		if err != nil {
			t.Fatalf("Error when reading list of apps to test for agentType=%v, platform=%v. err=%v", agentType, platform, err)
		}
		if len(apps) == 0 {
			t.Fatalf("Found no applications when testing agentType=%v, platform=%v", agentType, platform)
		}
		for _, app := range apps {
			tests = append(tests, test{platform, app, ""})
		}
	}

	// Filter tests
	determineTestsToSkip(tests, determineImpactedApps(modifiedFiles(t), determineAllApps(t)), testConfig)

	// Execute tests
	for _, tc := range tests {
		tc := tc // https://golang.org/doc/faq#closures_and_goroutines
		t.Run(tc.platform, func(t *testing.T) {
			t.Parallel()
			t.Run(tc.app, func(t *testing.T) {
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
					vm := gce.SetupVM(ctx, t, logger.ToFile("VM_initialization.txt"), gce.VMOptions{Platform: tc.platform, MachineType: agents.RecommendedMachineType(tc.platform)})
					logger.ToMainLog().Printf("VM is ready: %#v", vm)

					var retryable bool
					retryable, err = runSingleTest(ctx, logger, vm, agentType, tc.app)
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
		})
	}
}
