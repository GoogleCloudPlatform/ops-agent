// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build windows
// +build windows

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sync"

	"go.opentelemetry.io/otel"
	"golang.org/x/sys/windows/svc/mgr"
	"google.golang.org/grpc/status"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	pb "github.com/GoogleCloudPlatform/ops-agent/cmd/ops_agent_uap_plugin/google_guest_agent/plugin"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/internal/healthchecks"
	"github.com/GoogleCloudPlatform/ops-agent/internal/logs"
	"github.com/GoogleCloudPlatform/ops-agent/internal/self_metrics"
	"github.com/kardianos/osext"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
)

const (
	GeneratedConfigsOutDir           = "generated_configs"
	LogsDirectory                    = "log"
	RuntimeDirectory                 = "run"
	OpsAgentUAPPluginEventID  uint32 = 8
	DiagnosticsEventID        uint32 = 2
	WindowsEventLogIdentifier        = "google-cloud-ops-agent-uap-plugin"
	AgentWrapperBinary               = "google-cloud-ops-agent-wrapper.exe"
	DiagnosticsBinary                = "google-cloud-ops-agent-diagnostics.exe"
	FluentbitBinary                  = "fluent-bit.exe"
	OtelBinary                       = "google-cloud-metrics-agent_windows_amd64.exe"
)

var (
	AgentWindowsServiceName       = []string{"StackdriverLogging", "StackdriverMonitoring", "google-cloud-ops-agent"}
	DefaultPluginStateDirectory   = filepath.Join(os.Getenv("PROGRAMDATA"), "Google/Compute Engine/google-guest-agent/agent_state/plugins/ops-agent-plugin")
	OpsAgentConfigLocationWindows = filepath.Join(os.Getenv("PROGRAMDATA"), "Google/Cloud Operations/Ops Agent/config/config.yaml")
)

// Apply applies the config sent or performs the work defined in the message.
// ApplyRequest is opaque to the agent and is expected to be well known contract
// between Plugin and the server itself. For e.g. service might want to update
// plugin config to enable/disable feature here plugins can react to such requests.
func (ps *OpsAgentPluginServer) Apply(ctx context.Context, msg *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	return &pb.ApplyResponse{}, nil
}

// Start starts the plugin and initiates the plugin functionality.
// Until plugin receives Start request plugin is expected to be not functioning
// and just listening on the address handed off waiting for the request.
func (ps *OpsAgentPluginServer) Start(ctx context.Context, msg *pb.StartRequest) (*pb.StartResponse, error) {
	ps.mu.Lock()
	if ps.cancel != nil {
		log.Printf("The Ops Agent plugin is started already, skipping the current request")
		ps.mu.Unlock()
		return &pb.StartResponse{}, nil
	}
	log.Printf("Received a Start request: %s. Starting the Ops Agent", msg)

	pContext, cancel := context.WithCancel(context.Background())
	ps.cancel = cancel
	ps.mu.Unlock()

	windowsEventLogger, err := createWindowsEventLogger()
	defer windowsEventLogger.Close()
	if err != nil {
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
		log.Printf("Start() failed, because it failed to create Windows event logger: %s", err)
		return nil, status.Error(1, err.Error())
	}

	// Calculate plugin install and state dirs
	pluginInstallDir, err := osext.ExecutableFolder()
	if err != nil {
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
		windowsEventLogger.Error(OpsAgentUAPPluginEventID, fmt.Sprintf("Start() failed, because it cannot determine the plugin install location: %s", err))
		return nil, status.Error(1, err.Error())
	}

	pluginStateDir := msg.GetConfig().GetStateDirectoryPath()
	if pluginStateDir == "" {
		pluginStateDir = DefaultPluginStateDirectory
	}
	log.Printf("Determined pluginInstallDir: %v, and pluginStateDir: %v", pluginInstallDir, pluginStateDir)

	// Subagents config validation and generation
	if err := generateSubAgentConfigs(ctx, pluginStateDir); err != nil {
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
		windowsEventLogger.Error(OpsAgentUAPPluginEventID, fmt.Sprintf("Start() failed at the subagent config validation and generation step: %s", err))
		return nil, status.Error(1, err.Error())
	}

	// Detect conflicting installations
	foundConflictingInstallations, err := findPreExistentAgents(AgentWindowsServiceName)
	if foundConflictingInstallations || err != nil {
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
		windowsEventLogger.Error(OpsAgentUAPPluginEventID, fmt.Sprintf("Start() failed because it detected conflicting installations: %s", err))
		return nil, status.Error(1, err.Error())
	}

	// Trigger Healthchecks
	runHealthChecks(pluginStateDir, windowsEventLogger)
	windowsEventLogger.Info(OpsAgentUAPPluginEventID, "Health checks completed")

	// the diagnostics service and subagent startups
	cancelFunc := func() {
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
	}
	go runSubagents(pContext, cancelFunc, pluginInstallDir, pluginStateDir, runSubAgentCommand, ps.runCommand, windowsEventLogger)

	return &pb.StartResponse{}, nil
}

