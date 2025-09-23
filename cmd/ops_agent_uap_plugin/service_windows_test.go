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

//go:build windows
// +build windows

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	pb "github.com/GoogleCloudPlatform/google-guest-agent/tree/main/pkg/proto/plugin_comm"
)

// serviceManager is a mock implementation of the serviceManager interface. This is used to test the findPreExistentAgents function.
type mockServiceManager struct {
	connectError      error
	listServices      []string
	listServicesError error
}

// serviceManagerConnection is a mock implementation of the serviceManagerConnection interface. This is used to test the findPreExistentAgents function.
type mockServiceManagerConnection struct {
	listServices      []string
	listServicesError error
	disconnectError   error
}

func (m *mockServiceManager) Connect() (serviceManagerConnection, error) {
	return &mockServiceManagerConnection{listServices: m.listServices, listServicesError: m.listServicesError}, m.connectError
}

func (m *mockServiceManagerConnection) ListServices() ([]string, error) {
	return m.listServices, m.listServicesError
}

func (m *mockServiceManagerConnection) Disconnect() error {
	return nil
}

func Test_findPreExistentAgents(t *testing.T) {
	testCases := []struct {
		name                         string
		mockMgr                      *mockServiceManager
		agentWindowsServiceNames     []string
		conflictingInstallationCount int
		wantError                    bool
	}{
		{
			name: "No conflicts",
			mockMgr: &mockServiceManager{
				listServices: []string{"ServiceA", "ServiceB"},
			},
			agentWindowsServiceNames: []string{"ServiceC", "ServiceD"},
		},
		{
			name: "Has conflicting installations",
			mockMgr: &mockServiceManager{
				listServices: []string{"ServiceA", "AgentService"},
			},
			agentWindowsServiceNames:     []string{"AgentService", "ServiceB"},
			conflictingInstallationCount: 1,
			wantError:                    true,
		},
		{
			name: "service manager connection error",
			mockMgr: &mockServiceManager{
				connectError: errors.New("connection failed"),
			},
			agentWindowsServiceNames: []string{"AgentService"},
			wantError:                true,
		},
		{
			name: "list installed windows services error",
			mockMgr: &mockServiceManager{
				listServicesError: errors.New("list services failed"),
			},
			agentWindowsServiceNames: []string{"AgentService"},
			wantError:                true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			preinstalledAgents, gotError := findPreExistentAgents(tc.mockMgr, tc.agentWindowsServiceNames)
			if (gotError != nil) != tc.wantError {
				t.Errorf("%s: findPreExistentAgents() returned error: %v, want error: %v", tc.name, gotError, tc.wantError)
			}
			if len(preinstalledAgents) != tc.conflictingInstallationCount {
				t.Errorf("%s: findPreExistentAgents() found %v conflicting installations:%v, want %v identified", tc.name, len(preinstalledAgents), preinstalledAgents, tc.conflictingInstallationCount)
			}
		})
	}
}

// mockWindowsEventLogger is a mock implementation of the debug.Log interface.
type mockWindowsEventLogger struct{}

func (m *mockWindowsEventLogger) Info(eid uint32, msg string) error {
	return nil
}
func (m *mockWindowsEventLogger) Warning(eid uint32, msg string) error {
	return nil
}
func (m *mockWindowsEventLogger) Error(eid uint32, msg string) error {
	return nil
}
func (m *mockWindowsEventLogger) Close() error {
	return nil
}

// mockHealthCheckLogger is a mock implementation of the logs.StructuredLogger interface.
type mockHealthCheckLogger struct {
	logFile *os.File
}

func writeStringToFile(file *os.File, content string) {
	if _, err := file.Write([]byte(content)); err != nil {
		panic(err)
	}
}
func (m *mockHealthCheckLogger) Infof(format string, v ...interface{}) {
	writeStringToFile(m.logFile, format)
}
func (m *mockHealthCheckLogger) Warnf(format string, v ...interface{}) {
	writeStringToFile(m.logFile, format)
}
func (m *mockHealthCheckLogger) Errorf(format string, v ...interface{}) {
	writeStringToFile(m.logFile, format)
}
func (m *mockHealthCheckLogger) Infow(msg string, keysAndValues ...interface{}) {
	writeStringToFile(m.logFile, msg)
}
func (m *mockHealthCheckLogger) Warnw(msg string, keysAndValues ...interface{}) {
	writeStringToFile(m.logFile, msg)
}
func (m *mockHealthCheckLogger) Errorw(msg string, keysAndValues ...interface{}) {
	writeStringToFile(m.logFile, msg)
}
func (m *mockHealthCheckLogger) Println(v ...interface{}) {
	writeStringToFile(m.logFile, "println")
}

