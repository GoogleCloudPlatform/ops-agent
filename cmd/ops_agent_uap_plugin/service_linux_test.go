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
	"testing"

	pb "github.com/GoogleCloudPlatform/ops-agent/cmd/ops_agent_uap_plugin/google_guest_agent/plugin"
)

func Test_runCommand(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	_, err := runCommand(cmd)
	if err != nil {
		t.Errorf("runCommand got unexpected error: %v", err)
	}
}
func Test_runCommandFailure(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1", "GO_HELPER_FAILURE=1"}
	if _, err := runCommand(cmd); err == nil {
		t.Error("runCommand got nil error, want exec failure")
	}
}

func Test_findPreExistentAgent(t *testing.T) {
	cases := []struct {
		name               string
		mockRunCommandFunc RunCommandFunc
		wantExist          bool
		serviceName        string
	}{
		{
			name: "Found conflicting Ops Agent installation",
			mockRunCommandFunc: func(cmd *exec.Cmd) (string, error) {
				return "Loaded: loaded (/lib/systemd/system/google-cloud-ops-agent.service; enabled; preset: enabled)", nil
			},
			wantExist:   true,
			serviceName: "google-cloud-ops-agent.service",
		},
		{
			name: "Pre-existent Ops Agent not found",
			mockRunCommandFunc: func(cmd *exec.Cmd) (string, error) {
				return "", fmt.Errorf("google-cloud-ops-agent.service could not be found.")
			},
			wantExist:   false,
			serviceName: "google-cloud-ops-agent.service",
		},
		{
			name: "Pre-existent Legacy Monitoring Agent not found",
			mockRunCommandFunc: func(cmd *exec.Cmd) (string, error) {
				return "", fmt.Errorf("stackdriver-agent.service could not be found.")
			},
			wantExist:   false,
			serviceName: "stackdriver-agent.service",
		},
		{
			name: "Pre-existent Legacy Logging Agent not found",
			mockRunCommandFunc: func(cmd *exec.Cmd) (string, error) {
				return "", fmt.Errorf("google-fluentd.service could not be found.")
			},
			wantExist:   false,
			serviceName: "google-fluentd.service",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotExist, _ := findPreExistentAgent(context.Background(), tc.mockRunCommandFunc, tc.serviceName)

			if gotExist != tc.wantExist {
				t.Errorf("%v: findPreExistentAgent(%v) failed to verify if the service exists: gotExist: %v, wantExist %v", tc.name, tc.serviceName, gotExist, tc.wantExist)
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
			err := validateOpsAgentConfig(ctx, mockRunCommand, "")
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
			err := generateSubagentConfigs(ctx, mockRunCommand, "")
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
			if tc.wantError && ps.cancel != nil {
				t.Errorf("%v: Start() did not reset the cancel function to nil", tc.name)
			}
			if !tc.wantError && ps.cancel == nil {
				t.Errorf("%v: Start() reset cancel function to nil but shouldn't", tc.name)
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
	default:
		// A "successful" mock execution exits with a successful (zero) exit code.
		os.Exit(0)
	}
}
