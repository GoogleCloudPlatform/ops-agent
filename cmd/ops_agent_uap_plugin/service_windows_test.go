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
	"testing"

	pb "github.com/GoogleCloudPlatform/ops-agent/cmd/ops_agent_uap_plugin/google_guest_agent/plugin"
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

func TestFindPreExistentAgents(t *testing.T) {
	testCases := []struct {
		name                     string
		mockMgr                  *mockServiceManager
		agentWindowsServiceNames []string
		wantFoundConflicts       bool
		wantError                bool
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
			agentWindowsServiceNames: []string{"AgentService", "ServiceB"},
			wantFoundConflicts:       true,
		},
		{
			name: "service manager connection error",
			mockMgr: &mockServiceManager{
				connectError: errors.New("connection failed"),
			},
			agentWindowsServiceNames: []string{"AgentService"},
			wantError:                true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotFoundConflicts, gotError := findPreExistentAgents(tc.mockMgr, tc.agentWindowsServiceNames)
			if (gotError != nil) != tc.wantError {
				t.Errorf("%s: findPreExistentAgents() returned error: %v, want error: %v", tc.name, gotError, tc.wantError)
			}
			if gotFoundConflicts != tc.wantFoundConflicts {
				t.Errorf("%s: findPreExistentAgents() found conflicting installations:%v, want %v", tc.name, gotFoundConflicts, tc.wantFoundConflicts)
			}
		})
	}
}

func TestStart(t *testing.T) {
	cases := []struct {
		name      string
		cancel    context.CancelFunc
		wantError bool
	}{
		{
			name:      "Plugin already started",
			cancel:    func() {}, // Non-nil function
			wantError: false,
		},
		{
			name:      "Start() returns errors, cancel() function should be reset to nil",
			cancel:    nil,
			wantError: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ps := &OpsAgentPluginServer{cancel: tc.cancel}
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
