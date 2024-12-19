package main

import (
	"context"
	"strings"
	"sync"

	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/journal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	pb "github.com/GoogleCloudPlatform/ops-agent/cmd/ops_agent_uap_wrapper/google_guest_agent/plugin"
	"github.com/GoogleCloudPlatform/ops-agent/internal/logs"
)

const MaximumWaitForProcessStart = 5 * time.Second
const Prefix = "/opt/google-cloud-ops-agent" // @PREFIX@
const Sysconfdir = "/etc"                    // @SYSCONFDIR@
const LogsDirectory = "/var/log/google-cloud-ops-agent"
const FluentBitStateDiectory = "/var/lib/google-cloud-ops-agent/fluent-bit"
const FluentBitRuntimeDirectory = "/run/google-cloud-ops-agent-fluent-bit"
const OtelRuntimeDirectory = "/run/google-cloud-ops-agent-opentelemetry-collector"
const restartLimit = 3 // if a command crashes more than `restartLimit`` times consecutively, the command will no longer be restarted.

// PluginServer implements the plugin RPC server interface.
type OpsAgentPluginServer struct {
	pb.UnimplementedGuestAgentPluginServer
	server *grpc.Server
	// cancel is the cancel function to be called when core plugin is stopped.
	cancel       context.CancelFunc
	startContext context.Context
	logger       logs.StructuredLogger
}

// Apply applies the config sent or performs the work defined in the message.
// ApplyRequest is opaque to the agent and is expected to be well known contract
// between Plugin and the server itself. For e.g. service might want to update
// plugin config to enable/disable feature here plugins can react to such requests.
func (ps *OpsAgentPluginServer) Apply(ctx context.Context, msg *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	return &pb.ApplyResponse{}, nil
}
func (ps *OpsAgentPluginServer) Cancel() {
	if ps.cancel != nil {
		ps.cancel()
	}
}

// sigHandler only handles SIGTERM signal. The function provided in the
// cancel argument handles internal framework termination and the plugin
// interface notification of the "exiting" state.
func sigHandler(ctx context.Context, cancel func(sig os.Signal)) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-sigChan:
			log.Printf("Got signal: %d, leaving...", sig)
			close(sigChan)
			cancel(sig)
		case <-ctx.Done():
			break
		}
	}()
}

func (ps *OpsAgentPluginServer) findPreExistentAgent(ctx context.Context, serviceName string) (bool, error) {
	findOpsAgentCmd := exec.CommandContext(ctx,
		"systemctl", "status", serviceName,
	)
	output, err := runCommand(findOpsAgentCmd, ps.logger)
	if strings.Contains(output, fmt.Sprintf("Unit %s could not be found.", serviceName)) {
		return false, nil
	}
	if strings.Contains(output, "Loaded:") {
		return true, nil
	}

	if err != nil {
		return false, fmt.Errorf("unable to verify the existing installation of %s. Error: %s", serviceName, err)
	}
	return false, nil
}

func (ps *OpsAgentPluginServer) generateConfig(ctx context.Context) error {
	// Config Generation Tasks
	configValidationCmd := exec.CommandContext(ctx,
		Prefix+"/libexec/google_cloud_ops_agent_engine",
		"-in", Sysconfdir+"/google-cloud-ops-agent/config.yaml",
	)
	if _, err := runCommand(configValidationCmd, ps.logger); err != nil {
		errorWithDetails := fmt.Sprintf("%s failed to validate Ops Agent default config.yaml: %s", OpsAgentPluginSelfLogTag, err)
		journal.Print(journal.PriErr, errorWithDetails)
		ps.logger.Errorf("%s", errorWithDetails)
		return fmt.Errorf("%s", errorWithDetails)
	}

	otelConfigGenerationCmd := exec.CommandContext(ctx,
		Prefix+"/libexec/google_cloud_ops_agent_engine",
		"-service", "otel",
		"-in", Sysconfdir+"/google-cloud-ops-agent/config.yaml",
		"-out", OtelRuntimeDirectory,
		"-logs", LogsDirectory)

	if _, err := runCommand(otelConfigGenerationCmd, ps.logger); err != nil {
		errorWithDetails := fmt.Sprintf("%s failed to generate config yaml for Otel: %s", OpsAgentPluginSelfLogTag, err)
		journal.Print(journal.PriErr, errorWithDetails)
		ps.logger.Errorf(errorWithDetails)
		return fmt.Errorf("%s", errorWithDetails)
	}

	fluentBitConfigGenerationCmd := exec.CommandContext(ctx,
		Prefix+"/libexec/google_cloud_ops_agent_engine",
		"-service", "fluentbit",
		"-in", Sysconfdir+"/google-cloud-ops-agent/config.yaml",
		"-out", FluentBitRuntimeDirectory,
		"-logs", LogsDirectory, "-state", FluentBitStateDiectory)

	if _, err := runCommand(fluentBitConfigGenerationCmd, ps.logger); err != nil {
		errorWithDetails := fmt.Sprintf("failed to generate config yaml for FluentBit: %s", err)
		ps.logger.Errorf("%s", errorWithDetails)
		journal.Print(journal.PriErr, errorWithDetails)
		return fmt.Errorf("%s", errorWithDetails)
	}
	return nil
}

