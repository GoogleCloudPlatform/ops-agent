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

//go:build !windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	pb "github.com/GoogleCloudPlatform/google-guest-agent/pkg/proto/plugin_comm"
)

func Test_runCommand(t *testing.T) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	_, err := runCommand(cmd)
	if err != nil {
		t.Errorf("runCommand got unexpected error: %v", err)
	}
}
func Test_runCommandFailure(t *testing.T) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1", "GO_HELPER_FAILURE=1"}
	if _, err := runCommand(cmd); err == nil {
		t.Error("runCommand got nil error, want exec failure")
	}
}

func Test_findPreExistentAgents(t *testing.T) {
	cases := []struct {
		name               string
		mockRunCommandFunc RunCommandFunc
		wantExist          bool
	}{
		{
			name: "Found conflicting agent installations",
			mockRunCommandFunc: func(cmd *exec.Cmd) (string, error) {
				return `UNIT FILE                     STATE    PRESET 
						google-cloud-ops-agent.service disabled enabled
						stackdriver-agent.service      masked   enabled
						
						2 unit files listed.`, nil
			},
			wantExist: true,
		},
		{
			name: "Pre-existent agent installation not found",
			mockRunCommandFunc: func(cmd *exec.Cmd) (string, error) {
				return `UNIT FILE STATE PRESET

						0 unit files listed.`, fmt.Errorf("exit status 1")
			},
			wantExist: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotExist, err := findPreExistentAgents(context.Background(), tc.mockRunCommandFunc, AgentSystemdServiceNames)

			if gotExist != tc.wantExist {
				t.Errorf("%v: findPreExistentAgents() failed to verify conflicting agent installations: gotExist: %v, wantExist %v, err: %v", tc.name, gotExist, tc.wantExist, err)
			}
		})
	}
}

func Test_validateOpsAgentConfig(t *testing.T) {
	cases := []struct {
		name          string
		mockCmdOutput string
		mockCmdErr    error
		wantSuccess   bool
	}{
		{
			name:          "config validation successful",
			mockCmdOutput: "",
			mockCmdErr:    nil,
			wantSuccess:   true,
		},
		{
			name:          "config validation failed",
			mockCmdOutput: "",
			mockCmdErr:    fmt.Errorf("error"),
			wantSuccess:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock RunCommand function
			mockRunCommand := func(cmd *exec.Cmd) (string, error) {
				return tc.mockCmdOutput, tc.mockCmdErr
			}

			ctx := context.Background()
			err := validateOpsAgentConfig(ctx, "", "", mockRunCommand)
			gotSuccess := (err == nil)
			if gotSuccess != tc.wantSuccess {
				t.Errorf("%s: validateOpsAgentConfig() failed to valide Ops Agent config: %v, want successful config validation: %v, error:%v", tc.name, gotSuccess, tc.wantSuccess, err)
			}
		})
	}
}

func Test_generateSubagentConfigs(t *testing.T) {
	cases := []struct {
		name          string
		mockCmdOutput string
		mockCmdErr    error
		wantSuccess   bool
	}{
		{
			name:          "configs generation successful",
			mockCmdOutput: "",
			mockCmdErr:    nil,
			wantSuccess:   true,
		},
		{
			name:          "configs generation failed",
			mockCmdOutput: "",
			mockCmdErr:    fmt.Errorf("error"),
			wantSuccess:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock RunCommand function
			mockRunCommand := func(cmd *exec.Cmd) (string, error) {
				return tc.mockCmdOutput, tc.mockCmdErr
			}

			ctx := context.Background()
			err := generateSubagentConfigs(ctx, mockRunCommand, "", "")
			gotSuccess := (err == nil)
			if gotSuccess != tc.wantSuccess {
				t.Errorf("%s: generateSubagentConfigs() failed to generate subagents configs: %v, want successful config validation: %v, error:%v", tc.name, gotSuccess, tc.wantSuccess, err)
			}
		})
	}
}

func mockRunCommandSuccess(cmd *exec.Cmd) (string, error) {
	switch {
	case strings.HasSuffix(cmd.Path, "systemctl"):
		return "0 unit files listed.", nil
	case strings.HasSuffix(cmd.Path, "google_cloud_ops_agent_engine"):
		return "", nil
	default:
		time.Sleep(2 * time.Minute) // Simulate subagent running.
		return "", nil
	}
}

func TestStart_subagentsRunning(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name               string
		cancel             context.CancelFunc
		mockRunCommandFunc RunCommandFunc
	}{
		{
			name:               "Happy path: Start() starts the plugin successfully when plugin is not already started, sub-agent processes are running",
			mockRunCommandFunc: mockRunCommandSuccess,
		},
		{
			name:               "Plugin already started, sub-agent processes are running",
			cancel:             func() {}, // Non-nil function
			mockRunCommandFunc: mockRunCommandSuccess,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ps := &OpsAgentPluginServer{cancel: tc.cancel, runCommand: tc.mockRunCommandFunc}
			ps.Start(context.Background(), &pb.StartRequest{})
			time.Sleep(2 * time.Second)
			ps.mu.Lock()
			defer ps.mu.Unlock()
			if ps.cancel == nil {
				t.Errorf("%v: got nil cancel function, want non-nil", tc.name)
			}
			if ps.pluginError != nil {
				t.Errorf("%v: got pluginError: %v, want nil", tc.name, ps.pluginError)
			}
		})
	}
}

