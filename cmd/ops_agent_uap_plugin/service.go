package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	pb "github.com/GoogleCloudPlatform/ops-agent/cmd/ops_agent_uap_plugin/google_guest_agent/plugin"
)

const (
	OpsAgentConfigLocationLinux = "/etc/google-cloud-ops-agent/config.yaml"
	ConfGeneratorBinary         = "libexec/google_cloud_ops_agent_engine"
	LogsDirectory               = "log/google-cloud-ops-agent"
	FluentBitStateDiectory      = "state/fluent-bit"
	FluentBitRuntimeDirectory   = "run/google-cloud-ops-agent-fluent-bit"
	OtelRuntimeDirectory        = "run/google-cloud-ops-agent-opentelemetry-collector"
	DefaultPluginStateDirectory = "/var/lib/google-guest-agent/plugins/ops-agent-plugin"
)

// PluginServer implements the plugin RPC server interface.
type OpsAgentPluginServer struct {
	pb.UnimplementedGuestAgentPluginServer
	server *grpc.Server
	// cancel is the cancel function to be called when core plugin is stopped.
	cancel context.CancelFunc
}

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
	if ps.cancel != nil {
		log.Printf("The Ops Agent plugin is started already, skipping the current request")
		return &pb.StartResponse{}, nil
	}
	log.Printf("Received a Start request: %s. Starting the Ops Agent", msg)

	pContext, cancel := context.WithCancel(context.Background())
	ps.cancel = cancel

	pluginStateDir := msg.GetConfig().GetStateDirectoryPath()
	if pluginStateDir == "" {
		pluginStateDir = DefaultPluginStateDirectory
	}
	// Ops Agent config validation
	if err := validateOpsAgentConfig(pContext, pluginStateDir); err != nil {
		log.Printf("failed to validate Ops Agent config: %s", err)
		return nil, status.Errorf(1, "failed to validate Ops Agent config: %s", err)
	}
	// Subagent config generation
	if err := generateSubagentConfigs(pContext, pluginStateDir); err != nil {
		log.Printf("failed to generate subagent configs: %s", err)
		return nil, status.Errorf(1, "failed to generate subagent configs: %s", err)
	}

	return &pb.StartResponse{}, nil
}

// Stop is the stop hook and implements any cleanup if required.
// Stop maybe called if plugin revision is being changed.
// For e.g. if plugins want to stop some task it was performing or remove some
// state before exiting it can be done on this request.
func (ps *OpsAgentPluginServer) Stop(ctx context.Context, msg *pb.StopRequest) (*pb.StopResponse, error) {
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
	if ps.cancel == nil {
		log.Println("The Ops Agent plugin is not running")
		return &pb.Status{Code: 1, Results: []string{"The Ops Agent Plugin is not running."}}, nil
	}
	log.Println("The Ops Agent plugin is running")
	return &pb.Status{Code: 0, Results: []string{"The Ops Agent Plugin is running ok."}}, nil
}

func runCommand(cmd *exec.Cmd) error {
	if cmd == nil {
		return nil
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
	log.Printf("Running command: %s, with arguments: %s", cmd.Path, cmd.Args)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to execute cmd: %s with arguments %s, \ncommand output: %s\ncommand error: %s", cmd.Path, cmd.Args, out, err)
	}
	return nil
}

func validateOpsAgentConfig(ctx context.Context, pluginBaseLocation string) error {
	configValidationCmd := exec.CommandContext(ctx,
		pluginBaseLocation+"/"+ConfGeneratorBinary,
		"-in", OpsAgentConfigLocationLinux,
	)
	if err := runCommand(configValidationCmd); err != nil {
		return fmt.Errorf("failed to validate the Ops Agent config: %s", err)
	}
	return nil
}

func generateSubagentConfigs(ctx context.Context, pluginBaseLocation string) error {
	otelConfigGenerationCmd := exec.CommandContext(ctx,
		pluginBaseLocation+"/"+ConfGeneratorBinary,
		"-service", "otel",
		"-in", OpsAgentConfigLocationLinux,
		"-out", pluginBaseLocation+"/"+OtelRuntimeDirectory,
		"-logs", pluginBaseLocation+"/"+LogsDirectory)

	if err := runCommand(otelConfigGenerationCmd); err != nil {
		return fmt.Errorf("failed to generate Otel config: %s", err)
	}

	fluentBitConfigGenerationCmd := exec.CommandContext(ctx,
		pluginBaseLocation+"/libexec/google_cloud_ops_agent_engine",
		"-service", "fluentbit",
		"-in", OpsAgentConfigLocationLinux,
		"-out", pluginBaseLocation+"/"+FluentBitRuntimeDirectory,
		"-logs", pluginBaseLocation+"/"+LogsDirectory, "-state", pluginBaseLocation+"/"+FluentBitStateDiectory)

	if err := runCommand(fluentBitConfigGenerationCmd); err != nil {
		return fmt.Errorf("failed to generate Fluntbit config: %s", err)
	}
	return nil
}
