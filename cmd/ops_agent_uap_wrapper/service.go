package main

import (
	"context"

	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	pb "github.com/GoogleCloudPlatform/ops-agent/cmd/ops_agent_uap_wrapper/google_guest_agent/plugin"
)

const MaximumWaitForProcessStart = 5 * time.Second
const Prefix = "/opt/google-cloud-ops-agent" // @PREFIX@
const Sysconfdir = "/etc"                    // @SYSCONFDIR@
const LogsDirectory = "/var/log/google-cloud-ops-agent"
const FluentBitStateDiectory = "/var/lib/google-cloud-ops-agent/fluent-bit"
const FluentBitRuntimeDirectory = "/run/google-cloud-ops-agent-fluent-bit"
const OtelRuntimeDirectory = "/run/google-cloud-ops-agent-opentelemetry-collector"

// PluginServer implements the plugin RPC server interface.
type OpsAgentPluginServer struct {
	pb.UnimplementedGuestAgentPluginServer
	server *grpc.Server
	// cancel is the cancel function to be called when core plugin is stopped.
	cancel       context.CancelFunc
	startContext context.Context
}

// Apply applies the config sent or performs the work defined in the message.
// ApplyRequest is opaque to the agent and is expected to be well known contract
// between Plugin and the server itself. For e.g. service might want to update
// plugin config to enable/disable feature here plugins can react to such requests.
func (ps *OpsAgentPluginServer) Apply(ctx context.Context, msg *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	return &pb.ApplyResponse{}, nil
}

// sigHandler handles SIGTERM, SIGINT etc signals. The function provided in the
// cancel argument handles internal framework termination and the plugin
// interface notification of the "exiting" state.
func sigHandler(ctx context.Context, cancel func(sig os.Signal)) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP, syscall.SIGKILL)
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

func (ps *OpsAgentPluginServer) runAgent(ctx context.Context) {
	// Register signal handler and implements its callback.
	sigHandler(ctx, func(_ os.Signal) {
		// We're handling some external signal here, set cleanup to [false].
		// If this was Guest Agent trying to stop it would call [Stop] RPC directly
		// or do a [SIGKILL] which anyways cannot be intercepted.
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
	})

	defer func() {
		log.Print("Stopping Ops Agent plugin...")
		ps.server.GracefulStop()
	}()

	// Starting ExecStartPre commands
	execStartPreConfigValidationCmd := exec.CommandContext(ctx,
		Prefix+"/libexec/google_cloud_ops_agent_engine",
		"-in", Sysconfdir+"/google-cloud-ops-agent/config.yaml",
	)
	if err := runCommand(execStartPreConfigValidationCmd); err != nil {
		log.Fatalf("failed to validate Ops Agent default config.yaml: %s", err)
	}

	execStartPreOtelCmd := exec.CommandContext(ctx,
		Prefix+"/libexec/google_cloud_ops_agent_engine",
		"-service", "otel",
		"-in", Sysconfdir+"/google-cloud-ops-agent/config.yaml",
		"-out", OtelRuntimeDirectory,
		"-logs", LogsDirectory)

	if err := runCommand(execStartPreOtelCmd); err != nil {
		log.Fatalf("failed to generate config yaml for Otel: %s", err)
	}

	execStartPreFluentBitCmd := exec.CommandContext(ctx,
		Prefix+"/libexec/google_cloud_ops_agent_engine",
		"-service", "fluentbit",
		"-in", Sysconfdir+"/google-cloud-ops-agent/config.yaml",
		"-out", FluentBitRuntimeDirectory,
		"-logs", LogsDirectory, "-state", FluentBitStateDiectory)

	if err := runCommand(execStartPreFluentBitCmd); err != nil {
		log.Fatalf("failed to generate config yaml for FluentBit: %s", err)
	}
	// Starting Diagnostics Service
	execDiagnosticsCmd := exec.CommandContext(ctx,
		Prefix+"/libexec/google_cloud_ops_agent_diagnostics",
		"-config", Sysconfdir+"/google-cloud-ops-agent/config.yaml",
	)
	go restartCommand(execDiagnosticsCmd)

	// Starting Otel
	execOtelCmd := exec.CommandContext(ctx,
		Prefix+"/subagents/opentelemetry-collector/otelopscol",
		"--config", OtelRuntimeDirectory+"/otel.yaml",
	)
	go restartCommand(execOtelCmd)

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
	if err := restartCommand(execFluentBitCmd); err != nil {
		log.Fatal(err)
	}
}

