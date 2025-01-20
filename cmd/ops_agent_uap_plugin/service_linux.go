// Copyright 2025 Google LLC
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

//go:build !windows

package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
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

// RunCommandFunc defines a function type that takes an exec.Cmd and returns
// its output and error. This abstraction is introduced
// primarily to facilitate testing by allowing the injection of mock
// implementations.
type RunCommandFunc func(cmd *exec.Cmd) (string, error)

// PluginServer implements the plugin RPC server interface.
type OpsAgentPluginServer struct {
	pb.UnimplementedGuestAgentPluginServer
	server     *grpc.Server
	cancel     context.CancelFunc
	runCommand RunCommandFunc
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

	// Find pre-existent ops agent installation, and conflicting legacy agent installation.
	foundOpsAgent, err := findPreExistentAgent(pContext, ps.runCommand, "google-cloud-ops-agent.service")
	if err != nil || foundOpsAgent {
		ps.cancel()
		ps.cancel = nil
		errMsg := fmt.Sprintf("found pre-existent Ops Agent: %v, err: %v", foundOpsAgent, err)
		log.Printf("Start() failed: %s ", errMsg)
		return nil, status.Error(1, errMsg)
	}

	foundLegacyMonitoringAgent, err := findPreExistentAgent(pContext, ps.runCommand, "stackdriver-agent.service")
	if err != nil || foundLegacyMonitoringAgent {
		ps.cancel()
		ps.cancel = nil
		errMsg := fmt.Sprintf("found pre-existent Legacy Monitoring Agent: %v, err: %v", foundLegacyMonitoringAgent, err)
		log.Printf("Start() failed: %s ", errMsg)
		return nil, status.Error(1, errMsg)
	}

	foundLegacyLoggingAgent, err := findPreExistentAgent(pContext, ps.runCommand, "google-fluentd.service")
	if err != nil || foundLegacyLoggingAgent {
		ps.cancel()
		ps.cancel = nil
		errMsg := fmt.Sprintf("found pre-existent Legacy Logging Agent: %v, err: %v", foundLegacyLoggingAgent, err)
		log.Printf("Start() failed: %s", errMsg)
		return nil, status.Error(1, errMsg)
	}

	// Ops Agent config validation
	if err := validateOpsAgentConfig(pContext, ps.runCommand, pluginStateDir); err != nil {
		log.Printf("Start() failed: %s", err)
		ps.cancel()
		ps.cancel = nil
		return nil, status.Errorf(1, "failed to validate Ops Agent config: %s", err)
	}
	// Subagent config generation
	if err := generateSubagentConfigs(pContext, ps.runCommand, pluginStateDir); err != nil {
		log.Printf("Start() failed: %s", err)
		ps.cancel()
		ps.cancel = nil
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

func runCommand(cmd *exec.Cmd) (string, error) {
	if cmd == nil {
		return "", nil
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
	log.Printf("Running command: %s", cmd.Args)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("Command %s failed, \ncommand output: %s\ncommand error: %s", cmd.Args, string(out), err)
		return string(out), err
	}
	return "", nil
}

func validateOpsAgentConfig(ctx context.Context, runCommand RunCommandFunc, pluginBaseLocation string) error {
	configValidationCmd := exec.CommandContext(ctx,
		pluginBaseLocation+"/"+ConfGeneratorBinary,
		"-in", OpsAgentConfigLocationLinux,
	)
	if output, err := runCommand(configValidationCmd); err != nil {
		return fmt.Errorf("failed to validate the Ops Agent config:\ncommand output: %s\ncommand error: %s", output, err)
	}
	return nil
}

func generateSubagentConfigs(ctx context.Context, runCommand RunCommandFunc, pluginBaseLocation string) error {
	otelConfigGenerationCmd := exec.CommandContext(ctx,
		pluginBaseLocation+"/"+ConfGeneratorBinary,
		"-service", "otel",
		"-in", OpsAgentConfigLocationLinux,
		"-out", pluginBaseLocation+"/"+OtelRuntimeDirectory,
		"-logs", pluginBaseLocation+"/"+LogsDirectory)

	if _, err := runCommand(otelConfigGenerationCmd); err != nil {
		return fmt.Errorf("failed to generate Otel config: %s", err)
	}

	fluentBitConfigGenerationCmd := exec.CommandContext(ctx,
		pluginBaseLocation+"/libexec/google_cloud_ops_agent_engine",
		"-service", "fluentbit",
		"-in", OpsAgentConfigLocationLinux,
		"-out", pluginBaseLocation+"/"+FluentBitRuntimeDirectory,
		"-logs", pluginBaseLocation+"/"+LogsDirectory, "-state", pluginBaseLocation+"/"+FluentBitStateDiectory)

	if output, err := runCommand(fluentBitConfigGenerationCmd); err != nil {
		return fmt.Errorf("failed to generate Fluntbit config:\ncommand output: %s\ncommand error: %s", output, err)
	}
	return nil
}

func findPreExistentAgent(ctx context.Context, runCommand RunCommandFunc, serviceName string) (bool, error) {
	findOpsAgentCmd := exec.CommandContext(ctx,
		"systemctl", "status", serviceName,
	)
	output, err := runCommand(findOpsAgentCmd)
	if strings.Contains(output, fmt.Sprintf("Unit %s could not be found.", serviceName)) || strings.Contains(output, "Loaded: not-found") {
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
