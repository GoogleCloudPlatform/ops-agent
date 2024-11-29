package main

import (
	"context"
	"flag"
	"log"
	"time"

	pb "github.com/GoogleCloudPlatform/ops-agent/cmd/ops_agent_uap_wrapper/google_guest_agent/plugin"
	"github.com/GoogleCloudPlatform/ops-agent/internal/logs"
	"google.golang.org/grpc"
)

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

	ps := &OpsAgentPluginServer{server: server, logger: logs.Default()}
	// Successfully registering the server and starting to listen on the address
	// offered mean Guest Agent was successful in installing/launching the plugin
	// & will manage the lifecycle (start, stop, or revision change) here onwards.
	// pb.RegisterGuestAgentPluginServer(server, ps)
	// if err := server.Serve(listener); err != nil {
	// 	fmt.Fprintf(os.Stderr, "Exiting, cannot continue serving: %v\n", err)
	// 	os.Exit(1)
	// }

	ctx := context.Background()
	ps.Start(ctx, &pb.StartRequest{})
	log.Print(ps.GetStatus(ctx, &pb.GetStatusRequest{}))
	time.Sleep(1 * time.Minute)
	log.Print(ps.GetStatus(ctx, &pb.GetStatusRequest{}))
	ps.Stop(ctx, &pb.StopRequest{})
	log.Print(ps.GetStatus(ctx, &pb.GetStatusRequest{}))
}
