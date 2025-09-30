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
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"

	pb "github.com/GoogleCloudPlatform/google-guest-agent/pkg/proto/plugin_comm"
	"google.golang.org/grpc/status"
)

const (
	OpsAgentConfigLocationLinux = "/etc/google-cloud-ops-agent/config.yaml"
	ConfGeneratorBinary         = "libexec/google_cloud_ops_agent_engine"
	AgentWrapperBinary          = "libexec/google_cloud_ops_agent_wrapper"
	FluentbitBinary             = "subagents/fluent-bit/bin/fluent-bit"
	OtelBinary                  = "subagents/opentelemetry-collector/otelopscol"

	LogsDirectory               = "log/google-cloud-ops-agent"
	FluentBitStateDiectory      = "state/fluent-bit"
	FluentBitRuntimeDirectory   = "run/google-cloud-ops-agent-fluent-bit"
	OtelRuntimeDirectory        = "run/google-cloud-ops-agent-opentelemetry-collector"
	DefaultPluginStateDirectory = "/var/lib/google-guest-agent/agent_state/plugins/ops-agent-plugin"
)

var (
	AgentServiceNameRegex    = regexp.MustCompile(`[\w-]+\.service`)
	AgentSystemdServiceNames = []string{"google-cloud-ops-agent.service", "stackdriver-agent.service", "google-fluentd.service"}
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

	pluginInstallPath, err := os.Executable()
	if err != nil {
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
		log.Printf("Start() failed, because it cannot determine the plugin install location: %s", err)
		return nil, status.Error(13, err.Error()) // Internal
	}
	pluginInstallPath, err = filepath.EvalSymlinks(pluginInstallPath)
	if err != nil {
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
		log.Printf("Start() failed, because it cannot determine the plugin install location: %s", err)
		return nil, status.Error(13, err.Error()) // Internal
	}
	pluginInstallDir := filepath.Dir(pluginInstallPath)

	pluginStateDir := msg.GetConfig().GetStateDirectoryPath()
	if pluginStateDir == "" {
		pluginStateDir = DefaultPluginStateDirectory
	}

	// Find existing ops agent installation, and conflicting legacy agent installation.
	foundConflictingInstallations, err := findPreExistentAgents(pContext, ps.runCommand, AgentSystemdServiceNames)
	if foundConflictingInstallations || err != nil {
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
		log.Printf("Start() failed: %s", err)
		return nil, status.Error(9, err.Error()) // FailedPrecondition
	}

	// Receive config from the Start request and write it to the Ops Agent config file.
	if err := writeCustomConfigToFile(msg, OpsAgentConfigLocationLinux); err != nil {
		log.Printf("Start() failed: %s", err)
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
		return nil, status.Errorf(13, "failed to write the custom Ops Agent config to file: %s", err) // Internal
	}

	// Ops Agent config validation
	if err := validateOpsAgentConfig(pContext, pluginInstallDir, pluginStateDir, ps.runCommand); err != nil {
		log.Printf("Start() failed: %s", err)
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
		return nil, status.Errorf(9, "failed to validate Ops Agent config: %s", err) // FailedPrecondition
	}
	// Subagent config generation
	if err := generateSubagentConfigs(pContext, ps.runCommand, pluginInstallDir, pluginStateDir); err != nil {
		log.Printf("Start() failed: %s", err)
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
		return nil, status.Errorf(9, "failed to generate subagent configs: %s", err) // FailedPrecondition
	}

	// the subagent startups
	cancelFunc := func() {
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
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
	// Register signal handler and implements its callback.
	sigHandler(ctx, func(_ os.Signal) {
		cancel()
	})

	var wg sync.WaitGroup

	// Starting Otel
	runOtelCmd := exec.CommandContext(ctx,
		path.Join(pluginInstallDirectory, OtelBinary),
		"--config", path.Join(pluginStateDirectory, OtelRuntimeDirectory, "otel.yaml"),
		"--feature-gates=receiver.prometheusreceiver.RemoveStartTimeAdjustment",
	)
	wg.Add(1)
	go runSubAgentCommand(ctx, cancel, runOtelCmd, runCommand, &wg)

	// Starting FluentBit
	runFluentBitCmd := exec.CommandContext(ctx,
		path.Join(pluginInstallDirectory, AgentWrapperBinary),
		"-config_path", OpsAgentConfigLocationLinux,
		"-log_path", path.Join(pluginStateDirectory, LogsDirectory, "subagents/logging-module.log"),
		path.Join(pluginInstallDirectory, FluentbitBinary),
		"--config", path.Join(pluginStateDirectory, FluentBitRuntimeDirectory, "fluent_bit_main.conf"),
		"--parser", path.Join(pluginStateDirectory, FluentBitRuntimeDirectory, "fluent_bit_parser.conf"),
		"--storage_path", path.Join(pluginStateDirectory, FluentBitStateDiectory, "buffers"),
	)
	wg.Add(1)
	go runSubAgentCommand(ctx, cancel, runFluentBitCmd, runCommand, &wg)

	wg.Wait()
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

// sigHandler handles SIGTERM, SIGINT etc signals. The function provided in the
// cancel argument handles internal framework termination and the plugin
// interface notification of the "exiting" state.
func sigHandler(ctx context.Context, cancel func(sig os.Signal)) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP)
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

func runCommand(cmd *exec.Cmd) (string, error) {
	if cmd == nil {
		return "", nil
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
	log.Printf("Running command: %s", cmd.Args)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Command %s failed, \ncommand output: %s\ncommand error: %s", cmd.Args, string(out), err)
	}
	return string(out), err
}

func validateOpsAgentConfig(ctx context.Context, pluginInstallDirectory string, pluginStateDirectory string, runCommand RunCommandFunc) error {
	configValidationCmd := exec.CommandContext(ctx,
		path.Join(pluginInstallDirectory, ConfGeneratorBinary),
		"-in", OpsAgentConfigLocationLinux,
		"-logs", path.Join(pluginStateDirectory, LogsDirectory),
	)
	if output, err := runCommand(configValidationCmd); err != nil {
		return fmt.Errorf("failed to validate the Ops Agent config:\ncommand output: %s\ncommand error: %s", output, err)
	}
	return nil
}

func generateSubagentConfigs(ctx context.Context, runCommand RunCommandFunc, pluginInstallDirectory string, pluginStateDirectory string) error {
	confGeneratorBinaryFullPath := path.Join(pluginInstallDirectory, ConfGeneratorBinary)
	otelConfigGenerationCmd := exec.CommandContext(ctx,
		confGeneratorBinaryFullPath,
		"-service", "otel",
		"-in", OpsAgentConfigLocationLinux,
		"-out", path.Join(pluginStateDirectory, OtelRuntimeDirectory),
		"-logs", path.Join(pluginStateDirectory, LogsDirectory))

	if output, err := runCommand(otelConfigGenerationCmd); err != nil {
		return fmt.Errorf("failed to generate Otel config:\ncommand output: %s\ncommand error: %s", output, err)
	}

	fluentBitConfigGenerationCmd := exec.CommandContext(ctx,
		confGeneratorBinaryFullPath,
		"-service", "fluentbit",
		"-in", OpsAgentConfigLocationLinux,
		"-out", path.Join(pluginStateDirectory, FluentBitRuntimeDirectory),
		"-logs", path.Join(pluginStateDirectory, LogsDirectory), "-state", path.Join(pluginStateDirectory, FluentBitStateDiectory))

	if output, err := runCommand(fluentBitConfigGenerationCmd); err != nil {
		return fmt.Errorf("failed to generate Fluntbit config:\ncommand output: %s\ncommand error: %s", output, err)
	}
	return nil
}

func findPreExistentAgents(ctx context.Context, runCommand RunCommandFunc, agentSystemdServiceNames []string) (bool, error) {
	cmdArgs := []string{"systemctl", "list-unit-files"}
	cmdArgs = append(cmdArgs, agentSystemdServiceNames...)
	findOpsAgentCmd := exec.CommandContext(ctx,
		cmdArgs[0], cmdArgs[1:]...,
	)
	output, err := runCommand(findOpsAgentCmd)
	if strings.Contains(output, "0 unit files listed.") {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("unable to verify the existing Ops Agent and legacy agent installations, error: %s", err)
	}
	alreadyInstalledAgents := AgentServiceNameRegex.FindAllString(output, -1)
	if len(alreadyInstalledAgents) == 0 {
		return false, nil
	}
	log.Printf("The following systemd services are already installed on the VM: %v\n command output: %v\ncommand error: %v", alreadyInstalledAgents, output, err)
	return true, fmt.Errorf("conflicting installations identified: %v", alreadyInstalledAgents)
}
