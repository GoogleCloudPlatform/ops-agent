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
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/integration_test/agents"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/gce"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/logging"

	"go.uber.org/multierr"
	"gopkg.in/yaml.v2"
)

var (
	scriptsDir    = os.Getenv("SCRIPTS_DIR")
	packagesInGCS = os.Getenv("AGENT_PACKAGES_IN_GCS")
)

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

// expectedEntries encodes a series of assertions about what data we expect to
// to see in the logging backend.
type expectedEntries struct {
	// Note on tags: the "yaml" tag specifies the name of this field in the
	// .yaml file.
	LogEntries []expectedEntry `yaml:"log_entries"`
}
type expectedEntry struct {
	LogName string `yaml:"log_name"`
	// Map of field name to a regex that is expected to match the field value.
	// For example, {"jsonPayload.message": ".*access denied.*"}.
	FieldMatchers map[string]string `yaml:"field_matchers"`
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
	var entries expectedEntries
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

// shouldSkip returns a reason that the given pair of application and platform
// should be skipped, or "" if it should not be skipped.
// New applications should return skip reasons sparingly if at all.
func shouldSkip(app, platform string) string {
	switch app {
	case "apache", "cassandra", "jvm", "mysql", "nginx", "redis":
		if platform != "debian-10" {
			return "application without multi-platform support implemented yet"
		}
		return ""
	case "wildfly":
		if platform != "debian-10" {
			// As wildfly does not have package installers & is installed from tar,
			// we only want to test on one distribution to help reduce integration test size
			return "installed from tar, no need to test across platforms"
		}
		return ""
	case "couchdb":
		if gce.IsSUSE(platform) {
			return "couchdb is not supported on SuSE"
		}
		return ""
	case "rabbitmq":
		if platform == "sles-12" {
			return "rabbitmq is not supported on sles-12"
		}
		return ""
	}
	return ""
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

	if _, err = runScriptFromScriptsDir(
		ctx, logger, vm, path.Join("applications", app, folder, "post"), nil); err != nil {
		return retryable, fmt.Errorf("error starting %s: %v", app, err)
	}

	if _, err = runScriptFromScriptsDir(ctx, logger, vm, path.Join("agent", agentType, osFolder(vm.Platform), "enable_"+app), nil); err != nil {
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
	testCaseBytes, err := readFileFromScriptsDir(path.Join("applications", app, "expected_logs.yaml"))
	if err == nil {
		logger.ToMainLog().Println("found expected_logs.yaml, running logging test cases...")
		if err = runLoggingTestCases(ctx, logger, vm, testCaseBytes); err != nil {
			return nonRetryable, err
		}
	}

	metricName, err := findMetricName(app)
	if err != nil {
		return nonRetryable, fmt.Errorf("error finding metric name for %v: %v", app, err)
	}
	if metricName == "" {
		logger.ToMainLog().Println("metric_name.txt is empty, skipping metrics testing...")
		return nonRetryable, nil
	}
	// Assert that the right metric has been uploaded for the given instance
	// at least once in the last hour.
	if err = gce.WaitForMetric(ctx, logger.ToMainLog(), vm, metricName, 1*time.Hour, nil); err != nil {
		return nonRetryable, err
	}
	return nonRetryable, nil
}

// This is the entry point for the test. Runs runSingleTest
// for each platform in PLATFORMS and each app in linuxApps or windowsApps.
func TestThirdPartyApps(t *testing.T) {
	t.Cleanup(gce.CleanupKeysOrDie)

	if scriptsDir == "" {
		t.Fatalf("Cannot run test with empty value of SCRIPTS_DIR.")
	}
	agentType := agents.OpsAgentType

	platforms := strings.Split(os.Getenv("PLATFORMS"), ",")
	for _, platform := range platforms {
		platform := platform // https://golang.org/doc/faq#closures_and_goroutines
		t.Run(platform, func(t *testing.T) {
			t.Parallel()

			apps, err := appsToTest(agentType, platform)
			if err != nil {
				t.Fatalf("Error when reading list of apps to test for agentType=%v, platform=%v. err=%v", agentType, platform, err)
			}
			if len(apps) == 0 {
				t.Fatalf("Found no applications when testing agentType=%v, platform=%v", agentType, platform)
			}
			for _, app := range apps {
				app := app // https://golang.org/doc/faq#closures_and_goroutines
				t.Run(app, func(t *testing.T) {
					t.Parallel()

					if reason := shouldSkip(app, platform); reason != "" {
						t.Skip(reason)
					}
					ctx, cancel := context.WithTimeout(context.Background(), gce.SuggestedTimeout)
					defer cancel()

					var err error
					for attempt := 1; attempt <= 4; attempt++ {
						logger := gce.SetupLogger(t)
						logger.ToMainLog().Println("Calling SetupVM(). For details, see VM_initialization.txt.")
						vm := gce.SetupVM(ctx, t, logger.ToFile("VM_initialization.txt"), gce.VMOptions{Platform: platform, MachineType: agents.RecommendedMachineType(platform)})
						logger.ToMainLog().Printf("VM is ready: %#v", vm)

						var retryable bool
						retryable, err = runSingleTest(ctx, logger, vm, agentType, app)
						log.Printf("Attempt %v of %s test of %s finished with err=%v, retryable=%v", attempt, platform, app, err, retryable)
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
		})
	}
}
