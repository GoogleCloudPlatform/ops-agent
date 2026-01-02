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
	"embed"
	"fmt"
	"log"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/opentelemetry-operations-collector/integration_test/gce-testing-internal/gce"
	"github.com/GoogleCloudPlatform/opentelemetry-operations-collector/integration_test/gce-testing-internal/logging"

	"github.com/blang/semver"
	"github.com/cenkalti/backoff/v4"
	"go.uber.org/multierr"
)

// TrailingQueryWindow represents how far into the past to look when querying
// for uptime metrics.
const TrailingQueryWindow = 2 * time.Minute

// OpsAgentPluginServerPort defines the port on which the Ops Agent UAP Plugin gRPC server runs.
const OpsAgentPluginServerPort = "1234"

//go:embed testdata
var scriptsDir embed.FS

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
func AgentServices(t *testing.T, imageSpec string, pkgs []AgentPackage) []AgentService {
	t.Helper()
	if gce.IsWindows(imageSpec) {
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
	logger.ToMainLog().Printf("Starting RunOpsAgentDiagnostics()...")
	if gce.IsWindows(vm.ImageSpec) {
		runOpsAgentDiagnosticsWindows(ctx, logger, vm)
		return
	}
	// Tests like TestPortsAndAPIHealthChecks will make these curl operations
	// hang, so give them a shorter timeout to avoid hanging the whole test.
	metricsCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	gce.RunRemotely(metricsCtx, logger.ToFile("fluent_bit_metrics.txt"), vm, "sudo curl -s localhost:20202/metrics")
	gce.RunRemotely(metricsCtx, logger.ToFile("otel_metrics.txt"), vm, "sudo curl -s localhost:20201/metrics")

	isUAPPlugin := gce.IsOpsAgentUAPPlugin()
	if isUAPPlugin {
		gce.RunRemotely(ctx, logger.ToFile("status_for_ops_agent_uap_plugin.txt"), vm, fmt.Sprintf("grpcurl -plaintext -d '{}' localhost:%s plugin_comm.GuestAgentPlugin/GetStatus", OpsAgentPluginServerPort))
	} else {
		gce.RunRemotely(ctx, logger.ToFile("systemctl_status_for_ops_agent.txt"), vm, "sudo systemctl status google-cloud-ops-agent*")
	}

	gce.RunRemotely(ctx, logger.ToFile("journalctl_output.txt"), vm, "sudo journalctl -xe")

	fileList := getOpsAgentLogFilesList(vm.ImageSpec)
	for _, log := range fileList {
		_, basename := path.Split(log)
		gce.RunRemotely(ctx, logger.ToFile(basename+txtSuffix), vm, "sudo cat "+log)
	}
}

func getOpsAgentLogFilesList(imageSpec string) []string {
	if gce.IsOpsAgentUAPPlugin() {
		return []string{
			gce.SyslogLocation(imageSpec),
			OpsAgentConfigPath(imageSpec),
			"/var/lib/google-guest-agent/agent_state/plugins/ops-agent-plugin/log/google-cloud-ops-agent/health-checks.log",
			"/var/lib/google-guest-agent/agent_state/plugins/ops-agent-plugin/log/google-cloud-ops-agent/subagents/logging-module.log",
			"/var/lib/google-guest-agent/agent_state/plugins/ops-agent-plugin/log/google-cloud-ops-agent/subagents/metrics-module.log",
			"/var/lib/google-guest-agent/agent_state/plugins/ops-agent-plugin/log/nvidia-installer.log",
			"/var/lib/google-guest-agent/agent_state/plugins/ops-agent-plugin/run/google-cloud-ops-agent-fluent-bit/fluent_bit_main.conf",
			"/var/lib/google-guest-agent/agent_state/plugins/ops-agent-plugin/run/google-cloud-ops-agent-fluent-bit/fluent_bit_parser.conf",
			"/var/lib/google-guest-agent/agent_state/plugins/ops-agent-plugin/run/google-cloud-ops-agent-opentelemetry-collector/otel.yaml",
			"/var/lib/google-guest-agent/agent_state/plugins/ops-agent-plugin/run/google-cloud-ops-agent-opentelemetry-collector/feature_tracking_otlp.json",
			"/var/lib/google-guest-agent/agent_state/plugins/ops-agent-plugin/run/google-cloud-ops-agent-opentelemetry-collector/enabled_receivers_otlp.json",
		}
	}
	return []string{
		gce.SyslogLocation(imageSpec),
		OpsAgentConfigPath(imageSpec),
		"/var/log/google-cloud-ops-agent/health-checks.log",
		"/var/log/google-cloud-ops-agent/subagents/logging-module.log",
		"/var/log/google-cloud-ops-agent/subagents/metrics-module.log",
		"/var/log/nvidia-installer.log",
		"/run/google-cloud-ops-agent-fluent-bit/fluent_bit_main.conf",
		"/run/google-cloud-ops-agent-fluent-bit/fluent_bit_parser.conf",
		"/run/google-cloud-ops-agent-opentelemetry-collector/otel.yaml",
		"/run/google-cloud-ops-agent-opentelemetry-collector/feature_tracking_otlp.json",
		"/run/google-cloud-ops-agent-opentelemetry-collector/enabled_receivers_otlp.json",
	}
}

func runOpsAgentDiagnosticsWindows(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM) {
	stateDir := `C:\ProgramData\Google\Cloud Operations\Ops Agent\`
	if gce.IsOpsAgentUAPPlugin() {
		stateDir = `C:\ProgramData\Google\Compute Engine\google-guest-agent\agent_state\plugins\ops-agent-plugin\`
		gce.RunRemotely(ctx, logger.ToFile("ops_agent_uap_plugin_logs.txt"), vm, "Get-WinEvent -FilterHashtable @{ Logname='Application'; ProviderName='google-cloud-ops-agent-uap-plugin' } | Format-Table -AutoSize -Wrap")
		gce.RunRemotely(ctx, logger.ToFile("status_for_ops_agent_uap_plugin.txt"), vm, fmt.Sprintf(`C:\grpcurl.exe -plaintext -d '{}' localhost:%s plugin_comm.GuestAgentPlugin/GetStatus`, OpsAgentPluginServerPort))
		gce.RunRemotely(ctx, logger.ToFile("active_processes_in_vm.txt"), vm, `Get-WmiObject -Class Win32_Process | Select-Object Name, ProcessId, ParentProcessId`)
	} else {
		gce.RunRemotely(ctx, logger.ToFile("windows_System_log.txt"), vm, "Get-WinEvent -LogName System | Format-Table -AutoSize -Wrap")

		gce.RunRemotely(ctx, logger.ToFile("Get-Service_output.txt"), vm, "Get-Service google-cloud-ops-agent* | Format-Table -AutoSize -Wrap")

		gce.RunRemotely(ctx, logger.ToFile("ops_agent_logs.txt"), vm, "Get-WinEvent -FilterHashtable @{ Logname='Application'; ProviderName='google-cloud-ops-agent' } | Format-Table -AutoSize -Wrap")
		gce.RunRemotely(ctx, logger.ToFile("open_telemetry_agent_logs.txt"), vm, "Get-WinEvent -FilterHashtable @{ Logname='Application'; ProviderName='google-cloud-ops-agent-opentelemetry-collector' } | Format-Table -AutoSize -Wrap")
	}
	// Fluent-Bit has not implemented exporting logs to the Windows event log yet.
	gce.RunRemotely(ctx, logger.ToFile("fluent_bit_agent_logs.txt"), vm, fmt.Sprintf("Get-Content -Path '%s' -Raw", stateDir+`log\logging-module.log`))
	gce.RunRemotely(ctx, logger.ToFile("health-checks.txt"), vm, fmt.Sprintf("Get-Content -Path '%s' -Raw", stateDir+`log\health-checks.log`))

	for _, conf := range []string{
		OpsAgentConfigPath(vm.ImageSpec),
		stateDir + `generated_configs\fluentbit\fluent_bit_main.conf`,
		stateDir + `generated_configs\fluentbit\fluent_bit_parser.conf`,
		stateDir + `generated_configs\otel\otel.yaml`,
		stateDir + `generated_configs\otel\feature_tracking_otlp.json`,
		stateDir + `generated_configs\otel\enabled_receivers_otlp.json`,
	} {
		pathParts := strings.Split(conf, `\`)
		basename := pathParts[len(pathParts)-1]
		gce.RunRemotely(ctx, logger.ToFile(basename+txtSuffix), vm, fmt.Sprintf("Get-Content -Path '%s' -Raw", conf))
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
			_, err := gce.WaitForMetric(
				ctx, logger, vm, "agent.googleapis.com/agent/uptime", TrailingQueryWindow,
				[]string{fmt.Sprintf("metric.labels.version = starts_with(%q)", service.UptimeMetricName)}, false)
			c <- err
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
		if gce.IsWindows(vm.ImageSpec) {
			output, statusErr := gce.RunRemotely(ctx, logger, vm, fmt.Sprintf("(Get-Service -Name '%s').Status", service.ServiceName))
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
		if _, statusErr := gce.RunRemotely(ctx, logger, vm, fmt.Sprintf("sudo service %s status", service.ServiceName)); statusErr != nil {
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
		if gce.IsWindows(vm.ImageSpec) {
			// There are two possible cases that are acceptable here:
			// 1) the service has been deleted
			// 2) the service still exists but is in some state other than "Running",
			//    such as "Stopped".
			// We currently only see case #1, but let's permit case #2 as well.
			// The following command should output nothing, or maybe just a blank
			// line, in either case.
			output, err := gce.RunRemotely(ctx, logger, vm, fmt.Sprintf("Get-Service | Where-Object {$_.Name -eq '%s' -and $_.Status -eq 'Running'}", service.ServiceName))
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
		if _, err := gce.RunRemotely(ctx, logger, vm, fmt.Sprintf("! sudo service %s status", service.ServiceName)); err != nil {
			return fmt.Errorf("CheckServicesNotRunning(): command could not be run or status of %s was unexpectedly OK. err: %v", service.ServiceName, err)
		}
	}
	return nil
}

func packageManagerCmd(vm *gce.VM) (string, error) {
	if gce.IsWindows(vm.ImageSpec) {
		return "googet -noconfirm", nil
	}

	switch vm.OS.ID {
	case "centos", "rhel", "rocky":
		return "sudo yum -y", nil
	case "opensuse-leap", "sles", "sles-sap":
		return "sudo zypper --non-interactive", nil
	case "debian", "ubuntu":
		return "sudo apt-get update; sudo apt-get -y", nil
	default:
		return "", fmt.Errorf("packageManagerCmd() doesn't support image spec %s with value '%s'", vm.ImageSpec, vm.OS.ID)
	}
}

// managePackages calls the package manager of the vm with the provided instruction (install/remove) for a set of packages.
func managePackages(ctx context.Context, logger *log.Logger, vm *gce.VM, instruction string, pkgs []string) error {
	pkgMCmd, err := packageManagerCmd(vm)
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("%s %s %s", pkgMCmd, instruction, strings.Join(pkgs, " "))
	_, err = gce.RunRemotely(ctx, logger, vm, cmd)
	return err
}

// tryInstallPackages attempts once to install the given packages.
func tryInstallPackages(ctx context.Context, logger *log.Logger, vm *gce.VM, pkgs []string) error {
	return managePackages(ctx, logger, vm, "install", pkgs)
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
	return managePackages(ctx, logger, vm, "remove", pkgs)
}

// checkPackages asserts that the given packages on the given VM are in a state matching the installed argument.
func checkPackages(ctx context.Context, logger *log.Logger, vm *gce.VM, pkgs []string, installed bool) error {
	errPrefix := ""
	if installed {
		errPrefix = "not "
	}
	if gce.IsWindows(vm.ImageSpec) {
		output, err := gce.RunRemotely(ctx, logger, vm, "googet installed")
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
		if IsRPMBased(vm.ImageSpec) {
			cmd = fmt.Sprintf("rpm --query %s", pkg)
			if !installed {
				cmd = "! " + cmd
			}
		} else if strings.HasPrefix(vm.ImageSpec, "debian-cloud") ||
			strings.HasPrefix(vm.ImageSpec, "ubuntu-os-cloud") {
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
			return fmt.Errorf("checkPackages() does not support image spec: %s", vm.ImageSpec)
		}
		if _, err := gce.RunRemotely(ctx, logger, vm, cmd); err != nil {
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

// IsRPMBased checks if the image spec is RPM based.
func IsRPMBased(imageSpec string) bool {
	return strings.HasPrefix(imageSpec, "centos-cloud") ||
		strings.HasPrefix(imageSpec, "rhel-") ||
		strings.HasPrefix(imageSpec, "rocky-linux-cloud") ||
		strings.HasPrefix(imageSpec, "suse-cloud") ||
		strings.HasPrefix(imageSpec, "suse-sap-cloud") ||
		strings.HasPrefix(imageSpec, "opensuse-cloud") ||
		strings.Contains(imageSpec, "sles-") ||
		strings.Contains(imageSpec, "rocky-linux-")
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
	output, err := gce.RunRemotely(ctx, logger, vm, "googet installed -info "+pkg)
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
	if gce.IsWindows(vm.ImageSpec) {
		return fetchPackageVersionWindows(ctx, logger, vm, pkg)
	}
	var version semver.Version
	tryGetVersion := func() error {
		cmd := `dpkg-query --show --showformat=\$\{Version} ` + pkg
		if IsRPMBased(vm.ImageSpec) {
			cmd = `rpm --query --queryformat=%\{VERSION} ` + pkg
		}
		output, err := gce.RunRemotely(ctx, logger, vm, cmd)
		if err != nil {
			return err
		}
		vStr := strings.TrimSpace(output.Stdout)
		if !IsRPMBased(vm.ImageSpec) {
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
func isRetriableInstallError(imageSpec string, err error) bool {
	if strings.Contains(err.Error(), "Could not refresh zypper repositories.") ||
		strings.Contains(err.Error(), "Credentials are invalid") ||
		strings.Contains(err.Error(), "Resource temporarily unavailable") ||
		strings.Contains(err.Error(), "System management is locked by the application") {
		return true
	}
	if gce.IsWindows(imageSpec) &&
		strings.Contains(err.Error(), "context deadline exceeded") {
		return true // See b/197127877 for history.
	}
	if strings.Contains(imageSpec, "rhel-8-") && strings.HasSuffix(imageSpec, "-sap-ha") &&
		strings.Contains(err.Error(), "Could not refresh the google-cloud-ops-agent yum repositories") {
		return true // See b/174039270 for history.
	}
	if strings.Contains(imageSpec, "rhel-8-") && strings.HasSuffix(imageSpec, "-sap-ha") &&
		strings.Contains(err.Error(), "Failed to download metadata for repo 'rhui-rhel-8-") {
		return true // This happens when the RHEL servers are down. See b/189950957.
	}
	if strings.HasPrefix(imageSpec, "rhel-") && strings.Contains(err.Error(), "SSL_ERROR_SYSCALL") {
		return true // b/187661497. Example: screen/3PMwAvhNBKWVYub
	}
	if strings.HasPrefix(imageSpec, "rhel-") && strings.Contains(err.Error(), "Encountered end of file") {
		return true // b/184729120#comment31. Example: screen/4yK9evoY68LiaLr
	}
	if strings.HasPrefix(imageSpec, "ubuntu-os-cloud") && strings.Contains(err.Error(), "Clearsigned file isn't valid") {
		// The upstream repo was in an inconsistent state. The error looks like:
		// screen/7U24zrRwADyYKqb
		return true
	}
	if strings.HasPrefix(imageSpec, "ubuntu-os-cloud") && strings.Contains(err.Error(), "Mirror sync in progress?") {
		// The mirror repo was in an inconsistent state. The error looks like:
		// http://screen/Ai2CHc7fcRosHJu
		return true
	}
	if strings.HasPrefix(imageSpec, "ubuntu-os-cloud") && strings.Contains(err.Error(), "Hash Sum mismatch") {
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
	shouldRetry := func(err error) bool { return isRetriableInstallError(vm.ImageSpec, err) }
	installWithRecovery := func() error {
		err := installFunc()
		if err != nil && shouldRetry(err) && strings.Contains(vm.ImageSpec, "rhel-8-") && strings.HasSuffix(vm.ImageSpec, "-sap-ha") {
			logger.Println("attempting recovery steps from https://access.redhat.com/discussions/4656371 so that subsequent attempts are more likely to succeed... see b/189950957")
			gce.RunRemotely(ctx, logger, vm, "sudo dnf clean all && sudo rm -r /var/cache/dnf && sudo dnf upgrade")
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
	// The command needed to be adjusted to work in a non-GUI context.
	cmd := `(New-Object Net.WebClient).DownloadFile("https://dl.google.com/cloudagents/windows/StackdriverLogging-v1-16.exe", "${env:UserProfile}\StackdriverLogging-v1-16.exe")
		Start-Process -FilePath "${env:UserProfile}\StackdriverLogging-v1-16.exe" -ArgumentList "/S" -Wait -NoNewWindow`
	_, err := gce.RunRemotely(ctx, logger, vm, cmd)
	return err
}

// InstallStandaloneWindowsMonitoringAgent installs the Stackdriver Monitoring
// agent on a Windows VM.
func InstallStandaloneWindowsMonitoringAgent(ctx context.Context, logger *log.Logger, vm *gce.VM) error {
	// https://cloud.google.com/monitoring/agent/installation#joint-install
	// The command needed to be adjusted to work in a non-GUI context.
	cmd := `(New-Object Net.WebClient).DownloadFile("https://repo.stackdriver.com/windows/StackdriverMonitoring-GCM-46.exe", "${env:UserProfile}\StackdriverMonitoring-GCM-46.exe")
		Start-Process -FilePath "${env:UserProfile}\StackdriverMonitoring-GCM-46.exe" -ArgumentList "/S" -Wait -NoNewWindow`
	_, err := gce.RunRemotely(ctx, logger, vm, cmd)
	return err
}

func getRestartOpsAgentCmd(imageSpec string) string {
	if gce.IsOpsAgentUAPPlugin() {
		grpcurlExecutable := "grpcurl"
		if gce.IsWindows(imageSpec) {
			grpcurlExecutable = `C:\grpcurl.exe`
			return fmt.Sprintf("%s -plaintext -d '{}' localhost:%s plugin_comm.GuestAgentPlugin/Stop; Start-Sleep -Seconds 5; %s -plaintext -d '{}' localhost:%s plugin_comm.GuestAgentPlugin/Start", grpcurlExecutable, OpsAgentPluginServerPort, grpcurlExecutable, OpsAgentPluginServerPort)
		}
		return fmt.Sprintf("%s -plaintext -d '{}' localhost:%s plugin_comm.GuestAgentPlugin/Stop && sleep 5 && %s -plaintext -d '{}' localhost:%s plugin_comm.GuestAgentPlugin/Start", grpcurlExecutable, OpsAgentPluginServerPort, grpcurlExecutable, OpsAgentPluginServerPort)
	}

	if gce.IsWindows(imageSpec) {
		return "Restart-Service google-cloud-ops-agent -Force"
	}
	// Return a command that works for both < 2.0.0 and >= 2.0.0 agents.
	return "sudo service google-cloud-ops-agent restart || sudo systemctl restart google-cloud-ops-agent"
}

// PackageLocation describes a location where packages
// (currently, only the Ops Agent packages) live.
type PackageLocation struct {
	// If provided, a URL for a directory in GCS containing .deb/.rpm/.goo files
	// to install on the testing VMs.
	// This setting is mutually exclusive with repoSuffix.
	packagesInGCS string
	// Package repository suffix to install from. Setting this and packagesInGCS
	// to "" means to install the latest stable release.
	repoSuffix string
	// Override the codename for the agent repository.
	// This setting is only used for ARM builds at the moment, and ignored when
	// installing from Artifact Registry.
	repoCodename string
	// Package repository GCP project to install from. Requires repoSuffix
	// to be nonempty.
	artifactRegistryProject string
	// Region the packages live in in Artifact Registry. Requires repoSuffix
	// to be nonempty.
	artifactRegistryRegion string
}

// LocationFromEnvVars assembles a PackageLocation from environment variables.
func LocationFromEnvVars() PackageLocation {
	return PackageLocation{
		packagesInGCS:           os.Getenv("AGENT_PACKAGES_IN_GCS"),
		repoSuffix:              os.Getenv("REPO_SUFFIX"),
		repoCodename:            os.Getenv("REPO_CODENAME"),
		artifactRegistryProject: os.Getenv("ARTIFACT_REGISTRY_PROJECT"),
		artifactRegistryRegion:  os.Getenv("ARTIFACT_REGISTRY_REGION"),
	}
}

func toEnvironment(environment map[string]string, format string, separator string) string {
	var assignments []string
	for k, v := range environment {
		if v != "" {
			assignments = append(assignments, fmt.Sprintf(format, k, v))
		}
	}
	return strings.Join(assignments, separator)
}

func linuxEnvironment(environment map[string]string) string {
	return toEnvironment(environment, "%s='%s'", " ")
}

func windowsEnvironment(environment map[string]string) string {
	return toEnvironment(environment, `$env:%s='%s'`, "\n")
}

// InstallOpsAgent installs the Ops Agent on the given VM. Consults the given
// PackageLocation to determine where to install the agent from. For details
// about PackageLocation, see the documentation for the PackageLocation struct.
func InstallOpsAgent(ctx context.Context, logger *log.Logger, vm *gce.VM, location PackageLocation) error {
	if location.packagesInGCS != "" && location.repoSuffix != "" {
		return fmt.Errorf("invalid PackageLocation: cannot provide both location.packagesInGCS and location.repoSuffix. location=%#v", location)
	}

	if location.artifactRegistryRegion != "" && location.repoSuffix == "" {
		return fmt.Errorf("invalid PackageLocation: location.artifactRegistryRegion was nonempty yet location.repoSuffix was empty. location=%#v", location)
	}

	if gce.IsOpsAgentUAPPlugin() {
		return InstallOpsAgentUAPPlugin(ctx, logger, vm, location)
	}

	if location.packagesInGCS != "" {
		return InstallPackageFromGCS(ctx, logger, vm, location.packagesInGCS)
	}

	preservedEnvironment := map[string]string{
		"REPO_SUFFIX":               location.repoSuffix,
		"REPO_CODENAME":             location.repoCodename,
		"ARTIFACT_REGISTRY_PROJECT": location.artifactRegistryProject,
		"ARTIFACT_REGISTRY_REGION":  location.artifactRegistryRegion,
	}

	if gce.IsWindows(vm.ImageSpec) {
		// Note that these commands match the ones from our public install docs
		// (https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/installation)
		// and keeping them in sync is encouraged so that we are testing the
		// same commands that our customers are running.
		if _, err := gce.RunRemotely(ctx, logger, vm, `(New-Object Net.WebClient).DownloadFile("https://dl.google.com/cloudagents/add-google-cloud-ops-agent-repo.ps1", "${env:UserProfile}\add-google-cloud-ops-agent-repo.ps1")`); err != nil {
			return fmt.Errorf("InstallOpsAgent() failed to download repo script: %w", err)
		}
		runScript := func() error {
			scriptCmd := fmt.Sprintf(`%s
& "${env:UserProfile}\add-google-cloud-ops-agent-repo.ps1" -AlsoInstall`, windowsEnvironment(preservedEnvironment))
			_, err := gce.RunRemotely(ctx, logger, vm, scriptCmd)
			return err
		}
		// TODO: b/202526819 - Remove retries once the script does retries internally.
		if err := RunInstallFuncWithRetry(ctx, logger, vm, runScript); err != nil {
			return fmt.Errorf("InstallOpsAgent() failed to run repo script: %w", err)
		}
		return nil
	}

	if _, err := gce.RunRemotely(ctx,
		logger, vm, "curl -sSO https://dl.google.com/cloudagents/add-google-cloud-ops-agent-repo.sh"); err != nil {
		return fmt.Errorf("InstallOpsAgent() failed to download repo script: %w", err)
	}

	runInstallScript := func() error {
		envVars := linuxEnvironment(preservedEnvironment)
		_, err := gce.RunRemotely(ctx, logger, vm, "sudo "+envVars+" bash -x add-google-cloud-ops-agent-repo.sh --also-install")
		return err
	}
	if err := RunInstallFuncWithRetry(ctx, logger, vm, runInstallScript); err != nil {
		return fmt.Errorf("InstallOpsAgent() error running repo script: %w", err)
	}
	return nil
}

// SetupOpsAgent installs the Ops Agent and installs the given config.
func SetupOpsAgent(ctx context.Context, logger *log.Logger, vm *gce.VM, config string) error {
	return SetupOpsAgentFrom(ctx, logger, vm, config, LocationFromEnvVars())
}

// RestartOpsAgent restarts the Ops Agent and waits for it to become available.
func RestartOpsAgent(ctx context.Context, logger *log.Logger, vm *gce.VM) error {
	if _, err := gce.RunRemotely(ctx, logger, vm, getRestartOpsAgentCmd(vm.ImageSpec)); err != nil {
		return fmt.Errorf("RestartOpsAgent() failed to restart ops agent: %v", err)
	}
	// Give agents time to shut down. Fluent-Bit's default shutdown grace period
	// is 5 seconds, so we should probably give it at least that long.
	time.Sleep(10 * time.Second)
	return nil
}

// StartOpsAgentPluginServer starts the Ops Agent Plugin gRPC server on the testing VM in the background.
func StartOpsAgentPluginServer(ctx context.Context, logger *log.Logger, vm *gce.VM, port string) error {
	if gce.IsWindows(vm.ImageSpec) {
		startUAPWindowsPlugin, err := scriptsDir.ReadFile(path.Join("testdata", "start-uap-plugin-server.ps1"))
		if err != nil {
			return fmt.Errorf("StartOpsAgentPluginServer() failed to read start-uap-plugin-server.ps1: %v", err)
		}
		if _, err := gce.RunScriptRemotely(ctx, logger, vm, string(startUAPWindowsPlugin), nil, nil); err != nil {
			return fmt.Errorf("StartOpsAgentPluginServer() failed to start the ops agent plugin: %v", err)
		}
		return nil
	}

	if _, err := gce.RunRemotely(ctx, logger, vm, fmt.Sprintf("sudo nohup ~/plugin --address=localhost:%s --errorlogfile=errorlog.txt --protocol=tcp 1>/dev/null 2>/dev/null &", port)); err != nil {
		return fmt.Errorf("StartOpsAgentPluginServer() failed to start the ops agent plugin: %v", err)
	}
	return nil

}

func StartCommandForImage(imageSpec string) string {
	if gce.IsOpsAgentUAPPlugin() {
		grpcurlExecutable := "grpcurl"
		if gce.IsWindows(imageSpec) {
			grpcurlExecutable = `C:\grpcurl.exe`
		}
		return fmt.Sprintf("%s -plaintext -d '{}' localhost:1234 plugin_comm.GuestAgentPlugin/Start", grpcurlExecutable)
	}

	if gce.IsWindows(imageSpec) {
		return "Start-Service google-cloud-ops-agent"
	}
	// Return a command that works for both < 2.0.0 and >= 2.0.0 agents.
	return "sudo service google-cloud-ops-agent start || sudo systemctl start google-cloud-ops-agent"
}

func StartOpsAgentViaUAPCommand(imageSpec string, config string) string {
	grpcurlExecutable := "grpcurl"
	if gce.IsWindows(imageSpec) {
		grpcurlExecutable = `C:\grpcurl.exe`
	}
	if len(config) > 0 {
		if gce.IsWindows(imageSpec) {
			return fmt.Sprintf("echo '{%s}' | %s -plaintext -d \"@\" localhost:1234 plugin_comm.GuestAgentPlugin/Start", config, grpcurlExecutable)
		}
		return fmt.Sprintf("%s -plaintext -d '{%s}' localhost:1234 plugin_comm.GuestAgentPlugin/Start", grpcurlExecutable, config)
	}
	return fmt.Sprintf("%s -plaintext -d '{}' localhost:1234 plugin_comm.GuestAgentPlugin/Start", grpcurlExecutable)
}

func StopCommandForImage(imageSpec string) string {
	if gce.IsOpsAgentUAPPlugin() {
		grpcurlExecutable := "grpcurl"
		if gce.IsWindows(imageSpec) {
			grpcurlExecutable = `C:\grpcurl.exe`
		}
		return fmt.Sprintf("%s -plaintext -d '{}' localhost:1234 plugin_comm.GuestAgentPlugin/Stop", grpcurlExecutable)
	}

	if gce.IsWindows(imageSpec) {
		return "Stop-Service google-cloud-ops-agent -Force"
	}
	// Return a command that works for both < 2.0.0 and >= 2.0.0 agents.
	return "sudo service google-cloud-ops-agent stop || sudo systemctl stop google-cloud-ops-agent"
}

func GetUAPPluginStatusForImage(imageSpec string) string {
	grpcurlExecutable := "grpcurl"
	if gce.IsWindows(imageSpec) {
		grpcurlExecutable = `C:\grpcurl.exe`
	}
	return fmt.Sprintf("%s -plaintext -d '{}' localhost:1234 plugin_comm.GuestAgentPlugin/GetStatus", grpcurlExecutable)

}

func StartOpsAgentPluginWithBackoff(ctx context.Context, logger *log.Logger, vm *gce.VM) error {
	tryStartOpsAgent := func() error {
		if _, err := gce.RunRemotely(ctx, logger, vm, StartCommandForImage(vm.ImageSpec)); err != nil {
			return fmt.Errorf("failed to start Ops Agent: %v", err)
		}
		time.Sleep(8 * time.Second)
		cmdOut, err := gce.RunRemotely(ctx, logger, vm, GetUAPPluginStatusForImage(vm.ImageSpec))
		if err != nil {
			return fmt.Errorf("failed to retrieve Ops Agent status: %v", err)
		}
		if !strings.Contains(cmdOut.Stdout, "is running ok") {
			return fmt.Errorf("Ops Agent is not started successfully: stdout: %v, stderr: %v", cmdOut.Stdout, cmdOut.Stderr)
		}
		return nil
	}
	backoffPolicy := backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(20*time.Second), 10), ctx)
	if err := backoff.Retry(tryStartOpsAgent, backoffPolicy); err != nil {
		return err
	}
	return nil
}

// SetupOpsAgentFrom is an overload of setupOpsAgent that allows the callsite to
// decide which version of the agent gets installed.
func SetupOpsAgentFrom(ctx context.Context, logger *log.Logger, vm *gce.VM, config string, location PackageLocation) error {
	if err := InstallOpsAgent(ctx, logger, vm, location); err != nil {
		return err
	}
	startupDelay := 20 * time.Second
	if len(config) > 0 {
		if gce.IsWindows(vm.ImageSpec) {
			// Sleep to avoid some flaky errors when restarting the agent because the
			// services have not fully started up yet.
			time.Sleep(startupDelay)
		}
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(config), OpsAgentConfigPath(vm.ImageSpec)); err != nil {
			return fmt.Errorf("SetupOpsAgentFrom() failed to upload config file: %v", err)
		}
	}

	// UAP Plugin needs to be started regardless, since it's not automatically started upon install.
	if gce.IsOpsAgentUAPPlugin() {
		return StartOpsAgentPluginWithBackoff(ctx, logger, vm)
	}

	// Regular Ops Agent only needs a restart if the config is not empty.
	if len(config) > 0 {
		return RestartOpsAgent(ctx, logger, vm)
	}
	// Give agents time to start up.
	time.Sleep(startupDelay)
	return nil
}

// RecommendedMachineType returns a reasonable setting for a VM's machine type
// (https://cloud.google.com/compute/docs/machine-types). Windows instances
// are configured to be larger because they need more CPUs to start up in a
// reasonable amount of time.
func RecommendedMachineType(imageSpec string) string {
	if gce.IsWindows(imageSpec) {
		return "e2-standard-4"
	}
	if gce.IsARM(imageSpec) {
		return "t2a-standard-2"
	}
	return "e2-standard-2"
}

// CommonSetup sets up the VM for testing.
func CommonSetup(t *testing.T, imageSpec string) (context.Context, *logging.DirectoryLogger, *gce.VM) {
	return CommonSetupWithExtraCreateArguments(t, imageSpec, nil)
}

// CommonSetupWithExtraCreateArguments sets up the VM for testing with extra creation arguments for the `gcloud compute instances create` command.
func CommonSetupWithExtraCreateArguments(t *testing.T, imageSpec string, extraCreateArguments []string) (context.Context, *logging.DirectoryLogger, *gce.VM) {
	return CommonSetupWithExtraCreateArgumentsAndMetadata(t, imageSpec, extraCreateArguments, nil)
}

// CommonSetupWithExtraCreateArgumentsAndMetadata sets up the VM for testing with extra creation arguments for the `gcloud compute instances create` command and additional metadata.
func CommonSetupWithExtraCreateArgumentsAndMetadata(t *testing.T, imageSpec string, extraCreateArguments []string, additionalMetadata map[string]string) (context.Context, *logging.DirectoryLogger, *gce.VM) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), gce.SuggestedTimeout)
	t.Cleanup(cancel)
	gcloudConfigDir := t.TempDir()
	if err := gce.SetupGcloudConfigDir(ctx, gcloudConfigDir); err != nil {
		t.Fatalf("Unable to set up a gcloud config directory: %v", err)
	}
	ctx = gce.WithGcloudConfigDir(ctx, gcloudConfigDir)

	logger := gce.SetupLogger(t)
	logger.ToMainLog().Println("Calling SetupVM(). For details, see VM_initialization.txt.")
	options := gce.VMOptions{
		ImageSpec:            imageSpec,
		TimeToLive:           "3h",
		MachineType:          RecommendedMachineType(imageSpec),
		ExtraCreateArguments: extraCreateArguments,
		Metadata:             additionalMetadata,
	}
	vm := gce.SetupVM(ctx, t, logger.ToFile("VM_initialization.txt"), options)
	logger.ToMainLog().Printf("VM is ready: %#v", vm)
	t.Cleanup(func() {
		RunOpsAgentDiagnostics(ctx, logger, vm)
	})
	return ctx, logger, vm
}

// ManagedInstanceGroupVMSetup sets up a Managed Instance Group VM for testing with extra creation arguments for the `gcloud compute instances create` command and additional metadata.
func ManagedInstanceGroupVMSetup(t *testing.T, imageSpec string, extraCreateArguments []string, additionalMetadata map[string]string) (context.Context, *logging.DirectoryLogger, *gce.ManagedInstanceGroupVM) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), gce.SuggestedTimeout)
	t.Cleanup(cancel)
	gcloudConfigDir := t.TempDir()
	if err := gce.SetupGcloudConfigDir(ctx, gcloudConfigDir); err != nil {
		t.Fatalf("Unable to set up a gcloud config directory: %v", err)
	}
	ctx = gce.WithGcloudConfigDir(ctx, gcloudConfigDir)

	logger := gce.SetupLogger(t)
	logger.ToMainLog().Println("Calling SetupManagedInstanceGroupVM(). For details, see VM_initialization.txt.")
	options := gce.VMOptions{
		ImageSpec:            imageSpec,
		TimeToLive:           "3h",
		MachineType:          RecommendedMachineType(imageSpec),
		ExtraCreateArguments: extraCreateArguments,
		Metadata:             additionalMetadata,
	}
	migVM := gce.SetupManagedInstanceGroupVM(ctx, t, logger.ToFile("VM_initialization.txt"), options)
	logger.ToMainLog().Printf("ManagedInstanceGroupVM is ready: %#v", migVM.VM)
	t.Cleanup(func() {
		RunOpsAgentDiagnostics(ctx, logger, migVM.VM)
	})
	return ctx, logger, migVM
}

func InstallOpsAgentUAPPlugin(ctx context.Context, logger *log.Logger, vm *gce.VM, location PackageLocation) error {
	// Used for manual testing or pre-submits
	if location.packagesInGCS != "" {
		return InstallOpsAgentUAPPluginFromGCS(ctx, logger, vm, location.packagesInGCS)
	}

	// Used for nightly builds
	artifactBucket := fmt.Sprintf("gs://%s-ops-agent-releases/%s", location.artifactRegistryProject, location.repoSuffix)
	return InstallOpsAgentUAPPluginFromGCS(ctx, logger, vm, artifactBucket)
}

// InstallOpsAgentUAPPluginFromGCS installs the Ops Agent plugin tarball from GCS onto the given Linux VM.
//
// gcsPath must point to a GCS Path to a .tar.gz file to install on the testing VMs.
func InstallOpsAgentUAPPluginFromGCS(ctx context.Context, logger *log.Logger, vm *gce.VM, gcsPath string) error {
	if err := gce.InstallGcloudIfNeeded(ctx, logger, vm); err != nil {
		return err
	}

	if gce.IsWindows(vm.ImageSpec) {
		if _, err := gce.RunRemotely(ctx, logger, vm, "New-Item -ItemType directory -Path C:\\agentPlugin"); err != nil {
			return err
		}

		if _, err := gce.RunRemotely(ctx, logger, vm, fmt.Sprintf(`gcloud storage cp %s/google-cloud-ops-agent-plugin*.tar.gz C:\\agentPlugin`, gcsPath)); err != nil {
			return fmt.Errorf("error copying down agent package from GCS: %v", err)
		}

		if _, err := gce.RunRemotely(ctx, logger, vm, "ls C:\\agentPlugin"); err != nil {
			return err
		}
		if _, err := gce.RunRemotely(ctx, logger, vm, `Get-ChildItem -Path "C:\agentPlugin" -Filter "google-cloud-ops-agent-plugin*.tar.gz" -File|Select-Object -First 1 -Expand FullName | ForEach-Object { if ($_){ & tar -xzf $_ -C "C:\"} }`); err != nil {
			return err
		}
		// Print the contents of the home dir into the logs.
		if _, err := gce.RunRemotely(ctx, logger, vm, "ls C:\\agentPlugin; ls C:\\"); err != nil {
			return err
		}

	} else {
		if _, err := gce.RunRemotely(ctx, logger, vm, "mkdir -p /tmp/agentPlugin"); err != nil {
			return err
		}

		if _, err := gce.RunRemotely(ctx, logger, vm, "sudo gcloud storage cp "+gcsPath+"/google-cloud-ops-agent-plugin*.tar.gz /tmp/agentPlugin"); err != nil {
			return fmt.Errorf("error copying down the agent uap plugin tarball from GCS: %v", err)
		}

		// Print the contents of /tmp/agentPlugin into the logs.
		if _, err := gce.RunRemotely(ctx, logger, vm, "ls -la /tmp/agentPlugin"); err != nil {
			return err
		}
		if _, err := gce.RunRemotely(ctx, logger, vm, "sudo find /tmp/agentPlugin -maxdepth 1 -name \"google-cloud-ops-agent-plugin*.tar.gz\" -print0 | xargs -0 -I {} sudo tar -xzf {} --no-overwrite-dir -C ~/ && ls -la"); err != nil {
			return err
		}
		// Print the contents of the home dir into the logs.
		if _, err := gce.RunRemotely(ctx, logger, vm, "ls -la ~/"); err != nil {
			return err
		}
	}

	if err := gce.InstallGrpcurlIfNeeded(ctx, logger, vm); err != nil {
		return err
	}
	return StartOpsAgentPluginServer(ctx, logger, vm, OpsAgentPluginServerPort)
}

// InstallPackageFromGCS installs the agent package from GCS onto the given Linux VM.
//
// gcsPath must point to a GCS Path that contains .deb/.rpm/.goo files to install on the testing VMs.
// Packages with "dbgsym" in their name are skipped because customers don't
// generally install those, so our tests shouldn't either.
func InstallPackageFromGCS(ctx context.Context, logger *log.Logger, vm *gce.VM, gcsPath string) error {
	tryInstallPackageFromGCS := func() error {
		if gce.IsWindows(vm.ImageSpec) {
			return installWindowsPackageFromGCS(ctx, logger, vm, gcsPath)
		}
		if _, err := gce.RunRemotely(ctx, logger, vm, "mkdir -p /tmp/agentUpload /tmp/agentPlugin"); err != nil {
			return err
		}
		if err := gce.InstallGcloudIfNeeded(ctx, logger, vm); err != nil {
			return err
		}
		if _, err := gce.RunRemotely(ctx, logger, vm, "sudo gcloud storage cp -r "+gcsPath+"/* /tmp/agentUpload"); err != nil {
			return fmt.Errorf("error copying down agent package from GCS: %v", err)
		}
		// Print the contents of /tmp/agentUpload into the logs.
		if _, err := gce.RunRemotely(ctx, logger, vm, "ls /tmp/agentUpload"); err != nil {
			return err
		}
		if _, err := gce.RunRemotely(ctx, logger, vm, "rm /tmp/agentUpload/*dbgsym* || echo nothing to delete"); err != nil {
			return err
		}
		if _, err := gce.RunRemotely(ctx, logger, vm, "mv -f /tmp/agentUpload/*.tar.gz /tmp/agentPlugin || echo nothing to move"); err != nil {
			return err
		}
		if IsRPMBased(vm.ImageSpec) {
			if _, err := gce.RunRemotely(ctx, logger, vm, "sudo rpm --upgrade -v --force /tmp/agentUpload/*"); err != nil {
				return fmt.Errorf("error installing agent from .rpm file: %v", err)
			}
			return nil
		}
		// --allow-downgrades is marked as dangerous, but I don't see another way
		// to get the following sequence to work (from TestUpgradeOpsAgent):
		// 1. install stable package from Rapture
		// 2. install just-built package from GCS
		// Nor do I know why apt considers that sequence to be a downgrade.
		// Setting DPkg::Lock::Timeout=600 to wait while other apt command may be executing.
		if _, err := gce.RunRemotely(ctx, logger, vm, "sudo apt-get -o DPkg::Lock::Timeout=600 install --allow-downgrades --yes --verbose-versions /tmp/agentUpload/*"); err != nil {
			return fmt.Errorf("error installing agent from .deb file: %v", err)
		}
		return nil
	}
	backoffPolicy := backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 3), ctx)
	return backoff.Retry(tryInstallPackageFromGCS, backoffPolicy)
}