// Stop is the stop hook and implements any cleanup if required.
// Stop maybe called if plugin revision is being changed.
// For e.g. if plugins want to stop some task it was performing or remove some
// state before exiting it can be done on this request.
func (ps *OpsAgentPluginServer) Stop(ctx context.Context, msg *pb.StopRequest) (*pb.StopResponse, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.cancel == nil {
		log.Printf("The Ops Agent plugin is stopped already, skipping the current request")
		return &pb.StopResponse{}, nil
	}
	log.Printf("Received a Stop request: %s. Stopping the Ops Agent", msg)
	ps.cancel()
	ps.cancel = nil
	return &pb.StopResponse{}, nil
}

// GetStatus is the health check agent would perform to make sure plugin process
// is alive. If request fails process is considered dead and relaunched. Plugins
// can share any additional information to report it to the service. For e.g. if
// plugins detect some non-fatal errors causing it unable to offer some features
// it can reported in status which is sent back to the service by agent.
func (ps *OpsAgentPluginServer) GetStatus(ctx context.Context, msg *pb.GetStatusRequest) (*pb.Status, error) {
	log.Println("Received a GetStatus request")
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.cancel == nil {
		log.Println("The Ops Agent plugin is not running")
		return &pb.Status{Code: 1, Results: []string{"The Ops Agent Plugin is not running."}}, nil
	}
	log.Println("The Ops Agent plugin is running")
	return &pb.Status{Code: 0, Results: []string{"The Ops Agent Plugin is running ok."}}, nil
}

func runCommand(cmd *exec.Cmd) (string, error) {
	if cmd == nil {
		return "", nil
	}

	log.Printf("Running command: %s", cmd.Args)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Command %s failed, \ncommand output: %s\ncommand error: %s", cmd.Args, string(out), err)
	}
	return string(out), err
}

func runSubAgentCommand(ctx context.Context, cancel context.CancelFunc, cmd *exec.Cmd, runCommand RunCommandFunc, wg *sync.WaitGroup) {
	defer wg.Done()
	if cmd == nil {
		return
	}
	if ctx.Err() != nil {
		// context has been cancelled
		log.Printf("cannot execute command: %s, because the context has been cancelled", cmd.Args)
		return
	}

	output, err := runCommand(cmd)
	if err != nil {
		log.Printf("command: %s exited with errors, not restarting.\nCommand output: %s\n Command error:%s", cmd.Args, string(output), err)
	} else {
		log.Printf("command: %s %s exited successfully.\nCommand output: %s", cmd.Path, cmd.Args, string(output))
	}
	cancel() // cancels the parent context which also stops other Ops Agent sub-binaries from running.
	return
}

func generateSubAgentConfigs(ctx context.Context, pluginStateDir string) error {
	uc, err := confgenerator.MergeConfFiles(ctx, OpsAgentConfigLocationWindows, apps.BuiltInConfStructs)
	if err != nil {
		return err
	}

	log.Printf("Built-in config:\n%s\n", apps.BuiltInConfStructs["windows"])
	log.Printf("Merged config:\n%s\n", uc)

	for _, subagent := range []string{
		"otel",
		"fluentbit",
	} {
		if err := uc.GenerateFilesFromConfig(
			ctx,
			subagent,
			filepath.Join(pluginStateDir, LogsDirectory),
			filepath.Join(pluginStateDir, RuntimeDirectory),
			filepath.Join(pluginStateDir, GeneratedConfigsOutDir, subagent)); err != nil {
			return err
		}
	}
	return nil
}

func runHealthChecks(pluginStateDir string, windowsEventLogger debug.Log) {
	logsDir := filepath.Join(pluginStateDir, LogsDirectory)
	gceHealthChecks := healthchecks.HealthCheckRegistryFactory()
	healthCheckFileLogger := healthchecks.CreateHealthChecksLogger(logsDir)
	// Log health check results to health-checks.log log file.
	healthCheckResults := gceHealthChecks.RunAllHealthChecks(healthCheckFileLogger)

	// Log health check results to windows event log too.
	healthCheckWindowsEventLogger := logs.WindowsServiceLogger{EventID: OpsAgentUAPPluginEventID, Logger: windowsEventLogger}
	healthchecks.LogHealthCheckResults(healthCheckResults, healthCheckWindowsEventLogger)
}

func createWindowsEventLogger() (debug.Log, error) {
	eventlog.InstallAsEventCreate(WindowsEventLogIdentifier, eventlog.Error|eventlog.Warning|eventlog.Info)
	elog, err := eventlog.Open(WindowsEventLogIdentifier)
	if err != nil {
		return nil, err
	}
	return elog, nil
}

