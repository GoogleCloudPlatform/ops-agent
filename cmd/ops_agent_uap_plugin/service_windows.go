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
	"unsafe"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/internal/healthchecks"
	"github.com/GoogleCloudPlatform/ops-agent/internal/logs"
	"github.com/GoogleCloudPlatform/ops-agent/internal/self_metrics"
	"github.com/kardianos/osext"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
	"google.golang.org/grpc/status"

	pb "github.com/GoogleCloudPlatform/google-guest-agent/pkg/proto/plugin_comm"
)

const (
	GeneratedConfigsOutDir           = "generated_configs"
	LogsDirectory                    = "log"
	RuntimeDirectory                 = "run"
	OpsAgentUAPPluginEventID  uint32 = 8
	WindowsEventLogIdentifier        = "google-cloud-ops-agent-uap-plugin"
	WindowJobHandleIdentifier        = "google-cloud-ops-agent-uap-plugin-job-handle"
	AgentWrapperBinary               = "google-cloud-ops-agent-wrapper.exe"
	FluentbitBinary                  = "fluent-bit.exe"
	OtelBinary                       = "google-cloud-metrics-agent_windows_amd64.exe"
)

var (
	AgentWindowsServiceName       = []string{"StackdriverLogging", "StackdriverMonitoring", "google-cloud-ops-agent"}
	DefaultPluginStateDirectory   = filepath.Join(os.Getenv("PROGRAMDATA"), "Google/Compute Engine/google-guest-agent/agent_state/plugins/ops-agent-plugin")
	OpsAgentConfigLocationWindows = filepath.Join("C:", "Program Files/Google/Cloud Operations/Ops Agent/config/config.yaml")
)

// RunSubAgentCommandFunc defines a function type that starts a subagent. If one subagent execution exited, other sugagents are also terminated via context cancellation. This abstraction is introduced
// primarily to facilitate testing by allowing the injection of mock
// implementations.
type RunSubAgentCommandFunc func(ctx context.Context, cancel context.CancelFunc, cmd *exec.Cmd, runCommand RunCommandFunc, wg *sync.WaitGroup)

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

	// Detect conflicting installations.
	preInstalledAgents, err := findPreExistentAgents(&windowsServiceManager{}, AgentWindowsServiceName)
	if len(preInstalledAgents) != 0 || err != nil {
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
		log.Printf("Start() failed: %s", err)
		return nil, status.Error(9, err.Error()) // FailedPrecondition
	}

	// Calculate plugin install and state dirs.
	pluginInstallDir, err := osext.ExecutableFolder()
	if err != nil {
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
		log.Printf("Start() failed, because it cannot determine the plugin install location: %s", err)
		return nil, status.Error(13, err.Error()) // Internal
	}

	pluginStateDir := msg.GetConfig().GetStateDirectoryPath()
	if pluginStateDir == "" {
		pluginStateDir = DefaultPluginStateDirectory
	}
	log.Printf("Determined pluginInstallDir: %v, and pluginStateDir: %v", pluginInstallDir, pluginStateDir)

	// Create a windows Event logger. This is used to log generated subagent configs, and health check results.
	windowsEventLogger, err := createWindowsEventLogger()
	if err != nil {
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
		log.Printf("Start() failed, because it failed to create Windows event logger: %s", err)
		return nil, status.Error(13, err.Error()) // Internal
	}

	// Receive config from the Start request and write it to the Ops Agent config file.
	if err := writeCustomConfigToFile(msg, OpsAgentConfigLocationWindows); err != nil {
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
		windowsEventLogger.Close()
		log.Printf("Start() failed, because it failed to write the custom Ops Agent config to file: %s", err)
		return nil, status.Errorf(13, "failed to write the custom Ops Agent config to file: %s", err) // Internal
	}

	// Subagents config validation and generation.
	if err := generateSubAgentConfigs(ctx, OpsAgentConfigLocationWindows, pluginStateDir, windowsEventLogger); err != nil {
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
		windowsEventLogger.Close()
		log.Printf("Start() failed at the subagent config validation and generation step: %s", err)
		return nil, status.Error(9, err.Error()) // FailedPrecondition
	}

	// Trigger Healthchecks.
	healthCheckFileLogger := healthchecks.CreateHealthChecksLogger(filepath.Join(pluginStateDir, LogsDirectory))
	runHealthChecks(healthCheckFileLogger, windowsEventLogger)

	// Create a Windows Job object and stores its handle, to ensure that all child processes are killed when the parent process exits.
	_, err = createWindowsJobHandle()
	if err != nil {
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
		windowsEventLogger.Close()
		log.Printf("Start() failed, because it failed to create a Windows Job object: %s", err)
		return nil, status.Error(13, err.Error()) // Internal
	}

	cancelFunc := func() {
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
		windowsEventLogger.Close()
	}

	go runSubagents(pContext, cancelFunc, pluginInstallDir, pluginStateDir, runSubAgentCommand, ps.runCommand)

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

// serviceManager is an interface to abstract the Windows service manager. This is used to facilitate testing.
type serviceManager interface {
	Connect() (serviceManagerConnection, error)
}

// serviceManagerConnection is an interface to abstract the connection to the Windows service manager. This is used to facilitate testing.
type serviceManagerConnection interface {
	ListServices() ([]string, error)
	Disconnect() error
}

type windowsServiceManager struct{}

