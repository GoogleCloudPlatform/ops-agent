//go:build integration_test

/*
Package agents holds various helpers for interacting with the agents on GCE
instances.

Note: This file contains various things that appear unused, but in the future
we would like them to be used for a few tests inside Google's code silo. So
please don't delete things just because they are unused within the Ops Agent
repo.
*/
package agents

import (
	"context"
	"fmt"
	"log"
	"path"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/integration_test/gce"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/logging"

	"github.com/blang/semver"
	"github.com/cenkalti/backoff/v4"
	"go.uber.org/multierr"
)

// TrailingQueryWindow represents how far into the past to look when querying
// for uptime metrics.
const TrailingQueryWindow = 2 * time.Minute

// AgentPackage represents a thing that we ask OS Config to install for us.
// One agentPackage can expand out and result in multiple AgentServices
// being installed on the VM.
type AgentPackage struct {
	// Passed to the --agent-rules flag of "gcloud ... policies create|update"
	Type string
	// The name of the package that actually gets installed on the VM.
	PackageName string
}

// AgentService represents an agent service/process that will end up running
// on the VM.
type AgentService struct {
	// The name of the systemctl service for this agent.
	ServiceName string
	// The name of the agent according to package managers. The PackageName
	// is what is passed to dpkg/apt/yum/zypper/etc commands.
	PackageName string
	// The name of the agent according to the uptime metric that it uploads to
	// the monitoring backend.
	// If this is "", it means that this service doesn't upload an uptime metric.
	UptimeMetricName string
}

// TestCase holds the name of a test scenario and the packages being tested.
type TestCase struct {
	Name          string
	AgentPackages []AgentPackage
}

// AgentServices expands the list of packages to install into the list of services
// that will end up running on the machine. This is necessary for the ops agent,
// which is one package but results in multiple running agent services.
func AgentServices(t *testing.T, platform string, pkgs []AgentPackage) []AgentService {
	t.Helper()
	if gce.IsWindows(platform) {
		if len(pkgs) != 1 || pkgs[0].Type != OpsAgentType {
			t.Fatalf("AgentServices() assumes that the only package we want to install on Windows is the ops agent. Requested packages: %v", pkgs)
		}
		return []AgentService{
			{
				ServiceName: "google-cloud-ops-agent",
				PackageName: "google-cloud-ops-agent",
				// This service doesn't currently have an uptime metric.
				UptimeMetricName: "",
			},
			{
				ServiceName: "google-cloud-ops-agent-fluent-bit",
				PackageName: "google-cloud-ops-agent-fluent-bit",
				// TODO(b/170138116): Enable this metric once it is being uploaded for
				// Fluent-Bit.
				UptimeMetricName: "",
			},
			{
				ServiceName:      "google-cloud-ops-agent-opentelemetry-collector",
				PackageName:      "google-cloud-ops-agent-opentelemetry-collector",
				UptimeMetricName: "google-cloud-ops-agent-metrics",
			},
		}
	}
	var services []AgentService
	for _, pkg := range pkgs {
		switch pkg.Type {
		case LoggingAgentType:
			services = append(services,
				AgentService{
					ServiceName:      "google-fluentd",
					PackageName:      "google-fluentd",
					UptimeMetricName: "google-fluentd",
				},
			)
		case MetricsAgentType:
			services = append(services,
				AgentService{
					ServiceName:      "stackdriver-agent",
					PackageName:      "stackdriver-agent",
					UptimeMetricName: "stackdriver_agent",
				},
			)
		case OpsAgentType:
			services = append(services,
				AgentService{
					ServiceName: "google-cloud-ops-agent-fluent-bit",
					PackageName: "google-cloud-ops-agent",
					// TODO(b/170138116): Enable this check once the uptime metric is
					// being uploaded for Fluent-Bit.
					UptimeMetricName: "",
				},
				AgentService{
					ServiceName:      "google-cloud-ops-agent-opentelemetry-collector",
					PackageName:      "google-cloud-ops-agent",
					UptimeMetricName: "google-cloud-ops-agent-metrics",
				},
			)
		default:
			t.Fatalf("Package %#v has a Type that is not supported by this test", pkg)
		}
	}
	return services
}

