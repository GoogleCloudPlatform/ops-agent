package main

import (
	"context"
	"log"

	"google.golang.org/grpc"

	pb "github.com/GoogleCloudPlatform/ops-agent/cmd/ops_agent_uap_plugin/google_guest_agent/plugin"
)

// PluginServer implements the plugin RPC server interface.
type OpsAgentPluginServer struct {
	pb.UnimplementedGuestAgentPluginServer
	server *grpc.Server
	// cancel is the cancel function to be called when core plugin is stopped.
	cancel context.CancelFunc
}

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
	if ps.cancel != nil {
		log.Printf("The Ops Agent plugin is started already, skipping the current request")
		return &pb.StartResponse{}, nil
	}
	log.Printf("Received a Start request: %s. Starting the Ops Agent", msg)

	_, cancel := context.WithCancel(context.Background())
	ps.cancel = cancel
	return &pb.StartResponse{}, nil
}

// Stop is the stop hook and implements any cleanup if required.
// Stop maybe called if plugin revision is being changed.
// For e.g. if plugins want to stop some task it was performing or remove some
// state before exiting it can be done on this request.
func (ps *OpsAgentPluginServer) Stop(ctx context.Context, msg *pb.StopRequest) (*pb.StopResponse, error) {
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
	if ps.cancel == nil {
		log.Println("The Ops Agent plugin is not running")
		return &pb.Status{Code: 1, Results: []string{"The Ops Agent Plugin is not running."}}, nil
	}
	log.Println("The Ops Agent plugin is running")
	return &pb.Status{Code: 0, Results: []string{"The Ops Agent Plugin is running ok."}}, nil
}