// Installs the agent package from GCS (see packagesInGCS) onto the given Windows VM.
func installWindowsPackageFromGCS(ctx context.Context, logger *log.Logger, vm *gce.VM, gcsPath string) error {
	if _, err := gce.RunRemotely(ctx, logger, vm, "New-Item -ItemType directory -Path C:\\agentUpload"); err != nil {
		return err
	}
	if _, err := gce.RunRemotely(ctx, logger, vm, fmt.Sprintf("gcloud storage cp -r %s/*.goo C:\\agentUpload", gcsPath)); err != nil {
		return fmt.Errorf("error copying down agent package from GCS: %v", err)
	}
	if _, err := gce.RunRemotely(ctx, logger, vm, "googet -noconfirm -verbose install -reinstall (Get-ChildItem C:\\agentUpload\\*.goo | Select-Object -Expand FullName)"); err != nil {
		return fmt.Errorf("error installing agent from .goo file: %v", err)
	}
	return nil
}

// OpsAgentConfigPath returns the platform-specific filesystem location where
// the Ops Agent config is stored.
func OpsAgentConfigPath(imageSpec string) string {
	if gce.IsWindows(imageSpec) {
		return `C:\Program Files\Google\Cloud Operations\Ops Agent\config\config.yaml`
	}
	return "/etc/google-cloud-ops-agent/config.yaml"
}

func GetOtelConfigPath(imageSpec string) string {
	if gce.IsOpsAgentUAPPlugin() {
		if gce.IsWindows(imageSpec) {
			return `C:\ProgramData\Google\Compute Engine\google-guest-agent\agent_state\plugins\ops-agent-plugin\generated_configs\otel\otel.yaml`
		}
		return "/var/lib/google-guest-agent/agent_state/plugins/ops-agent-plugin/run/google-cloud-ops-agent-opentelemetry-collector/otel.yaml"
	}

	if gce.IsWindows(imageSpec) {
		return `C:\ProgramData\Google\Cloud Operations\Ops Agent\generated_configs\otel\otel.yaml`
	}
	return "/var/run/google-cloud-ops-agent-opentelemetry-collector/otel.yaml"
}