var (
	// LoggingAgentType represents the type var for the Logging Agent.
	LoggingAgentType = "logging"
	// MetricsAgentType represents the type var for the Monitoring Agent.
	MetricsAgentType = "metrics"
	// OpsAgentType represents the type var for the Ops Agent.
	OpsAgentType = "ops-agent"

	// LoggingPackage holds the type var and package name of the Logging Agent.
	LoggingPackage = AgentPackage{Type: LoggingAgentType, PackageName: "google-fluentd"}
	// MetricsPackage holds the type var and package name of the Metrics Agent.
	MetricsPackage = AgentPackage{Type: MetricsAgentType, PackageName: "stackdriver-agent"}
	// OpsPackage holds the type var and package name of the Ops Agent.
	OpsPackage = AgentPackage{Type: OpsAgentType, PackageName: "google-cloud-ops-agent"}

	// This suffix helps Kokoro set the right Content-type for log files. See b/202432085.
	txtSuffix = ".txt"
)

// RunOpsAgentDiagnostics will fetch as much debugging info as it can from the
// given VM. This function assumes that the VM is running the ops agent.
// All the commands run and their output are dumped to various files in the
// directory managed by the given DirectoryLogger.
func RunOpsAgentDiagnostics(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM) {
	if gce.IsWindows(vm.Platform) {
		runOpsAgentDiagnosticsWindows(ctx, logger, vm)
		return
	}
	gce.RunRemotely(ctx, logger.ToFile("systemctl_status_for_ops_agent.txt"), vm, "", "sudo systemctl status google-cloud-ops-agent*")

	gce.RunRemotely(ctx, logger.ToFile("journalctl_output.txt"), vm, "", "sudo journalctl -xe")

	for _, log := range []string{
		gce.SyslogLocation(vm.Platform),
		"/var/log/google-cloud-ops-agent/subagents/logging-module.log",
		"/var/log/google-cloud-ops-agent/subagents/metrics-module.log",
		"/run/google-cloud-ops-agent-fluent-bit/fluent_bit_main.conf",
		"/run/google-cloud-ops-agent-fluent-bit/fluent_bit_parser.conf",
		"/run/google-cloud-ops-agent-opentelemetry-collector/otel.yaml",
	} {
		_, basename := path.Split(log)
		gce.RunRemotely(ctx, logger.ToFile(basename+txtSuffix), vm, "", "sudo cat "+log)
	}
}

