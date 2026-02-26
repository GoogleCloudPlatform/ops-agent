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
	"io"
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
	OtelStateDiectory           = "state/opentelemetry-collector"
	OtelRuntimeDirectory        = "run/google-cloud-ops-agent-opentelemetry-collector"
	DefaultPluginStateDirectory = "/var/lib/google-guest-agent/agent_state/plugins/ops-agent-plugin"
)

var (
	AgentServiceNameRegex    = regexp.MustCompile(`[\w-]+\.service`)
	AgentSystemdServiceNames = []string{"google-cloud-ops-agent.service", "stackdriver-agent.service", "google-fluentd.service"}
)

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
		ps.cancelAndSetPluginError(&OpsAgentPluginError{Message: fmt.Sprintf("Start() failed, because it cannot determine the plugin install location: %s", err), ShouldRestart: false})
		return &pb.StartResponse{}, nil
	}
	pluginInstallPath, err = filepath.EvalSymlinks(pluginInstallPath)
	if err != nil {
		ps.cancelAndSetPluginError(&OpsAgentPluginError{Message: fmt.Sprintf("Start() failed, because it cannot determine the plugin install location: %s", err), ShouldRestart: false})
		return &pb.StartResponse{}, nil
	}

	pluginInstallDir := filepath.Dir(pluginInstallPath)
	pluginStateDir := msg.GetConfig().GetStateDirectoryPath()
	if pluginStateDir == "" {
		pluginStateDir = DefaultPluginStateDirectory
	}

	// Find existing ops agent installation, and conflicting legacy agent installation.
	foundConflictingInstallations, err := findPreExistentAgents(pContext, ps.runCommand, AgentSystemdServiceNames)
	if foundConflictingInstallations || err != nil {
		ps.cancelAndSetPluginError(&OpsAgentPluginError{Message: fmt.Sprintf("Start() failed, because it detected agent installations unmanaged by the VM Extension Manager: %s", err), ShouldRestart: false})
		return &pb.StartResponse{}, nil
	}

	// Receive config from the Start request and write it to the Ops Agent config file.
	if err := writeCustomConfigToFile(msg, OpsAgentConfigLocationLinux); err != nil {
		ps.cancelAndSetPluginError(&OpsAgentPluginError{Message: fmt.Sprintf("Start() failed to write the custom Ops Agent config to file: %s", err), ShouldRestart: false})
		return &pb.StartResponse{}, nil
	}

	// Ops Agent config validation
	if err := validateOpsAgentConfig(pContext, pluginInstallDir, pluginStateDir, ps.runCommand); err != nil {
		ps.cancelAndSetPluginError(&OpsAgentPluginError{Message: fmt.Sprintf("Start() failed to validate the custom Ops Agent config: %s", err), ShouldRestart: false})
		return &pb.StartResponse{}, nil
	}
	// Subagent config generation
	if err := generateSubagentConfigs(pContext, ps.runCommand, pluginInstallDir, pluginStateDir); err != nil {
		ps.cancelAndSetPluginError(&OpsAgentPluginError{Message: fmt.Sprintf("Start() failed to generate subagent configs: %s", err), ShouldRestart: false})
		return &pb.StartResponse{}, nil
	}

	// the subagent startups
	go runSubagents(pContext, ps.cancelAndSetPluginError, pluginInstallDir, pluginStateDir, runSubAgentCommand, ps.runCommand)
	return &pb.StartResponse{}, nil
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
// cancelAndSetError: should be called by subagents from within go routines. It cancels the parent context, and collects the runtime errors from subagents and record them. The recorded errors are surfaced to users via GetStatus().
func runSubagents(ctx context.Context, cancelAndSetError CancelContextAndSetPluginErrorFunc, pluginInstallDirectory string, pluginStateDirectory string, runSubAgentCommand RunSubAgentCommandFunc, runCommand RunCommandFunc) {
	// Register signal handler and implements its callback.
	sigHandler(ctx, func(s os.Signal) {
		cancelAndSetError(&OpsAgentPluginError{Message: fmt.Sprintf("Received signal: %s, stopping the Ops Agent", s.String()), ShouldRestart: true})
	})

	var wg sync.WaitGroup

	// Starting Otel
	runOtelCmd := exec.CommandContext(ctx,
		path.Join(pluginInstallDirectory, OtelBinary),
		"--config", path.Join(pluginStateDirectory, OtelRuntimeDirectory, "otel.yaml"),
		"--feature-gates=receiver.prometheusreceiver.RemoveStartTimeAdjustment",
	)
	wg.Add(1)
	go runSubAgentCommand(ctx, cancelAndSetError, runOtelCmd, runCommand, &wg)

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
	go runSubAgentCommand(ctx, cancelAndSetError, runFluentBitCmd, runCommand, &wg)

	wg.Wait()
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
		"-logs", path.Join(pluginStateDirectory, LogsDirectory),
		"-state", path.Join(pluginStateDirectory, OtelStateDiectory))

	if output, err := runCommand(otelConfigGenerationCmd); err != nil {
		return fmt.Errorf("failed to generate Otel config:\ncommand output: %s\ncommand error: %s", output, err)
	}

	fluentBitConfigGenerationCmd := exec.CommandContext(ctx,
		confGeneratorBinaryFullPath,
		"-service", "fluentbit",
		"-in", OpsAgentConfigLocationLinux,
		"-out", path.Join(pluginStateDirectory, FluentBitRuntimeDirectory),
		"-logs", path.Join(pluginStateDirectory, LogsDirectory),
		"-state", path.Join(pluginStateDirectory, FluentBitStateDiectory))

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

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

func createLogger() (io.Closer, error) {
	return nopCloser{}, nil
}
