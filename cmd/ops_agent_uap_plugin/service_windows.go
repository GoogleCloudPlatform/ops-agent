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
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/windows/svc/mgr"
	"google.golang.org/grpc/status"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	pb "github.com/GoogleCloudPlatform/ops-agent/cmd/ops_agent_uap_plugin/google_guest_agent/plugin"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/kardianos/osext"
)

var (
	AgentWindowsServiceName       = []string{"StackdriverLogging", "StackdriverMonitoring", "google-cloud-ops-agent"}
	DefaultPluginStateDirectory   = filepath.Join(os.Getenv("PROGRAMDATA"), "Google/Compute Engine/google-guest-agent/agent_state/plugins/ops-agent-plugin")
	OpsAgentConfigLocationWindows = filepath.Join(os.Getenv("PROGRAMDATA"), "Google/Cloud Operations/Ops Agent/config/config.yaml")
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
	ps.mu.Lock()
	if ps.cancel != nil {
		log.Printf("The Ops Agent plugin is started already, skipping the current request")
		ps.mu.Unlock()
		return &pb.StartResponse{}, nil
	}
	log.Printf("Received a Start request: %s. Starting the Ops Agent", msg)

	_, cancel := context.WithCancel(context.Background())
	ps.cancel = cancel
	ps.mu.Unlock()

	pluginInstallDir, err := osext.ExecutableFolder()
	if err != nil {
		log.Printf("Start() failed, because it cannot determine the plugin install location: %s", err)
		return nil, status.Error(1, err.Error())
	}

	pluginStateDir := msg.GetConfig().GetStateDirectoryPath()
	if pluginStateDir == "" {
		pluginStateDir = DefaultPluginStateDirectory
	}
	log.Printf("Determined pluginInstallDir: %v, and pluginStateDir: %v", pluginInstallDir, pluginStateDir)

	// Detect conflicting installations
	foundConflictingInstallations, err := findPreExistentAgents(AgentWindowsServiceName)
	if foundConflictingInstallations || err != nil {
		ps.Stop(ctx, &pb.StopRequest{Cleanup: false})
		log.Printf("Start() failed: %s", err)
		return nil, status.Error(1, err.Error())
	}

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

func runCommand(cmd *exec.Cmd) (string, error) {
	panic("runCommand method is not implemented on Windows yet")
}

func generateSubagentConfigs(ctx context.Context) error {
	// TODO(lingshi) Move this to a shared place across Linux and Windows.
	uc, err := confgenerator.MergeConfFiles(ctx, s.userConf, apps.BuiltInConfStructs)
	if err != nil {
		return err
	}

	log.Printf("Built-in config:\n%s", apps.BuiltInConfStructs["windows"])
	log.Printf("Merged config:\n%s", uc)

	// TODO: Add flag for passing in log/run path?
	for _, subagent := range []string{
		"otel",
		"fluentbit",
	} {
		if err := uc.GenerateFilesFromConfig(
			ctx,
			subagent,
			filepath.Join(os.Getenv("PROGRAMDATA"), dataDirectory, "log"),
			filepath.Join(os.Getenv("PROGRAMDATA"), dataDirectory, "run"),
			filepath.Join(s.outDirectory, subagent)); err != nil {
			return err
		}
	}
	return nil
}

func runHealthChecks() error {
	return nil
}

func findPreExistentAgents(agentWindowsServiceNames []string) (bool, error) {
	mgr, err := mgr.Connect()
	if err != nil {
		return false, fmt.Errorf("failed to connect to service manager: %s", err)
	}
	defer mgr.Disconnect()

	installedServices, err := mgr.ListServices()
	if err != nil {
		return false, fmt.Errorf("failed to list installed Windows services: %s", err)
	}

	installedServicesSet := make(map[string]bool)
	for _, s := range installedServices {
		installedServicesSet[s] = true
	}

	agentAlreadyInstalled := false
	alreadyInstalledAgentServiceName := ""
	for _, s := range agentWindowsServiceNames {
		if installedServicesSet[s] {
			agentAlreadyInstalled = true
			alreadyInstalledAgentServiceName = s
			break
		}
	}

	if agentAlreadyInstalled {
		return true, fmt.Errorf("conflicting installations identified: %v", alreadyInstalledAgentServiceName)
	}
	return false, nil
}
