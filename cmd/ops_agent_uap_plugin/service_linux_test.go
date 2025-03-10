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
	"sync"
	"syscall"
	"testing"
	"time"

	pb "github.com/GoogleCloudPlatform/ops-agent/cmd/ops_agent_uap_plugin/google_guest_agent/plugin"
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

func TestStart(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name               string
		cancel             context.CancelFunc
		mockRunCommandFunc RunCommandFunc
		wantError          bool
	}{
		{
			name:   "Happy path: plugin not already started, Start() exits successfully",
			cancel: nil,
			mockRunCommandFunc: func(cmd *exec.Cmd) (string, error) {
				return "", nil
			},
			wantError: false,
		},
		{
			name:   "Plugin already started",
			cancel: func() {}, // Non-nil function
			mockRunCommandFunc: func(cmd *exec.Cmd) (string, error) {
				return "", nil
			},
			wantError: false,
		},
		{
			name:   "Start() returns errors, cancel() function should be reset to nil",
			cancel: nil,
			mockRunCommandFunc: func(cmd *exec.Cmd) (string, error) {
				return "", fmt.Errorf("error")
			},
			wantError: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ps := &OpsAgentPluginServer{cancel: tc.cancel, runCommand: tc.mockRunCommandFunc}
			_, err := ps.Start(context.Background(), &pb.StartRequest{})
			gotError := (err != nil)
			if gotError != tc.wantError {
				t.Errorf("%v: Start() got error: %v, err msg: %v, want error:%v", tc.name, gotError, err, tc.wantError)
			}
			if ps.cancel == nil {
				t.Error("got nil cancel function after calling Start(), want non-nil")
			}
		})
	}
}
func TestStop(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		cancel context.CancelFunc
	}{
		{
			name:   "PluginAlreadyStopped",
			cancel: nil,
		},
		{
			name:   "PluginRunning",
			cancel: func() {}, // Non-nil function

		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ps := &OpsAgentPluginServer{cancel: tc.cancel}
			_, err := ps.Stop(context.Background(), &pb.StopRequest{})
			if err != nil {
				t.Errorf("got error from Stop(): %v, wanted nil", err)
			}

			if ps.cancel != nil {
				t.Error("got non-nil cancel function after calling Stop(), want nil")
			}
		})
	}
}

func TestGetStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		cancel         context.CancelFunc
		wantStatusCode int32
	}{
		{
			name:           "PluginNotRunning",
			cancel:         nil,
			wantStatusCode: 1,
		},
		{
			name:           "PluginRunning",
			cancel:         func() {}, // Non-nil function
			wantStatusCode: 0,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ps := &OpsAgentPluginServer{cancel: tc.cancel}
			status, err := ps.GetStatus(context.Background(), &pb.GetStatusRequest{})
			if err != nil {
				t.Errorf("got error from GetStatus: %v, wanted nil", err)
			}
			gotStatusCode := status.Code
			if gotStatusCode != tc.wantStatusCode {
				t.Errorf("Got status code %d from GetStatus(), wanted %d", gotStatusCode, tc.wantStatusCode)
			}

		})
	}
}

func Test_runSubAgentCommand_CancelContextWhenCmdExitsWithErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1", "GO_HELPER_FAILURE=1"}
	var wg sync.WaitGroup
	wg.Add(1)
	runSubAgentCommand(ctx, cancel, cmd, runCommand, &wg)
	if ctx.Err() == nil {
		t.Error("runSubAgentCommand() did not cancel context but should")
	}
}

func Test_runSubAgentCommand_CancelContextWhenCmdExitsSuccessfully(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	var wg sync.WaitGroup
	wg.Add(1)
	runSubAgentCommand(ctx, cancel, cmd, runCommand, &wg)
	if ctx.Err() == nil {
		t.Error("runSubAgentCommand() did not cancel context but should")
	}
}

func Test_runSubAgentCommand_CancelContextWhenCmdTerminatedBySignals(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
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
	runSubAgentCommand(ctx, cancel, cmd, mockRunCommandFunc, &wg)
	if ctx.Err() == nil {
		t.Error("runSubAgentCommand() didn't cancel the context but should")
	}
}

func Test_runSubagents_TerminatesWhenSpawnedGoRoutinesReturn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	mockCmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
	mockCmd.Env = []string{"GO_WANT_HELPER_PROCESS=1", "GO_HELPER_FAILURE=1"}

	mockRestartCommandFunc := func(ctx context.Context, cancel context.CancelFunc, _ *exec.Cmd, runCommand RunCommandFunc, wg *sync.WaitGroup) {
		runSubAgentCommand(ctx, cancel, mockCmd, runCommand, wg)
	}
	cancel() // child go routines return immediately, because the parent context has been cancelled.
	// the test times out and fails if runSubagents does not returns
	runSubagents(ctx, cancel, "", "", mockRestartCommandFunc, runCommand)
}

// TestHelperProcess isn't a real test. It's used as a helper process to mock
// command executions.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		// Skip this test if it's not invoked explicitly as a helper
		// process. return allows the next tests to continue running.
		return
	}
	switch {
	case os.Getenv("GO_HELPER_FAILURE") == "1":
		os.Exit(1)
	case os.Getenv("GO_HELPER_KILL_BY_SIGNALS") == "1":
		time.Sleep(1 * time.Minute)
	default:
		// A "successful" mock execution exits with a successful (zero) exit code.
		os.Exit(0)
	}
}
