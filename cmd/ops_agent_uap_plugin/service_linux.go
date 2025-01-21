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
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc/status"

	pb "github.com/GoogleCloudPlatform/ops-agent/cmd/ops_agent_uap_plugin/google_guest_agent/plugin"
)

const (
	OpsAgentConfigLocationLinux = "/etc/google-cloud-ops-agent/config.yaml"
	ConfGeneratorBinary         = "libexec/google_cloud_ops_agent_engine"
	DiagnosticsBinary           = "libexec/google_cloud_ops_agent_diagnostics"
	AgentWrapperBinary          = "libexec/google_cloud_ops_agent_wrapper"
	FluentbitBinary             = "subagents/fluent-bit/bin/fluent-bit"
	OtelBinary                  = "subagents/opentelemetry-collector/otelopscol"

	LogsDirectory               = "log/google-cloud-ops-agent"
	FluentBitStateDiectory      = "state/fluent-bit"
	FluentBitRuntimeDirectory   = "run/google-cloud-ops-agent-fluent-bit"
	OtelRuntimeDirectory        = "run/google-cloud-ops-agent-opentelemetry-collector"
	DefaultPluginStateDirectory = "/var/lib/google-guest-agent/plugins/ops-agent-plugin"
	RestartLimit                = 3 // if a command crashes more than `restartLimit` times consecutively, the command will no longer be restarted.
)

