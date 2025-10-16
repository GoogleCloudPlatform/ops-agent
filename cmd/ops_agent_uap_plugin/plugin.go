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
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"buf.build/go/protoyaml"
	pb "github.com/GoogleCloudPlatform/google-guest-agent/pkg/proto/plugin_comm"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
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

// RunSubAgentCommandFunc defines a function type that starts a subagent. If one subagent execution exited, other sugagents are also terminated via context cancellation. This abstraction is introduced
// primarily to facilitate testing by allowing the injection of mock
// implementations.
type RunSubAgentCommandFunc func(ctx context.Context, cancel CancelContextAndSetPluginErrorFunc, cmd *exec.Cmd, runCommand RunCommandFunc, wg *sync.WaitGroup)

// CancelContextAndSetPluginErrorFunc defines a function type that terminates the Ops Agent from running and records the latest error that occurred.
// This abstraction is introduced primarily to facilitate testing by allowing the injection of mock implementations.
type CancelContextAndSetPluginErrorFunc func(err *OpsAgentPluginError)

type OpsAgentPluginError struct {
	Message       string
	ShouldRestart bool
}

// PluginServer implements the plugin RPC server interface.
type OpsAgentPluginServer struct {
	pb.UnimplementedGuestAgentPluginServer
	server *grpc.Server

	// mu protects the cancel and the pluginError field.
	mu          sync.Mutex
	cancel      context.CancelFunc
	pluginError *OpsAgentPluginError

	runCommand RunCommandFunc
}

// Stop is the stop hook and implements any cleanup if required.
// Stop maybe called if plugin revision is being changed.
// For e.g. if plugins want to stop some task it was performing or remove some
// state before exiting it can be done on this request.
func (ps *OpsAgentPluginServer) Stop(ctx context.Context, msg *pb.StopRequest) (*pb.StopResponse, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.pluginError = nil
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
	if ps.cancel != nil {
		log.Println("The Ops Agent plugin is running")
		return &pb.Status{Code: 0, Results: []string{"The Ops Agent Plugin is running ok."}}, nil

	}
	if ps.pluginError != nil {
		log.Printf("The Ops Agent plugin is not running, last error: %s", ps.pluginError.Message)
		if ps.pluginError.ShouldRestart {
			return nil, errors.New(ps.pluginError.Message)
		}
		return &pb.Status{Code: 1, Results: []string{fmt.Sprintf("The Ops Agent Plugin is not running: %s", ps.pluginError.Message)}}, nil
	}
	return &pb.Status{Code: 1, Results: []string{"The Ops Agent Plugin is not running."}}, nil
}

// cancelAndSetPluginError terminates the current attempt of running the Ops Agent and records the latest error that occurred.
func (ps *OpsAgentPluginServer) cancelAndSetPluginError(e *OpsAgentPluginError) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.cancel != nil {
		ps.cancel()
		ps.cancel = nil
	}
	if e != nil {
		ps.pluginError = e
		log.Print(e.Message)
	}
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

func runSubAgentCommand(ctx context.Context, cancelAndSetError CancelContextAndSetPluginErrorFunc, cmd *exec.Cmd, runCommand RunCommandFunc, wg *sync.WaitGroup) {
	defer wg.Done()
	if cmd == nil {
		return
	}
	if ctx.Err() != nil {
		// context has been cancelled
		log.Printf("cannot execute command: %s, because the context has been cancelled", cmd.Args)
		return
	}

	output, err := runCommand(cmd)
	var pluginErr *OpsAgentPluginError
	if err != nil {
		fullErr := fmt.Sprintf("command: %s exited with errors, not restarting.\nCommand output: %s\n Command error:%s", cmd.Args, string(output), err)
		log.Print(fullErr)
		pluginErr = &OpsAgentPluginError{Message: fullErr, ShouldRestart: true}
	} else {
		log.Printf("command: %s %s exited successfully.\nCommand output: %s", cmd.Path, cmd.Args, string(output))
	}
	cancelAndSetError(pluginErr)
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