type windowsServiceManagerConn struct {
	mgr *mgr.Mgr
}

func (w *windowsServiceManager) Connect() (serviceManagerConnection, error) {
	m, err := mgr.Connect()
	if err != nil {
		return nil, err
	}
	return &windowsServiceManagerConn{mgr: m}, nil
}

func (c *windowsServiceManagerConn) ListServices() ([]string, error) {
	return c.mgr.ListServices()
}

func (c *windowsServiceManagerConn) Disconnect() error {
	return c.mgr.Disconnect()
}

// findPreExistentAgents checks if any of the Ops Agent and legacy agents are already installed as Window Services.
func findPreExistentAgents(mgr serviceManager, agentWindowsServiceNames []string) ([]string, error) {
	conn, err := mgr.Connect()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to service manager: %s", err)
	}
	defer conn.Disconnect()

	installedServices, err := conn.ListServices()
	if err != nil {
		return nil, fmt.Errorf("failed to list installed Windows services: %s", err)
	}

	installedServicesSet := make(map[string]bool)
	for _, s := range installedServices {
		installedServicesSet[s] = true
	}

	alreadyInstalledAgentServiceNames := []string{}
	for _, s := range agentWindowsServiceNames {
		if installedServicesSet[s] {
			alreadyInstalledAgentServiceNames = append(alreadyInstalledAgentServiceNames, s)
		}
	}

	if len(alreadyInstalledAgentServiceNames) != 0 {
		return alreadyInstalledAgentServiceNames, fmt.Errorf("conflicting installations identified: %v", alreadyInstalledAgentServiceNames)
	}
	return alreadyInstalledAgentServiceNames, nil
}

// eventLogWriter implements the io.Writer interface. It writes logs to the Windows Event Log.
type eventLogWriter struct {
	EventID  uint32
	EventLog *eventlog.Log
}

func (w *eventLogWriter) Write(p []byte) (int, error) {
	err := w.EventLog.Info(w.EventID, string(p))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func createWindowsEventLogger() (debug.Log, error) {
	eventlog.InstallAsEventCreate(WindowsEventLogIdentifier, eventlog.Error|eventlog.Warning|eventlog.Info)
	elog, err := eventlog.Open(WindowsEventLogIdentifier)
	if err != nil {
		return nil, err
	}

	// ConfGenerator might log messages to stdout, redirect them to the windows event log.
	log.SetOutput(&eventLogWriter{
		EventID:  OpsAgentUAPPluginEventID,
		EventLog: elog,
	})
	return elog, nil
}

func generateSubAgentConfigs(ctx context.Context, userConfigPath string, pluginStateDir string, windowsEventLogger debug.Log) error {
	uc, err := confgenerator.MergeConfFiles(ctx, userConfigPath, apps.BuiltInConfStructs)
	if err != nil {
		return err
	}

	windowsEventLogger.Info(OpsAgentUAPPluginEventID, fmt.Sprintf("Built-in config:\n%s\n", apps.BuiltInConfStructs["windows"]))
	windowsEventLogger.Info(OpsAgentUAPPluginEventID, fmt.Sprintf("Merged config:\n%s\n", uc))

	// The generated otlp metric json files are used only by the otel service.
	if err = self_metrics.GenerateOpsAgentSelfMetricsOTLPJSON(ctx, userConfigPath, filepath.Join(pluginStateDir, GeneratedConfigsOutDir, "otel")); err != nil {
		return err
	}

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

func runHealthChecks(healthCheckFileLogger logs.StructuredLogger, windowsEventLogger debug.Log) {
	gceHealthChecks := healthchecks.HealthCheckRegistryFactory()

	// Log health check results to health-checks.log log file.
	healthCheckResults := gceHealthChecks.RunAllHealthChecks(healthCheckFileLogger)

	// Log health check results to windows event log too.
	healthCheckWindowsEventLogger := logs.WindowsServiceLogger{EventID: OpsAgentUAPPluginEventID, Logger: windowsEventLogger}
	healthchecks.LogHealthCheckResults(healthCheckResults, healthCheckWindowsEventLogger)
	windowsEventLogger.Info(OpsAgentUAPPluginEventID, "Health checks completed")
}

func createWindowsJobHandle() (windows.Handle, error) {
	jobHandle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	_, err = windows.SetInformationJobObject(
		jobHandle,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)))
	if err != nil {
		windows.CloseHandle(jobHandle)
		return 0, err
	}

	// Assign the current process to the job object. This ensures that all child processes are automatically assigned to the same Job object.
	err = windows.AssignProcessToJobObject(jobHandle, windows.CurrentProcess())

	if err != nil {
		windows.CloseHandle(jobHandle)
		return 0, err
	}

	return jobHandle, nil
}

// runSubagents starts up otel and fluent bit subagents in separate goroutines.
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
func runSubagents(ctx context.Context, cancel context.CancelFunc, pluginInstallDirectory string, pluginStateDirectory string, runSubAgentCommand RunSubAgentCommandFunc, runCommand RunCommandFunc) {

	var wg sync.WaitGroup

	// Starting Otel
	runOtelCmd := exec.CommandContext(ctx,
		path.Join(pluginInstallDirectory, OtelBinary),
		"--config", path.Join(pluginStateDirectory, GeneratedConfigsOutDir, "otel/otel.yaml"),
		"--feature-gates=receiver.prometheusreceiver.RemoveStartTimeAdjustment",
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