var (
	AgentServiceNameRegex    = regexp.MustCompile(`[\w-]+\.service`)
	AgentSystemdServiceNames = []string{"google-cloud-ops-agent.service", "stackdriver-agent.service", "google-fluentd.service"}
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

	// Find existing ops agent installation, and conflicting legacy agent installation.
	foundConflictingInstallations, err := findPreExistentAgents(pContext, ps.runCommand, AgentSystemdServiceNames)
	if foundConflictingInstallations || err != nil {
		ps.cancel()
		ps.cancel = nil
		log.Printf("Start() failed: %s", err)
		return nil, status.Error(1, err.Error())
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

	// Trigger subagent startups
	go runSubagents(pContext, cancel, pluginStateDir, ps.runCommand)
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

func runSubagents(ctx context.Context, cancel context.CancelFunc, pluginBaseLocation string, runCommand RunCommandFunc) {
	// Register signal handler and implements its callback.
	sigHandler(ctx, func(_ os.Signal) {
		// We're handling some external signal here, set cleanup to [false].
		// If this was Guest Agent trying to stop it would call [Stop] RPC directly
		// or do a [SIGKILL] which anyways cannot be intercepted.
		cancel()
	})

	var wg sync.WaitGroup
	// Starting the diagnostics service
	runDiagnosticsCmd := exec.CommandContext(ctx,
		path.Join(pluginBaseLocation, DiagnosticsBinary),
		"-config", OpsAgentConfigLocationLinux,
	)
	wg.Add(1)
	go restartCommand(ctx, cancel, runDiagnosticsCmd, runCommand, RestartLimit, RestartLimit, &wg)

	// Starting Otel
	runOtelCmd := exec.CommandContext(ctx,
		path.Join(pluginBaseLocation, OtelBinary),
		"--config", path.Join(pluginBaseLocation, OtelRuntimeDirectory, "otel.yaml"),
	)
	wg.Add(1)
	go restartCommand(ctx, cancel, runOtelCmd, runCommand, RestartLimit, RestartLimit, &wg)

	// Starting FluentBit
	runFluentBitCmd := exec.CommandContext(ctx,
		path.Join(pluginBaseLocation, AgentWrapperBinary),
		"-config_path", OpsAgentConfigLocationLinux,
		"-log_path", path.Join(pluginBaseLocation, LogsDirectory, "subagents/logging-module.log"),
		path.Join(pluginBaseLocation, FluentbitBinary),
		"--config", path.Join(pluginBaseLocation, FluentBitRuntimeDirectory, "fluent_bit_main.conf"),
		"--parser", path.Join(pluginBaseLocation, FluentBitRuntimeDirectory, "fluent_bit_parser.conf"),
		"--storage_path", path.Join(pluginBaseLocation, FluentBitStateDiectory, "buffers"),
	)
	wg.Add(1)
	go restartCommand(ctx, cancel, runFluentBitCmd, runCommand, RestartLimit, RestartLimit, &wg)

	wg.Wait()
}

func restartCommand(ctx context.Context, cancel context.CancelFunc, cmd *exec.Cmd, runCommand RunCommandFunc, remainingRetry int, totalRetry int, wg *sync.WaitGroup) {
	defer wg.Done()
	if cmd == nil {
		return
	}
	if ctx.Err() != nil {
		// context has been cancelled
		log.Printf("cannot execute command: %s, because the context has been cancelled", cmd.Args)
		return
	}
	if remainingRetry == 0 {
		log.Printf("out of retries, command: %s is not restarted", cmd.Args)
		cancel()
		return
	}

	childCtx, childCtxCancel := context.WithCancel(ctx)
	defer childCtxCancel()
	retryCount := remainingRetry
	cmd = exec.CommandContext(childCtx, cmd.Path, cmd.Args[1:]...)
	output, err := runCommand(cmd)
	if err != nil {
		// https://pkg.go.dev/os#ProcessState.ExitCode Don't restart if the command was terminated by signals.
		if exiterr, ok := err.(*exec.ExitError); ok && exiterr.ProcessState.ExitCode() == -1 {
			log.Printf("command: %s terminated by signals, not restarting.\nCommand output: %s\n Command error:%s", cmd.Args, string(output), err)
			return
		}
		retryCount -= 1

	} else {
		log.Printf("command: %s exited successfully.\nCommand output: %s", cmd.Args, string(output))
		retryCount = totalRetry
	}
	// Sleep 5 seconds before retarting the task
	time.Sleep(5 * time.Second)
	wg.Add(1)
	go restartCommand(ctx, cancel, cmd, runCommand, retryCount, totalRetry, wg)
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
		return string(out), err
	}
	return string(out), nil
}

func validateOpsAgentConfig(ctx context.Context, runCommand RunCommandFunc, pluginBaseLocation string) error {
	configValidationCmd := exec.CommandContext(ctx,
		path.Join(pluginBaseLocation, ConfGeneratorBinary),
		"-in", OpsAgentConfigLocationLinux,
	)
	if output, err := runCommand(configValidationCmd); err != nil {
		return fmt.Errorf("failed to validate the Ops Agent config:\ncommand output: %s\ncommand error: %s", output, err)
	}
	return nil
}

func generateSubagentConfigs(ctx context.Context, runCommand RunCommandFunc, pluginBaseLocation string) error {
	confGeneratorBinaryFullPath := path.Join(pluginBaseLocation, ConfGeneratorBinary)
	otelConfigGenerationCmd := exec.CommandContext(ctx,
		confGeneratorBinaryFullPath,
		"-service", "otel",
		"-in", OpsAgentConfigLocationLinux,
		"-out", path.Join(pluginBaseLocation, OtelRuntimeDirectory),
		"-logs", path.Join(pluginBaseLocation, LogsDirectory))

	if output, err := runCommand(otelConfigGenerationCmd); err != nil {
		return fmt.Errorf("failed to generate Otel config:\ncommand output: %s\ncommand error: %s", output, err)
	}

	fluentBitConfigGenerationCmd := exec.CommandContext(ctx,
		confGeneratorBinaryFullPath,
		"-service", "fluentbit",
		"-in", OpsAgentConfigLocationLinux,
		"-out", path.Join(pluginBaseLocation, FluentBitRuntimeDirectory),
		"-logs", path.Join(pluginBaseLocation, LogsDirectory), "-state", path.Join(pluginBaseLocation, FluentBitStateDiectory))

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
