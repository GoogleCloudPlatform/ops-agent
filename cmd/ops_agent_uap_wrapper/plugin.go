package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	pb "github.com/GoogleCloudPlatform/ops-agent/cmd/ops_agent_uap_wrapper/google_guest_agent/plugin"
	"github.com/GoogleCloudPlatform/ops-agent/internal/logs"
	"google.golang.org/grpc"
)

// We are able to compute UAP Plugin State Directory in advance, instead of receiving it through Start().
const OpsAgentUapPluginLog = "ops-agent-uap-plugin.log"
const UapPluginStateDir = "/var/log/google-cloud-ops-agent"

var (
	// protocol is the protocol to use tcp/uds.
	protocol string
	// address is the address to start server listening on.
	address string
	// logfile is the path to the log file to capture error logs.
	logfile string
)

func init() {
	flag.StringVar(&protocol, "protocol", "", "protocol to use uds/tcp")
	flag.StringVar(&address, "address", "", "address to start server listening on")
	flag.StringVar(&logfile, "errorlogfile", "", "path to the error log file")
}

func main() {
	flag.Parse()

	// if _, err := os.Stat(address); err == nil {
	// 	if err := os.RemoveAll(address); err != nil {
	// 		// Unix sockets must be unlinked (listener.Close()) before
	// 		// being reused again. If file already exist bind can fail.
	// 		fmt.Fprintf(os.Stderr, "Failed to remove %q: %v\n", address, err)
	// 		os.Exit(1)
	// 	}
	// }

	// listener, err := net.Listen(protocol, address)
	// if err != nil {
	// 	fmt.Fprintf(os.Stderr, "Failed to start listening on %q using %q: %v\n", address, protocol, err)
	// 	os.Exit(1)
	// }
	// defer listener.Close()

	// This is the grpc server in communication with the Guest Agent.
	server := grpc.NewServer()
	defer server.GracefulStop()

	ps := &OpsAgentPluginServer{server: server, logger: CreateOpsAgentUapPluginLogger(UapPluginStateDir, OpsAgentUapPluginLog)}
	// Successfully registering the server and starting to listen on the address
	// offered mean Guest Agent was successful in installing/launching the plugin
	// & will manage the lifecycle (start, stop, or revision change) here onwards.
	// pb.RegisterGuestAgentPluginServer(server, ps)
	// if err := server.Serve(listener); err != nil {
	// 	fmt.Fprintf(os.Stderr, "Exiting, cannot continue serving: %v\n", err)
	// 	os.Exit(1)
	// }

	ctx := context.Background()
	log.Println("Starting Ops Agent UAP Plugin")
	ps.Start(ctx, &pb.StartRequest{Config: &pb.StartRequest_Config{StateDirectoryPath: "/var/log/google-cloud-ops-agent"}})
	for {
		status, _ := ps.GetStatus(ctx, &pb.GetStatusRequest{})
		log.Print(status)
		if status.Code != 0 {
			break
		}
		time.Sleep(30 * time.Second)
	}
}

func CreateOpsAgentUapPluginLogger(logDir string, fileName string) logs.StructuredLogger {
	// Check if the directory already exists
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		// Directory does not exist, create it
		err := os.Mkdir(logDir, 0755) // 0755 sets permissions (read/write/execute for owner, read/execute for group and others)
		if err != nil {
			log.Printf("failed to create directory for %q: %v", logDir, err)
			logDir = ""
		}
	}

	// Create the log file under the directory
	path := filepath.Join(logDir, fileName)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("failed to open health checks log file %q: %v", path, err)
		return logs.Default()
	}
	file.Close()

	return logs.New(path)
}