func (ps *OpsAgentPluginServer) runAgent(ctx context.Context) {
	// Register signal handler and implements its callback.
	sigHandler(ctx, func(_ os.Signal) {
		// We're handling some external signal here, set cleanup to [false].
		// If this was Guest Agent trying to stop it would call [Stop] RPC directly
		// or do a [SIGKILL] which anyways cannot be intercepted.
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
	})

	// Starting the Diagnostics service, and subagents.
	var wg sync.WaitGroup
	// Starting Diagnostics Service
	runDiagnosticsCmd := exec.CommandContext(ctx,
		Prefix+"/libexec/google_cloud_ops_agent_diagnostics",
		"-config", Sysconfdir+"/google-cloud-ops-agent/config.yaml",
	)
	wg.Add(1)
	go restartCommand(ctx, &wg, ps.logger, runDiagnosticsCmd, restartLimit, restartLimit)

	// Starting Otel
	execOtelCmd := exec.CommandContext(ctx,
		Prefix+"/subagents/opentelemetry-collector/otelopscol",
		"--config", OtelRuntimeDirectory+"/otel.yaml",
	)
	wg.Add(1)
	go restartCommand(ctx, &wg, ps.logger, execOtelCmd, restartLimit, restartLimit)

	// Starting FluentBit
	execFluentBitCmd := exec.CommandContext(ctx,
		Prefix+"/libexec/google_cloud_ops_agent_wrapper",
		"-config_path", Sysconfdir+"/google-cloud-ops-agent/config.yaml",
		"-log_path", LogsDirectory+"/subagents/logging-module.log",
		Prefix+"/subagents/fluent-bit/bin/fluent-bit",
		"--config", FluentBitRuntimeDirectory+"/fluent_bit_main.conf",
		"--parser", FluentBitRuntimeDirectory+"/fluent_bit_parser.conf",
		"--storage_path", FluentBitStateDiectory+"/buffers",
	)
	wg.Add(1)
	go restartCommand(ctx, &wg, ps.logger, execFluentBitCmd, restartLimit, restartLimit)
	wg.Wait()
	msg := "wait group has exited"
	ps.logger.Infof(msg)
	journal.Print(journal.PriInfo, msg)
}

