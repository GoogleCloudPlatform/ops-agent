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

package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"buf.build/go/protoyaml"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	pb "github.com/GoogleCloudPlatform/ops-agent/cmd/ops_agent_uap_plugin/google_guest_agent/plugin"
)

var (
	// protocol is the protocol to use tcp/uds.
	protocol string
	// address is the address to start server listening on.
	address string
	// logfile is the path to the log file to capture error logs.
	logfile string
)

// RunCommandFunc defines a function type that takes an exec.Cmd and returns
// its output and error. This abstraction is introduced
// primarily to facilitate testing by allowing the injection of mock
// implementations.
type RunCommandFunc func(cmd *exec.Cmd) (string, error)

// PluginServer implements the plugin RPC server interface.
type OpsAgentPluginServer struct {
	pb.UnimplementedGuestAgentPluginServer
	server *grpc.Server

	// mu protects the cancel field.
	mu     sync.Mutex
	cancel context.CancelFunc

	runCommand RunCommandFunc
}

func init() {
	flag.StringVar(&protocol, "protocol", "", "protocol to use uds/tcp")
	flag.StringVar(&address, "address", "", "address to start server listening on")
	flag.StringVar(&logfile, "errorlogfile", "", "path to the error log file")
}

func main() {
	flag.Parse()

	if _, err := os.Stat(address); err == nil {
		if err := os.RemoveAll(address); err != nil {
			// Unix sockets must be unlinked (listener.Close()) before
			// being reused again. If file already exist bind can fail.
			fmt.Fprintf(os.Stderr, "Failed to remove %q: %v\n", address, err)
			os.Exit(1)
		}
	}

	listener, err := net.Listen(protocol, address)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start listening on %q using %q: %v\n", address, protocol, err)
		os.Exit(1)
	}
	defer listener.Close()

	// This is the grpc server in communication with the Guest Agent.
	server := grpc.NewServer()
	defer server.GracefulStop()

	ps := &OpsAgentPluginServer{server: server, runCommand: runCommand}
	// Successfully registering the server and starting to listen on the address
	// offered mean Guest Agent was successful in installing/launching the plugin
	// & will manage the lifecycle (start, stop, or revision change) here onwards.
	pb.RegisterGuestAgentPluginServer(server, ps)
	reflection.Register(server)
	if err := server.Serve(listener); err != nil {
		fmt.Fprintf(os.Stderr, "Exiting, cannot continue serving: %v\n", err)
		os.Exit(1)
	}
}

func writeCustomConfigToFile(req *pb.StartRequest, configPath string) error {
	customConfig := []byte{}
	switch req.GetServiceConfig().(type) {
	case *pb.StartRequest_StringConfig:
		customConfig = []byte(req.GetStringConfig())
	case *pb.StartRequest_StructConfig:
		structConfig := req.GetStructConfig()
		yamlBytes, err := protoyaml.Marshal(structConfig)
		if err != nil {
			return fmt.Errorf("failed to parse the custom Ops Agent config: %v", err)
		}
		customConfig = yamlBytes
	}

	if len(customConfig) > 0 {
		parentDir := filepath.Dir(configPath)
		if _, err := os.Stat(parentDir); os.IsNotExist(err) {
			err := os.MkdirAll(parentDir, 0755)
			if err != nil {
				return fmt.Errorf("failed to create parent directory %s: %v", parentDir, err)
			}
		}

		file, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("failed to open the config.yaml file at location: %s, error: %v", configPath, err)
		}
		defer file.Close()
		if _, err := file.Write(customConfig); err != nil {
			return fmt.Errorf("failed to write to the config.yaml file at location: %s, error: %v", configPath, err)
		}
	}
	return nil
}
