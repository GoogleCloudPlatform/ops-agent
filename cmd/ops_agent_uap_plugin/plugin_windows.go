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

//go:build windows
// +build windows

package main

import (
	"flag"
	"log"
	"net"
	"os"

	pb "github.com/GoogleCloudPlatform/google-guest-agent/pkg/proto/plugin_comm"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const (
	OpsAgentUAPPluginEventID  uint32 = 8
	WindowsEventLogIdentifier        = "google-cloud-ops-agent-uap-plugin"
)

// eventLogWriter implements the io.Writer interface. It writes logs to the Windows Event Log.
type eventLogWriter struct {
	EventID  uint32
	EventLog *eventlog.Log
}

func (w *eventLogWriter) Write(p []byte) (int, error) {
	err := w.EventLog.Info(w.EventID, string(p))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func createWindowsEventLogger() (debug.Log, error) {
	eventlog.InstallAsEventCreate(WindowsEventLogIdentifier, eventlog.Error|eventlog.Warning|eventlog.Info)
	elog, err := eventlog.Open(WindowsEventLogIdentifier)
	if err != nil {
		return nil, err
	}

	log.SetOutput(&eventLogWriter{
		EventID:  OpsAgentUAPPluginEventID,
		EventLog: elog,
	})
	return elog, nil
}

func main() {
	flag.Parse()

	if _, err := os.Stat(address); err == nil {
		if err := os.RemoveAll(address); err != nil {
			// Unix sockets must be unlinked (listener.Close()) before
			// being reused again. If file already exist bind can fail.
			log.Fatalf("Failed to remove %q: %v\n", address, err)
		}
	}

	logger, err := createWindowsEventLogger()
	if err != nil {
		log.Fatalf("Failed to create windows event logger: %v", err)
	}
	defer logger.Close()

	listener, err := net.Listen(protocol, address)
	if err != nil {
		log.Fatalf("Failed to start listening on %q using %q: %v\n", address, protocol, err)
	}
	log.Printf("Listening on %q using %q\n", address, protocol)
	defer listener.Close()

	// This is the grpc server in communication with the Guest Agent.
	server := grpc.NewServer()
	defer server.GracefulStop()

	ps := &OpsAgentPluginServer{server: server, runCommand: runCommand}
	// Successfully registering the server and starting to listen on the address
	// offered mean Guest Agent was successful in installing/launching the plugin
	// & will manage the lifecycle (start, stop, or revision change) here onwards.
	pb.RegisterGuestAgentPluginServer(server, ps)
	log.Println("Registered plugin server")

	reflection.Register(server)
	log.Println("Registered service reflection service")
	if err := server.Serve(listener); err != nil {
		log.Fatalf("Exiting, cannot continue serving: %v\n", err)
	}
	log.Println("Exiting")
}