// Start starts the plugin and initiates the plugin functionality.
// Until plugin receives Start request plugin is expected to be not functioning
// and just listening on the address handed off waiting for the request.
func (ps *OpsAgentPluginServer) Start(ctx context.Context, msg *pb.StartRequest) (*pb.StartResponse, error) {
	if ps.startContext != nil && ps.startContext.Err() == nil {
		skipMsg := fmt.Sprintf("%s Ops Agent plugin is started already, skipping the current Start() request", OpsAgentPluginSelfLogTag)
		journal.Print(journal.PriInfo, skipMsg)
		ps.logger.Infof(skipMsg)
		return &pb.StartResponse{}, nil
	}

	pCtx, cancel := context.WithCancel(context.Background())
	ps.cancel = cancel
	ps.startContext = pCtx
	// Find pre-existent ops agent installation, and conflicting legacy agent installation.
	// foundOpsAgent, err := ps.findPreExistentAgent(pCtx, "google-cloud-ops-agent.service")
	// if err != nil {
	// 	cancel()
	// 	journal.Print(journal.PriWarning, err.Error())
	// 	ps.logger.Warnf(err.Error())
	// 	return nil, status.Error(1, err.Error())
	// }
	// if foundOpsAgent {
	// 	cancel()
	// 	msg := "The Ops Agent is already installed; stopping the current installation."
	// 	journal.Print(journal.PriWarning, msg)
	// 	ps.logger.Warnf(msg)
	// 	return nil, status.Errorf(1, msg)
	// }
	foundLegacyMonitoringAgent, err := ps.findPreExistentAgent(pCtx, "stackdriver-agent.service")
	if err != nil {
		cancel()
		journal.Print(journal.PriWarning, err.Error())
		ps.logger.Warnf(err.Error())
		return nil, status.Error(1, fmt.Sprintf("%s", err))
	}
	if foundLegacyMonitoringAgent {
		cancel()
		msg := "The legacy monitoring agent is installed on the VM; stopping the current Ops Agent installation."
		journal.Print(journal.PriWarning, msg)
		ps.logger.Warnf(msg)
		return nil, status.Errorf(1, msg)
	}
	foundLegacyLoggingAgent, err := ps.findPreExistentAgent(pCtx, "google-fluentd.service")
	if err != nil {
		cancel()
		journal.Print(journal.PriWarning, err.Error())
		ps.logger.Warnf(err.Error())
		return nil, status.Error(1, fmt.Sprintf("%s", err))
	}
	if foundLegacyLoggingAgent {
		cancel()
		msg := "The legacy logging agent is installed on the VM; stopping the current Ops Agent installation."
		journal.Print(journal.PriWarning, msg)
		ps.logger.Warnf(msg)
		return nil, status.Errorf(1, msg)
	}

	// No pre-existent installations identified; start the new Ops Agent installation with config validation and config generation.
	if err := ps.generateConfig(pCtx); err != nil {
		cancel()
		journal.Print(journal.PriWarning, err.Error())
		ps.logger.Warnf(err.Error())
		return nil, status.Errorf(1, fmt.Sprintf("%s", err))
	}

	go ps.runAgent(pCtx)
	return &pb.StartResponse{}, nil
}

// Stop is the stop hook and implements any cleanup if required.
// Stop maybe called if plugin revision is being changed.
// For e.g. if plugins want to stop some task it was performing or remove some
// state before exiting it can be done on this request.
func (ps *OpsAgentPluginServer) Stop(ctx context.Context, msg *pb.StopRequest) (*pb.StopResponse, error) {
	if ps.cancel == nil || ps.startContext == nil || ps.startContext.Err() != nil {
		warningMsg := fmt.Sprintf("%s Ops Agent plugin is already stoppped, skipping the current Stop() request", OpsAgentPluginSelfLogTag)
		journal.Print(journal.PriWarning, warningMsg)
		ps.logger.Warnf(warningMsg)
		return &pb.StopResponse{}, nil

	}
	infoMsg := fmt.Sprintf("%s Handling stop request %+v, stopping core plugin...", OpsAgentPluginSelfLogTag, msg)
	journal.Print(journal.PriInfo, infoMsg)
	ps.logger.Infof(infoMsg)
	ps.cancel()
	return &pb.StopResponse{}, nil
}

// GetStatus is the health check agent would perform to make sure plugin process
// is alive. If request fails process is considered dead and relaunched. Plugins
// can share any additional information to report it to the service. For e.g. if
// plugins detect some non-fatal errors causing it unable to offer some features
// it can reported in status which is sent back to the service by agent.
func (ps *OpsAgentPluginServer) GetStatus(ctx context.Context, msg *pb.GetStatusRequest) (*pb.Status, error) {
	if err := ps.startContext.Err(); err != nil {
		// The context started by the Start() call has been cancelled(), which implies Stop() has been triggerred.
		return &pb.Status{Code: 1, Results: []string{"Plugin is not running ok"}}, nil
	}
	return &pb.Status{Code: 0, Results: []string{"Plugin is running ok"}}, nil
}