func runOpsAgentDiagnosticsWindows(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM) {
	gce.RunRemotely(ctx, logger.ToFile("windows_System_log.txt"), vm, "", "Get-WinEvent -LogName System | Format-Table -AutoSize -Wrap")

	gce.RunRemotely(ctx, logger.ToFile("Get-Service_output.txt"), vm, "", "Get-Service google-cloud-ops-agent* | Format-Table -AutoSize -Wrap")

	gce.RunRemotely(ctx, logger.ToFile("ops_agent_logs.txt"), vm, "", "Get-WinEvent -FilterHashtable @{ Logname='Application'; ProviderName='google-cloud-ops-agent' } | Format-Table -AutoSize -Wrap")
	gce.RunRemotely(ctx, logger.ToFile("open_telemetry_agent_logs.txt"), vm, "", "Get-WinEvent -FilterHashtable @{ Logname='Application'; ProviderName='google-cloud-ops-agent-opentelemetry-collector' } | Format-Table -AutoSize -Wrap")
	// Fluent-Bit has not implemented exporting logs to the Windows event log yet.
	gce.RunRemotely(ctx, logger.ToFile("fluent_bit_agent_logs.txt"), vm, "", fmt.Sprintf("Get-Content -Path '%s' -Raw", `C:\ProgramData\Google\Cloud Operations\Ops Agent\log\logging-module.log`))

	for _, conf := range []string{
		`C:\Program Files\Google\Cloud Operations\Ops Agent\config\config.yaml`,
		`C:\ProgramData\Google\Cloud Operations\Ops Agent\generated_configs\fluentbit\fluent_bit_main.conf`,
		`C:\ProgramData\Google\Cloud Operations\Ops Agent\generated_configs\fluentbit\fluent_bit_parser.conf`,
		`C:\ProgramData\Google\Cloud Operations\Ops Agent\generated_configs\otel\otel.yaml`,
	} {
		pathParts := strings.Split(conf, `\`)
		basename := pathParts[len(pathParts)-1]
		gce.RunRemotely(ctx, logger.ToFile(basename+txtSuffix), vm, "", fmt.Sprintf("Get-Content -Path '%s' -Raw", conf))
	}
}

// WaitForUptimeMetrics waits for the given uptime metrics to be visible in
// the monitoring backend, returning an error if the lookup failed or the
// metrics didn't become visible.
func WaitForUptimeMetrics(ctx context.Context, logger *log.Logger, vm *gce.VM, services []AgentService) error {
	// Run gce.WaitForMetric in parallel for each agent.
	var err error
	c := make(chan error, len(services))
	for _, service := range services {
		service := service // https://golang.org/doc/faq#closures_and_goroutines
		go func() {
			if service.UptimeMetricName == "" {
				logger.Printf("Skipping lookup of uptime metric for %v", service.ServiceName)
				c <- nil
				return
			}
			c <- gce.WaitForMetric(
				ctx, logger, vm, "agent.googleapis.com/agent/uptime", TrailingQueryWindow,
				[]string{fmt.Sprintf("metric.labels.version = starts_with(%q)", service.UptimeMetricName)})
		}()
	}
	for range services {
		err = multierr.Append(err, <-c)
	}
	if err != nil {
		return fmt.Errorf("WaitForUptimeMetrics() error: %v", err)
	}
	return nil
}

// CheckServicesRunning asserts that the given services are currently running
// on the given VM, returning an error if they are not (or if there was an
// error determining their status).
func CheckServicesRunning(ctx context.Context, logger *log.Logger, vm *gce.VM, services []AgentService) error {
	var err error
	for _, service := range services {
		if gce.IsWindows(vm.Platform) {
			output, statusErr := gce.RunRemotely(ctx, logger, vm, "", fmt.Sprintf("(Get-Service -Name '%s').Status", service.ServiceName))
			if statusErr != nil {
				err = multierr.Append(err, fmt.Errorf("no service named %q could be found: %v", service.ServiceName, statusErr))
				continue
			}
			status := strings.TrimSpace(output.Stdout)
			if status != "Running" {
				err = multierr.Append(err, fmt.Errorf(`for service %q, got status=%q, want "Running"`, service.ServiceName, status))
				continue
			}
			continue
		}
		if _, statusErr := gce.RunRemotely(ctx, logger, vm, "", fmt.Sprintf("sudo service %s status", service.ServiceName)); statusErr != nil {
			err = multierr.Append(err, fmt.Errorf("RunRemotely(): status of %s was not OK. error was: %v", service.ServiceName, statusErr))
		}
	}
	if err != nil {
		return fmt.Errorf("CheckServicesRunning() error: %v", err)
	}
	return nil
}

// CheckServicesRunningAndWaitForMetrics asserts that the agent services are
// currently running on the given VM and waits for their uptime metrics to
// become visible.
func CheckServicesRunningAndWaitForMetrics(ctx context.Context, logger *log.Logger, vm *gce.VM, services []AgentService) error {
	logger.Print("Checking that services are running.")
	if err := CheckServicesRunning(ctx, logger, vm, services); err != nil {
		return err
	}
	logger.Print("Waiting for uptime metrics to be visible.")
	if err := WaitForUptimeMetrics(ctx, logger, vm, services); err != nil {
		return err
	}
	logger.Print("Agents are successfully running.")
	return nil
}

// CheckServicesNotRunning asserts that the given services are not running on the
// given VM.
func CheckServicesNotRunning(ctx context.Context, logger *log.Logger, vm *gce.VM, services []AgentService) error {
	logger.Print("Checking that agents are NOT running")
	for _, service := range services {
		if gce.IsWindows(vm.Platform) {
			// There are two possible cases that are acceptable here:
			// 1) the service has been deleted
			// 2) the service still exists but is in some state other than "Running",
			//    such as "Stopped".
			// We currently only see case #1, but let's permit case #2 as well.
			// The following command should output nothing, or maybe just a blank
			// line, in either case.
			output, err := gce.RunRemotely(ctx, logger, vm, "", fmt.Sprintf("Get-Service | Where-Object {$_.Name -eq '%s' -and $_.Status -eq 'Running'}", service.ServiceName))
			if err != nil {
				return fmt.Errorf("CheckServicesNotRunning(): error looking up running services named %v: %v", service.ServiceName, err)
			}
			if strings.TrimSpace(output.Stdout) != "" {
				return fmt.Errorf("CheckServicesNotRunning(): service %v was unexpectedly 'Running'. output: %s", service.ServiceName, output.Stdout)
			}
			continue
		}
		if service.ServiceName == "google-fluentd" {
			continue // TODO(b/159817885): Assert that google-fluentd is not running.
		}
		if _, err := gce.RunRemotely(ctx, logger, vm, "", fmt.Sprintf("! sudo service %s status", service.ServiceName)); err != nil {
			return fmt.Errorf("CheckServicesNotRunning(): command could not be run or status of %s was unexpectedly OK. err: %v", service.ServiceName, err)
		}
	}
	return nil
}

// tryInstallPackages attempts once to install the given packages.
func tryInstallPackages(ctx context.Context, logger *log.Logger, vm *gce.VM, pkgs []string) error {
	pkgsString := strings.Join(pkgs, " ")
	if gce.IsWindows(vm.Platform) {
		_, err := gce.RunRemotely(ctx, logger, vm, "", fmt.Sprintf("googet -noconfirm install %s", pkgsString))
		return err
	}
	cmd := ""
	if strings.HasPrefix(vm.Platform, "centos-") ||
		strings.HasPrefix(vm.Platform, "rhel-") {
		cmd = fmt.Sprintf("sudo yum -y install %s", pkgsString)
	} else if strings.HasPrefix(vm.Platform, "sles-") {
		cmd = fmt.Sprintf("sudo zypper --non-interactive install %s", pkgsString)
	} else if strings.HasPrefix(vm.Platform, "debian-") ||
		strings.HasPrefix(vm.Platform, "ubuntu-") {
		cmd = fmt.Sprintf("sudo apt-get update; sudo apt-get -y install %s", pkgsString)
	} else {
		return fmt.Errorf("unsupported platform: %s", vm.Platform)
	}
	_, err := gce.RunRemotely(ctx, logger, vm, "", cmd)
	return err
}

// InstallPackages installs the given packages on the given VM. Assumes repo is
// configured. This function has some retries to paper over transient issues
// that often happen when installing packages.
func InstallPackages(ctx context.Context, logger *log.Logger, vm *gce.VM, pkgs []string) error {
	attemptFunc := func() error { return tryInstallPackages(ctx, logger, vm, pkgs) }
	if err := RunInstallFuncWithRetry(ctx, logger, vm, attemptFunc); err != nil {
		return fmt.Errorf("could not install %v. err: %v", pkgs, err)
	}
	return nil
}

// UninstallPackages removes the given packages from the given VM.
func UninstallPackages(ctx context.Context, logger *log.Logger, vm *gce.VM, pkgs []string) error {
	pkgsString := strings.Join(pkgs, " ")
	if gce.IsWindows(vm.Platform) {
		if _, err := gce.RunRemotely(ctx, logger, vm, "", fmt.Sprintf("googet -noconfirm remove %s", pkgsString)); err != nil {
			return fmt.Errorf("could not uninstall %s. err: %v", pkgsString, err)
		}
		return nil
	}
	cmd := ""
	if strings.HasPrefix(vm.Platform, "centos-") ||
		strings.HasPrefix(vm.Platform, "rhel-") {
		cmd = fmt.Sprintf("sudo yum -y remove %s", pkgsString)
	} else if strings.HasPrefix(vm.Platform, "sles-") {
		cmd = fmt.Sprintf("sudo zypper --non-interactive remove %s", pkgsString)
	} else if strings.HasPrefix(vm.Platform, "debian-") ||
		strings.HasPrefix(vm.Platform, "ubuntu-") {
		cmd = fmt.Sprintf("sudo apt-get -y remove %s", pkgsString)
	} else {
		return fmt.Errorf("unsupported platform: %s", vm.Platform)
	}
	if _, err := gce.RunRemotely(ctx, logger, vm, "", cmd); err != nil {
		return fmt.Errorf("could not uninstall %s. err: %v", pkgsString, err)
	}
	return nil
}

// checkPackages asserts that the given packages on the given VM are in a state matching the installed argument.
func checkPackages(ctx context.Context, logger *log.Logger, vm *gce.VM, pkgs []string, installed bool) error {
	errPrefix := ""
	if installed {
		errPrefix = "not "
	}
	if gce.IsWindows(vm.Platform) {
		output, err := gce.RunRemotely(ctx, logger, vm, "", "googet installed")
		if err != nil {
			return err
		}
		for _, pkg := range pkgs {
			// Look for the package name in the output. \b means word boundary.
			expr := `\b` + pkg + `\b`
			re, err := regexp.Compile(expr)
			if err != nil {
				return fmt.Errorf("regexp %q failed to compile: %v", expr, err)
			}
			if m := re.FindString(output.Stdout); (m == "") == installed {
				return fmt.Errorf("package %q was unexpectedly %sinstalled", pkg, errPrefix)
			}
		}
		// Success: the expected set of packages was found in the output of "googet installed".
		return nil
	}
	for _, pkg := range pkgs {
		cmd := ""
		if IsRPMBased(vm.Platform) {
			cmd = fmt.Sprintf("rpm --query %s", pkg)
			if !installed {
				cmd = "! " + cmd
			}
		} else if strings.HasPrefix(vm.Platform, "debian-") ||
			strings.HasPrefix(vm.Platform, "ubuntu-") {
			// dpkg's package states are documented in "man 1 dpkg".
			// "config-files" means that the package is not installed but its config files
			// are still on the system. Accepting both that and "not-installed" is more
			// future-proof than just accepting "config-files".
			cmd = fmt.Sprintf(
				`(dpkg-query --show '--showformat=${db:Status-Status}' %s || echo 'not-installed') | grep --extended-regexp '(not-installed)|(config-files)'`, pkg)
			if installed {
				cmd = "! (" + cmd + ")"
			}
		} else {
			return fmt.Errorf("checkPackages() does not support platform: %s", vm.Platform)
		}
		if _, err := gce.RunRemotely(ctx, logger, vm, "", cmd); err != nil {
			return fmt.Errorf("command could not be run or %q was unexpectedly %sinstalled. err: %v", pkg, errPrefix, err)
		}
	}
	return nil
}

// CheckPackagesNotInstalled asserts that the given packages are not installed on the given VM.
func CheckPackagesNotInstalled(ctx context.Context, logger *log.Logger, vm *gce.VM, pkgs []string) error {
	return checkPackages(ctx, logger, vm, pkgs, false)
}

// CheckPackagesInstalled asserts that the given packages are installed on the given VM.
func CheckPackagesInstalled(ctx context.Context, logger *log.Logger, vm *gce.VM, pkgs []string) error {
	return checkPackages(ctx, logger, vm, pkgs, true)
}

// CheckAgentsUninstalled asserts that the given agent services and packages
// are not installed on the given VM.
func CheckAgentsUninstalled(ctx context.Context, logger *log.Logger, vm *gce.VM, services []AgentService) error {
	if err := CheckServicesNotRunning(ctx, logger, vm, services); err != nil {
		return err
	}
	var pkgs []string
	for _, service := range services {
		pkgs = append(pkgs, service.PackageName)
	}
	if err := CheckPackagesNotInstalled(ctx, logger, vm, pkgs); err != nil {
		return err
	}
	// TODO(martijnvs): Also check that the ops agent package itself is uninstalled.
	return nil
}

// IsRPMBased checks if the platform is RPM based.
func IsRPMBased(platform string) bool {
	return strings.HasPrefix(platform, "centos-") ||
		strings.HasPrefix(platform, "rhel-") ||
		strings.HasPrefix(platform, "rocky-linux-") ||
		strings.HasPrefix(platform, "sles-") ||
		strings.HasPrefix(platform, "opensuse-")
}

// StripTildeSuffix strips off everything after the first ~ character. We see
// version numbers with tildes on debian (e.g. 1.0.1~debian10) and the semver
// library doesn't parse them out properly.
func StripTildeSuffix(version string) string {
	ind := strings.Index(version, "~")
	if ind == -1 {
		return version
	}
	return version[:ind]
}

func fetchPackageVersionWindows(ctx context.Context, logger *log.Logger, vm *gce.VM, pkg string) (semver.Version, error) {
	output, err := gce.RunRemotely(ctx, logger, vm, "", "googet installed -info "+pkg)
	if err != nil {
		return semver.Version{}, err
	}
	// Look for a line in the output with a version number. Some real examples:
	// "      Version      : 20201229.01.0+win@1"
	// "      Version      : 1.0.6@1"
	re, err := regexp.Compile(`\s*Version\s*:\s*([0-9]+\.[0-9]+\.[0-9]+)[^0-9]+.*`)
	if err != nil {
		return semver.Version{}, fmt.Errorf("Could not compile regular expression: %v", err)
	}

	lines := strings.Split(output.Stdout, "\r\n")
	var version string
	for _, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) > 1 {
			version = matches[1]
			break
		}
	}
	if version == "" {
		return semver.Version{}, fmt.Errorf("Could not parse version from googet output: %v", output.Stdout)
	}
	return semver.Make(version)
}

// fetchPackageVersion runs a googet/dpkg/rpm command remotely to get the
// installed version of a package. It has some retries to accommodate the fact
// that sometimes it takes ~1 minute between when the agents are up and running
// and the time that the package shows up as installed with rpm --query
// (b/178096139).
// TODO(martijnvs): Understand why b/178096139 happens and remove the retries.
func fetchPackageVersion(ctx context.Context, logger *log.Logger, vm *gce.VM, pkg string) (semver.Version, error) {
	logger.Printf("Getting %s version", pkg)
	if gce.IsWindows(vm.Platform) {
		return fetchPackageVersionWindows(ctx, logger, vm, pkg)
	}
	var version semver.Version
	tryGetVersion := func() error {
		cmd := `dpkg-query --show --showformat=\$\{Version} ` + pkg
		if IsRPMBased(vm.Platform) {
			cmd = `rpm --query --queryformat=%\{VERSION} ` + pkg
		}
		output, err := gce.RunRemotely(ctx, logger, vm, "", cmd)
		if err != nil {
			return err
		}
		vStr := strings.TrimSpace(output.Stdout)
		if !IsRPMBased(vm.Platform) {
			vStr = StripTildeSuffix(vStr)
		}
		version, err = semver.Make(vStr)
		if err != nil {
			err = fmt.Errorf("semver.Make() could not convert %q to a Version: %v", vStr, err)
			logger.Print(err)
			return err
		}
		return nil
	}
	backoffPolicy := backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(20*time.Second), 10), ctx)
	if err := backoff.Retry(tryGetVersion, backoffPolicy); err != nil {
		return semver.Version{}, err
	}
	return version, nil
}

// FetchPackageVersions retrieves the semver.Version on the given VM for all the agents
// in the given []agents.AgentPackage.
func FetchPackageVersions(ctx context.Context, logger *log.Logger, vm *gce.VM, packages []AgentPackage) (map[AgentPackage]semver.Version, error) {
	versions := make(map[AgentPackage]semver.Version)
	var err error
	for _, pkg := range packages {
		current, versionErr := fetchPackageVersion(ctx, logger, vm, pkg.PackageName)
		logger.Printf("fetchPackageVersion() returned version=%v, err=%v", current, versionErr)
		if versionErr != nil {
			err = multierr.Append(err, fmt.Errorf("fetchPackageVersion(%q) failed: %v", pkg.PackageName, versionErr))
			continue
		}
		versions[pkg] = current
	}
	return versions, err
}

// isRetriableInstallError checks to see if the error may be transient.
func isRetriableInstallError(platform string, err error) bool {
	if strings.Contains(err.Error(), "Could not refresh zypper repositories.") ||
		strings.Contains(err.Error(), "Credentials are invalid") ||
		strings.Contains(err.Error(), "Resource temporarily unavailable") ||
		strings.Contains(err.Error(), "System management is locked by the application") {
		return true
	}
	if gce.IsWindows(platform) &&
		strings.Contains(err.Error(), "context deadline exceeded") {
		return true // See b/197127877 for history.
	}
	if platform == "rhel-8-1-sap-ha" &&
		strings.Contains(err.Error(), "Could not refresh the google-cloud-ops-agent yum repositories") {
		return true // See b/174039270 for history.
	}
	if platform == "rhel-8-1-sap-ha" &&
		strings.Contains(err.Error(), "Failed to download metadata for repo 'rhui-rhel-8-") {
		return true // This happens when the RHEL servers are down. See b/189950957.
	}
	if strings.HasPrefix(platform, "rhel-") && strings.Contains(err.Error(), "SSL_ERROR_SYSCALL") {
		return true // b/187661497. Example: screen/3PMwAvhNBKWVYub
	}
	if strings.HasPrefix(platform, "rhel-") && strings.Contains(err.Error(), "Encountered end of file") {
		return true // b/184729120#comment31. Example: screen/4yK9evoY68LiaLr
	}
	if strings.HasPrefix(platform, "ubuntu-") && strings.Contains(err.Error(), "Clearsigned file isn't valid") {
		// The upstream repo was in an inconsistent state. The error looks like:
		// screen/7U24zrRwADyYKqb
		return true
	}
	if strings.HasPrefix(platform, "ubuntu-") && strings.Contains(err.Error(), "Mirror sync in progress?") {
		// The mirror repo was in an inconsistent state. The error looks like:
		// http://screen/Ai2CHc7fcRosHJu
		return true
	}
	if strings.HasPrefix(platform, "ubuntu-") && strings.Contains(err.Error(), "Hash Sum mismatch") {
		// The mirror repo was in an inconsistent state. The error looks like:
		// http://screen/8HjakedwVnXZvw6
		return true
	}
	return false
}

// RunInstallFuncWithRetry runs the given installFunc, retrying errors that
// seem transient. In between attempts, this function may attempt to recover
// from certain errors.
// This function is intended to paper over various problems that arise when
// running apt/zypper/yum. installFunc can be any function that is invoking
// apt/zypper/yum, directly or indirectly.
func RunInstallFuncWithRetry(ctx context.Context, logger *log.Logger, vm *gce.VM, installFunc func() error) error {
	shouldRetry := func(err error) bool { return isRetriableInstallError(vm.Platform, err) }
	installWithRecovery := func() error {
		err := installFunc()
		if err != nil && shouldRetry(err) && vm.Platform == "rhel-8-1-sap-ha" {
			logger.Println("attempting recovery steps from https://access.redhat.com/discussions/4656371 so that subsequent attempts are more likely to succeed... see b/189950957")
			gce.RunRemotely(ctx, logger, vm, "", "sudo dnf clean all && sudo rm -r /var/cache/dnf && sudo dnf upgrade")
		}
		if err != nil && !shouldRetry(err) {
			err = backoff.Permanent(err)
		}
		// Returning a non-permanent error triggers retries.
		return err
	}
	backoffPolicy := backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 5), ctx)
	return backoff.Retry(installWithRecovery, backoffPolicy)
}

// InstallStandaloneWindowsLoggingAgent installs the Stackdriver Logging agent
// on a Windows VM.
func InstallStandaloneWindowsLoggingAgent(ctx context.Context, logger *log.Logger, vm *gce.VM) error {
	// https://cloud.google.com/logging/docs/agent/installation#joint-install
	cmd := `(New-Object Net.WebClient).DownloadFile("https://dl.google.com/cloudagents/windows/StackdriverLogging-v1-16.exe", "${env:UserProfile}\StackdriverLogging-v1-16.exe")
		& "${env:UserProfile}\StackdriverLogging-v1-16.exe" /S`
	_, err := gce.RunRemotely(ctx, logger, vm, "", cmd)
	return err
}

// InstallStandaloneWindowsMonitoringAgent installs the Stackdriver Monitoring
// agent on a Windows VM.
func InstallStandaloneWindowsMonitoringAgent(ctx context.Context, logger *log.Logger, vm *gce.VM) error {
	// https://cloud.google.com/monitoring/agent/installation#joint-install
	cmd := `(New-Object Net.WebClient).DownloadFile("https://repo.stackdriver.com/windows/StackdriverMonitoring-GCM-46.exe", "${env:UserProfile}\StackdriverMonitoring-GCM-46.exe")
		& "${env:UserProfile}\StackdriverMonitoring-GCM-46.exe" /S`
	_, err := gce.RunRemotely(ctx, logger, vm, "", cmd)
	return err
}

// CommonSetup sets up the VM for testing.
func CommonSetup(t *testing.T, platform string) (context.Context, *logging.DirectoryLogger, *gce.VM) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), gce.SuggestedTimeout)
	t.Cleanup(cancel)

	logger := gce.SetupLogger(t)
	logger.ToMainLog().Println("Calling SetupVM(). For details, see VM_initialization.txt.")
	vm := gce.SetupVM(ctx, t, logger.ToFile("VM_initialization.txt"), gce.VMOptions{Platform: platform})
	logger.ToMainLog().Printf("VM is ready: %#v", vm)
	t.Cleanup(func() {
		if t.Failed() {
			RunOpsAgentDiagnostics(ctx, logger, vm)
		}
	})
	return ctx, logger, vm
}

func globForAgentPackage(platform string) (string, error) {
	if gce.IsWindows(platform) {
		return "*.goo", nil
	}

	// Here is a real example of what package names look like once built:
	// google-cloud-ops-agent-2.0.3-1.el7.x86_64.rpm
	// google-cloud-ops-agent-2.0.3-1.el8.x86_64.rpm
	// google-cloud-ops-agent-2.0.3-1.sles12.x86_64.rpm
	// google-cloud-ops-agent-2.0.3-1.sles15.x86_64.rpm
	// google-cloud-ops-agent_2.0.3~debian10_amd64.deb
	// google-cloud-ops-agent_2.0.3~debian9.13_amd64.deb
	// google-cloud-ops-agent_2.0.3~ubuntu16.04_amd64.deb
	// google-cloud-ops-agent_2.0.3~ubuntu18.04_amd64.deb
	// google-cloud-ops-agent_2.0.3~ubuntu20.04_amd64.deb
	//
	// The goal of this function is to convert what we have (vm.Platform, e.g.
	// debian-10)	into a glob that will pick out the appropriate package file for
	// that distro.

	// I honestly can't think of a better way to do this.
	switch {
	case strings.HasPrefix(platform, "centos-7") || strings.HasPrefix(platform, "rhel-7"):
		return "*.el7.*.rpm", nil
	case strings.HasPrefix(platform, "centos-8") || strings.HasPrefix(platform, "rhel-8") || strings.HasPrefix(platform, "rocky-linux-8"):
		return "*.el8.*.rpm", nil
	case strings.HasPrefix(platform, "sles-12"):
		return "*.sles12.*.rpm", nil
	case strings.HasPrefix(platform, "sles-15") || strings.HasPrefix(platform, "opensuse-leap"):
		return "*.sles15.*.rpm", nil
	case platform == "debian-9":
		return "*~debian9*.deb", nil
	case platform == "debian-10":
		return "*~debian10*.deb", nil
	case platform == "debian-11":
		return "*~debian11*.deb", nil
	case platform == "ubuntu-1804-lts" || platform == "ubuntu-minimal-1804-lts":
		return "*~ubuntu18.04_*.deb", nil
	case platform == "ubuntu-2004-lts" || platform == "ubuntu-minimal-2004-lts":
		return "*~ubuntu20.04_*.deb", nil
	case platform == "ubuntu-2104" || platform == "ubuntu-minimal-2104":
		return "*~ubuntu21.04_*.deb", nil
	default:
		return "", fmt.Errorf("agents.go does not know how to convert platform %q into a glob that can pick the appropriate package out of a lineup", platform)
	}
}

// InstallPackageFromGCS installs the agent package from GCS onto the given Linux VM.
//
// gcsPath must point to a GCS Path that contains .deb/.rpm/.goo files to install on the testing VMs. Each agent
// must have its own subdirectory. For example this would be
// a valid structure inside AGENT_PACKAGES_IN_GCS when testing metrics & ops-agent:
// ├── metrics
// │   ├── collectd-4.5.6.deb
// │   ├── collectd-4.5.6.rpm
// │   └── otel-collector-0.1.2.goo
// └── ops-agent
//     ├── ops-agent-google-cloud-1.2.3.deb
//     ├── ops-agent-google-cloud-1.2.3.rpm
//     └── ops-agent-google-cloud-1.2.3.goo
func InstallPackageFromGCS(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM, agentType string, gcsPath string) error {
	if gce.IsWindows(vm.Platform) {
		return installWindowsPackageFromGCS(ctx, logger, vm, agentType, gcsPath)
	}
	if _, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, "", "mkdir -p /tmp/agentUpload"); err != nil {
		return err
	}
	glob, err := globForAgentPackage(vm.Platform)
	if err != nil {
		return err
	}

	if err := gce.InstallGsutilIfNeeded(ctx, logger.ToMainLog(), vm); err != nil {
		return err
	}
	if _, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, "", fmt.Sprintf("sudo gsutil cp -r %s/%s/%s /tmp/agentUpload", gcsPath, agentType, glob)); err != nil {
		logger.ToMainLog().Printf("picking agent package using glob %q", glob)
		return fmt.Errorf("error copying down agent package from GCS: %v", err)
	}
	if IsRPMBased(vm.Platform) {
		if _, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, "", "sudo rpm -i /tmp/agentUpload/*"); err != nil {
			return fmt.Errorf("error installing agent from .rpm file: %v", err)
		}
		return nil
	}
	if _, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, "", "sudo apt install /tmp/agentUpload/*"); err != nil {
		return fmt.Errorf("error installing agent from .deb file: %v", err)
	}
	return nil
}

// Installs the agent package from GCS (see packagesInGCS) onto the given Windows VM.
func installWindowsPackageFromGCS(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM, agentType string, gcsPath string) error {
	if _, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, "", "New-Item -ItemType directory -Path C:\\agentUpload"); err != nil {
		return err
	}
	if _, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, "", fmt.Sprintf("gsutil cp -r %s/%s/*.goo C:\\agentUpload", gcsPath, agentType)); err != nil {
		return fmt.Errorf("error copying down agent package from GCS: %v", err)
	}
	if _, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, "", "googet -noconfirm install (Get-ChildItem C:\\agentUpload\\*.goo | Select-Object -Expand FullName)"); err != nil {
		return fmt.Errorf("error installing agent from .goo file: %v", err)
	}
	return nil
}