func mockRunCommandFailure(cmd *exec.Cmd) (string, error) {
	switch {
	case strings.HasSuffix(cmd.Path, "systemctl"):
		return "0 unit files listed.", nil
	case strings.HasSuffix(cmd.Path, "google_cloud_ops_agent_engine"):
		return "", nil
	default:
		return "", fmt.Errorf("error") // Simulate subagent process exiting with error.
	}
}

func TestStart_subagentsExitedWithError(t *testing.T) {
	t.Parallel()
	ps := &OpsAgentPluginServer{runCommand: mockRunCommandFailure}
	ps.Start(context.Background(), &pb.StartRequest{})
	time.Sleep(2 * time.Second)
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.cancel != nil {
		t.Errorf("got cancel function: %v, want cancel function to be reset to nil, because sub-agent processes exited with errors", ps.cancel)
	}
	if ps.pluginError == nil {
		t.Error("got nil pluginError, want pluginError to be set, because sub-agent processes exited with errors")
	}
}

func TestStart_subagentsExitedSuccessfully(t *testing.T) {
	t.Parallel()
	ps := &OpsAgentPluginServer{runCommand: func(cmd *exec.Cmd) (string, error) {
		return "", nil // Simulate subagent process exiting successfully.
	}}

	ps.Start(context.Background(), &pb.StartRequest{})
	time.Sleep(2 * time.Second)
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.cancel != nil {
		t.Errorf("got cancel function: %v, want cancel function to be reset to nil, because sub-agent processes exited successfully", ps.cancel)
	}
	if ps.pluginError != nil {
		t.Errorf("got pluginError: %v, want nil pluginError, because sub-agent processes exited successfully", ps.pluginError)
	}
}

func Test_runSubAgentCommand_CancelContextAndSetPluginErrorWhenCmdExitsWithErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1", "GO_HELPER_FAILURE=1"}
	var wg sync.WaitGroup
	wg.Add(1)

	pluginServer := &OpsAgentPluginServer{}
	pluginServer.cancel = cancel

	runSubAgentCommand(ctx, pluginServer.cancelAndSetPluginError, cmd, runCommand, &wg)
	if ctx.Err() != context.Canceled {
		t.Error("runSubAgentCommand() did not cancel context but should")
	}
	if pluginServer.pluginError == nil {
		t.Error("runSubAgentCommand() did not set pluginError but should")
	}
	if !pluginServer.pluginError.ShouldRestart {
		t.Error("runSubAgentCommand() set pluginError.ShouldRestart to false, want true")
	}
}

func Test_runSubAgentCommand_CancelContextWhenCmdExitsSuccessfully(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pluginServer := &OpsAgentPluginServer{}
	pluginServer.cancel = cancel

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	var wg sync.WaitGroup
	wg.Add(1)

	runSubAgentCommand(ctx, pluginServer.cancelAndSetPluginError, cmd, runCommand, &wg)
	if ctx.Err() != context.Canceled {
		t.Error("runSubAgentCommand() did not cancel context but should")
	}
	if pluginServer.pluginError != nil {
		t.Errorf("runSubAgentCommand() set pluginError: %v, want nil", pluginServer.pluginError)
	}
}

func Test_runSubAgentCommand_CancelContextWhenCmdTerminatedBySignals(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pluginServer := &OpsAgentPluginServer{}
	pluginServer.cancel = cancel

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1", "GO_HELPER_KILL_BY_SIGNALS=1"}
	var wg sync.WaitGroup
	wg.Add(1)

	mockRunCommandFunc := func(cmd *exec.Cmd) (string, error) {
		if err := cmd.Start(); err != nil {
			t.Errorf("the command %s did not start successfully", cmd.Args)
		}
		cmd.Process.Signal(syscall.SIGABRT)
		err := cmd.Wait()
		return "", err
	}

	runSubAgentCommand(ctx, pluginServer.cancelAndSetPluginError, cmd, mockRunCommandFunc, &wg)
	if ctx.Err() != context.Canceled {
		t.Error("runSubAgentCommand() didn't cancel the context but should")
	}
	if pluginServer.pluginError == nil {
		t.Errorf("runSubAgentCommand() did not set pluginError but should")
	}
	if !pluginServer.pluginError.ShouldRestart {
		t.Error("runSubAgentCommand() set pluginError.ShouldRestart to false, want true")
	}
}

func Test_runSubAgentCommand_WhenCmdExitsBecauseCtxIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pluginServer := &OpsAgentPluginServer{}
	pluginServer.cancel = cancel

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	var wg sync.WaitGroup
	wg.Add(1)

	runSubAgentCommand(ctx, pluginServer.cancelAndSetPluginError, cmd, runCommand, &wg)
	time.Sleep(3 * time.Second)
	cancel()

	if ctx.Err() != context.Canceled {
		t.Error("runSubAgentCommand() didn't cancel the context but should")
	}
	if pluginServer.pluginError != nil {
		t.Errorf("runSubAgentCommand() set pluginError %v, want nil", pluginServer.pluginError)
	}
}

func Test_runSubagents_TerminatesWhenSpawnedGoRoutinesReturn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pluginServer := &OpsAgentPluginServer{}
	pluginServer.cancel = cancel

	mockCmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
	mockCmd.Env = []string{"GO_WANT_HELPER_PROCESS=1", "GO_HELPER_FAILURE=1"}

	mockRunSubAgentCmd := func(ctx context.Context, cancel CancelContextAndSetPluginErrorFunc, _ *exec.Cmd, runCommand RunCommandFunc, wg *sync.WaitGroup) {
		runSubAgentCommand(ctx, cancel, mockCmd, runCommand, wg)
	}
	runSubagents(ctx, pluginServer.cancelAndSetPluginError, "", "", mockRunSubAgentCmd, runCommand)
}
