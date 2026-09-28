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
	"syscall"

	pb "github.com/GoogleCloudPlatform/google-guest-agent/pkg/proto/plugin_comm"
)

const (
	OpsAgentConfigLocationLinux = "/etc/google-cloud-ops-agent/config.yaml"
	OtelBinary                  = "subagents/opentelemetry-collector/otelopscol"

	LogsDirectory               = "log/google-cloud-ops-agent"
	OtelStateDirectory          = "state/opentelemetry-collector"
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
	if err := validateOpsAgentConfig(pContext, ps.runCommand, pluginInstallDir, pluginStateDir); err != nil {
		ps.cancelAndSetPluginError(&OpsAgentPluginError{Message: fmt.Sprintf("Start() failed to validate the custom Ops Agent config: %s", err), ShouldRestart: false})
		return &pb.StartResponse{}, nil
	}

	// the subagent startups
	go runSubagents(pContext, ps.cancelAndSetPluginError, pluginInstallDir, pluginStateDir, runSubAgentCommand, ps.runCommand)
	return &pb.StartResponse{}, nil
}

func newOtelCommand(ctx context.Context, pluginInstallDirectory, pluginStateDirectory string, args ...string) *exec.Cmd {
	cmdArgs := append(args, "--config", "opsagentconf:"+OpsAgentConfigLocationLinux)
	cmd := exec.CommandContext(ctx, path.Join(pluginInstallDirectory, OtelBinary), cmdArgs...)
	cmd.Env = append(os.Environ(),
		"STATE_DIRECTORY="+path.Join(pluginStateDirectory, OtelStateDirectory),
		"LOGS_DIRECTORY="+path.Join(pluginStateDirectory, LogsDirectory),
	)
	return cmd
}

func validateOpsAgentConfig(ctx context.Context, runCommand RunCommandFunc, pluginInstallDirectory string, pluginStateDirectory string) error {
	validateCmd := newOtelCommand(ctx, pluginInstallDirectory, pluginStateDirectory, "validate")
	if output, err := runCommand(validateCmd); err != nil {
		return fmt.Errorf("failed to validate Otel config:\ncommand output: %s\ncommand error: %s", output, err)
	}
	return nil
}

// runSubagents starts the OpenTelemetry Collector subagent.
// When the subagent exits with an error, it won't be restarted, and the parent context is canceled.
// This makes sure that GetStatus() returns a non-healthy status, signaling UAP to Start() the plugin again.
//
// ctx: the parent context for the subagent.
//
// cancelAndSetError: cancels the parent context and records runtime errors from the subagent to be surfaced via GetStatus().
func runSubagents(ctx context.Context, cancelAndSetError CancelContextAndSetPluginErrorFunc, pluginInstallDirectory string, pluginStateDirectory string, runSubAgentCommand RunSubAgentCommandFunc, runCommand RunCommandFunc) {
	// Register signal handler and implements its callback.
	sigHandler(ctx, func(s os.Signal) {
		cancelAndSetError(&OpsAgentPluginError{Message: fmt.Sprintf("Received signal: %s, stopping the Ops Agent", s.String()), ShouldRestart: true})
	})

	runOtelCmd := newOtelCommand(ctx, pluginInstallDirectory, pluginStateDirectory)
	runSubAgentCommand(ctx, cancelAndSetError, runOtelCmd, runCommand)
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