// Start starts the plugin and initiates the plugin functionality.
// Until plugin receives Start request plugin is expected to be not functioning
// and just listening on the address handed off waiting for the request.
func (ps *OpsAgentPluginServer) Start(ctx context.Context, msg *pb.StartRequest) (*pb.StartResponse, error) {
	if ps.startContext != nil && ps.startContext.Err() == nil {
		log.Print("Ops Agent plugin is started already, skipping the current Start() request")
		return &pb.StartResponse{}, nil
	}
	pCtx, cancel := context.WithCancel(context.Background())
	ps.cancel = cancel
	ps.startContext = pCtx

	go ps.runAgent(pCtx)
	return &pb.StartResponse{}, nil
}

// Stop is the stop hook and implements any cleanup if required.
// Stop maybe called if plugin revision is being changed.
// For e.g. if plugins want to stop some task it was performing or remove some
// state before exiting it can be done on this request.
func (ps *OpsAgentPluginServer) Stop(ctx context.Context, msg *pb.StopRequest) (*pb.StopResponse, error) {
	if ps.cancel == nil || ps.startContext == nil || ps.startContext.Err() != nil {
		log.Print("Ops Agent plugin is already stoppped, skipping the current Stop() request")
		return &pb.StopResponse{}, nil

	}
	log.Printf("Handling stop request %+v, stopping core plugin...", msg)
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

func runCommand(cmd *exec.Cmd) error {
	if cmd == nil {
		return nil
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
		Setpgid:   true,
	}
	var outb, errb bytes.Buffer
	cmd.Stderr = &errb
	cmd.Stdout = &outb
	log.Printf("Running command: %s, with arguments: %s", cmd.Path, cmd.Args)
	if err := cmd.Run(); err != nil {
		fullError := fmt.Errorf("failed to execute cmd: %s with arguments %s, \ncommand output: %s\n error: %s %s", cmd.Path, cmd.Args, outb.String(), errb.String(), err)
		log.Print(fullError)
		return fullError
	}
	return nil
}

func restartCommand(cmd *exec.Cmd) error {
	if cmd == nil {
		return nil
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
		Setpgid:   true,
	}
	var outb, errb bytes.Buffer
	cmd.Stderr = &errb
	cmd.Stdout = &outb
	log.Printf("Restarting command: %s, with arguments: %s", cmd.Path, cmd.Args)
	err := cmd.Run()
	if err != nil {
		// https://pkg.go.dev/os#ProcessState.ExitCode Don't restart if the command was terminated by signals.
		fullError := fmt.Errorf("failed to execute cmd: %s with arguments %s, \ncommand output: %s\n error: %s %s", cmd.Path, cmd.Args, outb.String(), errb.String(), err)

		if exiterr, ok := err.(*exec.ExitError); ok && exiterr.ProcessState.ExitCode() == -1 {
			notRestartedError := fmt.Errorf("command terminated by signals, not restarting\n%s", fullError)
			log.Print(notRestartedError)
			return notRestartedError
		}
		log.Print(fullError)
	} else {
		log.Printf("command: %s, with arguments: %s completed successfully", cmd.Path, cmd.Args)
	}
	cmdToRestart := exec.Command(cmd.Path, cmd.Args...)
	return restartCommand(cmdToRestart)
}