func runCommand(cmd *exec.Cmd, logger logs.StructuredLogger) (string, error) {
	if cmd == nil {
		return "", nil
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
		Setpgid:   true,
	}
	var outb, errb bytes.Buffer
	cmd.Stderr = &errb
	cmd.Stdout = &outb
	msg := fmt.Sprintf("%s Running command: %s, with arguments: %s", OpsAgentPluginSelfLogTag, cmd.Path, cmd.Args)
	logger.Infof(msg)
	journal.Print(journal.PriInfo, "%s", msg)
	if err := cmd.Run(); err != nil {
		fullError := fmt.Errorf("failed to execute cmd: %s with arguments %s, \ncommand output: %s\n error: %s %s", cmd.Path, cmd.Args, outb.String(), errb.String(), err)
		return fmt.Sprintf("%s %s", outb.String(), errb.String()), fullError
	}
	return fmt.Sprintf("%s %s", outb.String(), errb.String()), nil
}

func restartCommand(ctx context.Context, wg *sync.WaitGroup, logger logs.StructuredLogger, cmd *exec.Cmd, remainingRetry int, totalRetry int) {
	defer wg.Done()
	if cmd == nil {
		return
	}
	if ctx.Err() != nil {
		// context has been cancelled
		msg := fmt.Sprintf("%s Context has been cancelled, exiting", OpsAgentPluginSelfLogTag)
		journal.Print(journal.PriWarning, msg)
		logger.Warnf(msg)
		return
	}
	if remainingRetry == 0 {
		msg := fmt.Sprintf("%s exhaused all retry attempts, not restarting cmd", OpsAgentPluginSelfLogTag)
		journal.Print(journal.PriWarning, msg)
		logger.Warnf(msg)
		return
	}

	childCtx, ctxCancel := context.WithCancel(ctx)
	defer childCtx.Done()
	sigHandler(childCtx, func(_ os.Signal) {
		// We're handling some external signal here, set cleanup to [false].
		// If this was Guest Agent trying to stop it would call [Stop] RPC directly
		// or do a [SIGKILL] which anyways cannot be intercepted.
		ctxCancel()
	})
	cmd = exec.CommandContext(childCtx, cmd.Path, cmd.Args[1:]...)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
		Setpgid:   true,
	}
	var outb, errb bytes.Buffer
	cmd.Stderr = &errb
	cmd.Stdout = &outb
	msg := fmt.Sprintf("%s Restarting command: %s, with arguments: %s", OpsAgentPluginSelfLogTag, cmd.Path, cmd.Args)
	logger.Infof(msg)
	journal.Print(journal.PriInfo, "%s", msg)
	err := cmd.Run()
	retryCount := remainingRetry
	if err != nil {
		// https://pkg.go.dev/os#ProcessState.ExitCode Don't restart if the command was terminated by signals.
		fullError := fmt.Sprintf("%s failed to execute cmd: %s with arguments %s, \ncommand output: %s\n error: %s %s", OpsAgentPluginSelfLogTag, cmd.Path, cmd.Args, outb.String(), errb.String(), err)

		if exiterr, ok := err.(*exec.ExitError); ok && exiterr.ProcessState.ExitCode() == -1 {
			notRestartedError := fmt.Sprintf("%s command terminated by signals, not restarting\n%s", OpsAgentPluginSelfLogTag, fullError)
			logger.Errorf(notRestartedError)
			journal.Print(journal.PriErr, "%s", notRestartedError)
			return
		}
		logger.Errorf(fullError)
		journal.Print(journal.PriErr, "%s", fullError)
		retryCount -= 1

	} else {
		msg := fmt.Sprintf("%s command: %s, with arguments: %s completed successfully", OpsAgentPluginSelfLogTag, cmd.Path, cmd.Args)
		logger.Infof(msg)
		journal.Print(journal.PriInfo, msg)
		retryCount = totalRetry
	}
	// Sleep 10 seconds before retarting the task
	time.Sleep(5 * time.Second)
	wg.Add(1)
	go restartCommand(childCtx, wg, logger, cmd, retryCount, totalRetry)
}