func Test_runHealthChecks_LogFileNonEmpty(t *testing.T) {
	t.Parallel()
	// Create a temporary directory for plugin state
	pluginStateDir := t.TempDir()
	healthCheckLogFile, err := os.CreateTemp(pluginStateDir, "health-checks.log")
	if err != nil {
		t.Fatalf("Failed to create health-checks.log: %v", err)
	}
	defer os.Remove(healthCheckLogFile.Name())
	mockLogger := &mockWindowsEventLogger{}
	mockHealthCheckLogger := &mockHealthCheckLogger{logFile: healthCheckLogFile}

	runHealthChecks(mockHealthCheckLogger, mockLogger)

	// Check if the log file has content
	fileInfo, err := os.Stat(healthCheckLogFile.Name())
	if err != nil {
		t.Fatalf("Failed to get file info: %v", err)
	}
	if fileInfo.Size() == 0 {
		t.Errorf("health-checks.log is empty, wanted non-empty")
	}
	healthCheckLogFile.Close()
}

func Test_generateSubAgentConfigs(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name              string
		userConfigContent string // Content for the user config file
		pluginStateDir    string // Directory for the plugin state
		wantError         bool
	}{
		{
			name:              "happy path: successfully generate sub-agent configs",
			userConfigContent: "",
			pluginStateDir:    t.TempDir(),
		},
		{
			name:              "invalid user config",
			userConfigContent: "invalid content",
			pluginStateDir:    t.TempDir(),
			wantError:         true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			userConfigFile, err := os.CreateTemp(t.TempDir(), "config.yaml")
			if err != nil {
				t.Fatalf("Failed to create temporary user config file: %v", err)
			}
			defer os.Remove(userConfigFile.Name())

			if _, err := userConfigFile.Write([]byte(tc.userConfigContent)); err != nil {
				t.Fatalf("Failed to write user config content: %v", err)
			}
			userConfigFile.Close()
			logger := &mockWindowsEventLogger{}

			err = generateSubAgentConfigs(ctx, userConfigFile.Name(), tc.pluginStateDir, logger)
			if (err != nil) != tc.wantError {
				t.Errorf("generateSubAgentConfigs() returned error: %v, want error: %v", err, tc.wantError)
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
		},
		{
			name:   "Plugin already started",
			cancel: func() {}, // Non-nil function
			mockRunCommandFunc: func(cmd *exec.Cmd) (string, error) {
				return "", nil
			},
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
		})
	}
}

func TestStop(t *testing.T) {
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

func runCommandSuccessfully(_ *exec.Cmd) (string, error) {
	return "success", nil
}
func Test_runSubAgentCommand_CancelContextWhenCmdExitsSuccessfully(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "fake-command")

	var wg sync.WaitGroup
	wg.Add(1)
	runSubAgentCommand(ctx, cancel, cmd, runCommandSuccessfully, &wg)
	if ctx.Err() == nil {
		t.Error("runSubAgentCommand() did not cancel context but should")
	}
}

func runCommandAndFailed(_ *exec.Cmd) (string, error) {
	return "failure", errors.New("command failed")
}
func Test_runSubAgentCommand_CancelContextWhenCmdExitsWithErrors(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "fake-command")

	var wg sync.WaitGroup
	wg.Add(1)
	runSubAgentCommand(ctx, cancel, cmd, runCommandAndFailed, &wg)
	if ctx.Err() == nil {
		t.Error("runSubAgentCommand() did not cancel context but should")
	}
}

func Test_runCommand(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	_, err := runCommand(cmd)
	if err != nil {
		t.Errorf("runCommand got unexpected error: %v", err)
	}
}

func Test_runCommandFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1", "GO_HELPER_FAILURE=1"}

	if _, err := runCommand(cmd); err == nil {
		t.Error("runCommand got nil error, want exec failure")
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
	case os.Getenv("GO_HELPER_KILL_BY_SIGNALS") == "1":
		time.Sleep(1 * time.Minute)
	default:
		// A "successful" mock execution exits with a successful (zero) exit code.
		os.Exit(0)
	}
}