func findPreExistentAgents(agentWindowsServiceNames []string) (bool, error) {
	mgr, err := mgr.Connect()
	if err != nil {
		return false, fmt.Errorf("failed to connect to service manager: %s", err)
	}
	defer mgr.Disconnect()

	installedServices, err := mgr.ListServices()
	if err != nil {
		return false, fmt.Errorf("failed to list installed Windows services: %s", err)
	}

	installedServicesSet := make(map[string]bool)
	for _, s := range installedServices {
		installedServicesSet[s] = true
	}

	agentAlreadyInstalled := false
	alreadyInstalledAgentServiceName := ""
	for _, s := range agentWindowsServiceNames {
		if installedServicesSet[s] {
			agentAlreadyInstalled = true
			alreadyInstalledAgentServiceName = s
			break
		}
	}

	if agentAlreadyInstalled {
		return true, fmt.Errorf("conflicting installations identified: %v", alreadyInstalledAgentServiceName)
	}
	return false, nil
}

// runSubagents starts up the diagnostics service, otel, and fluent bit subagents in separate goroutines.
// All child goroutines create a new context derived from the same parent context.
// This ensures that crashes in one goroutine don't affect other goroutines.
// However, when one goroutine exits with errors, it won't be restarted, and all other goroutines are also terminated.
// This is done by canceling the parent context.
// This makes sure that GetStatus() returns a non-healthy status, signaling UAP to Start() the plugin again.
//
// ctx: the parent context that all child goroutines share.
//
// cancel: the cancel function for the parent context. By calling this function, the parent context is canceled,
// and GetStatus() returns a non-healthy status, signaling UAP to re-trigger Start().
func runSubagents(ctx context.Context, cancel context.CancelFunc, pluginInstallDirectory string, pluginStateDirectory string, runSubAgentCommand RunSubAgentCommandFunc, runCommand RunCommandFunc, windowsEventLogger debug.Log) {

	var wg sync.WaitGroup
	// Starting the diagnostics service
	wg.Add(1)
	go runDiagnosticsService(ctx, windowsEventLogger, cancel, &wg)

	// Starting Otel
	runOtelCmd := exec.CommandContext(ctx,
		path.Join(pluginInstallDirectory, OtelBinary),
		"--config", path.Join(pluginStateDirectory, GeneratedConfigsOutDir, "otel/otel.yaml"),
	)
	wg.Add(1)
	go runSubAgentCommand(ctx, cancel, runOtelCmd, runCommand, &wg)

	// Starting Fluentbit
	runFluentBitCmd := exec.CommandContext(ctx,
		path.Join(pluginInstallDirectory, AgentWrapperBinary),
		"-config_path", OpsAgentConfigLocationWindows,
		"-log_path", path.Join(pluginStateDirectory, LogsDirectory, "logging-module.log"),
		path.Join(pluginInstallDirectory, FluentbitBinary),
		"-c", path.Join(pluginStateDirectory, GeneratedConfigsOutDir, "fluentbit/fluent_bit_main.conf"),
		"-R", path.Join(pluginStateDirectory, GeneratedConfigsOutDir, "fluentbit/fluent_bit_parser.conf"),
		"--storage_path", path.Join(pluginStateDirectory, "run/buffers"),
	)
	wg.Add(1)
	go runSubAgentCommand(ctx, cancel, runFluentBitCmd, runCommand, &wg)

	wg.Wait()
}

func runDiagnosticsService(ctx context.Context, windowsEventLogger debug.Log, cancel context.CancelFunc, wg *sync.WaitGroup) {
	defer wg.Done()

	userUc, mergedUc, err := getUserAndMergedConfigs(ctx, OpsAgentConfigLocationWindows)
	if err != nil {
		log.Printf("Failed to run the diagnostics service: %v", err)
		cancel()
		return
	}

	h := &otelErrorHandler{windowsEventLogger: windowsEventLogger, windowsEventId: OpsAgentUAPPluginEventID}
	// Set otel error handler
	otel.SetErrorHandler(h)
	log.Printf("trying to send a fake error to otel error handler")
	otel.Handle(fmt.Errorf("fake error for testing"))

	err = self_metrics.CollectOpsAgentSelfMetrics(ctx, userUc, mergedUc)
	if err != nil {
		log.Printf("Failed to run the diagnostics service: %v", err)
		cancel()
		return
	}
}

func getUserAndMergedConfigs(ctx context.Context, userConfPath string) (*confgenerator.UnifiedConfig, *confgenerator.UnifiedConfig, error) {
	userUc, err := confgenerator.ReadUnifiedConfigFromFile(ctx, userConfPath)
	if err != nil {
		return nil, nil, err
	}
	if userUc == nil {
		userUc = &confgenerator.UnifiedConfig{}
	}

	mergedUc, err := confgenerator.MergeConfFiles(ctx, userConfPath, apps.BuiltInConfStructs)
	if err != nil {
		return nil, nil, err
	}

	return userUc, mergedUc, nil
}

type otelErrorHandler struct {
	windowsEventLogger debug.Log
	windowsEventId     uint32
}

func (h *otelErrorHandler) Handle(err error) {
	log.Printf("handling otel error")
	h.windowsEventLogger.Error(h.windowsEventId, fmt.Sprintf("error collecting metrics: %v", err))
}
