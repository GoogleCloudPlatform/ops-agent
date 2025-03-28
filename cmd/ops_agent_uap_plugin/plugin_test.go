package main

import (
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	pb "github.com/GoogleCloudPlatform/ops-agent/cmd/ops_agent_uap_plugin/google_guest_agent/plugin"
)

func TestWriteCustomConfigToFile(t *testing.T) {
	tests := []struct {
		name        string
		req         *pb.StartRequest
		wantError   bool
		wantContent string
	}{
		{
			name: "StringConfig Success",
			req: &pb.StartRequest{
				ServiceConfig: &pb.StartRequest_StringConfig{
					StringConfig: "custom_string_config",
				},
			},
			wantContent: "custom_string_config",
		},
		{
			name: "StructConfig Success",
			req: &pb.StartRequest{
				ServiceConfig: &pb.StartRequest_StructConfig{
					StructConfig: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key1": structpb.NewStringValue("value1"),
							"key2": structpb.NewNumberValue(123),
						},
					},
				},
			},
			wantContent: "key1: value1\nkey2: 123\n",
		},
		{
			name: "No ServiceConfig",
			req:  &pb.StartRequest{},
		},
		{
			name: "StructConfig Marshal Error",
			req: &pb.StartRequest{
				ServiceConfig: &pb.StartRequest_StructConfig{
					StructConfig: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"invalid": {Kind: nil}, // Simulate an invalid struct
						},
					},
				},
			},
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temporary directory for the test file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")

			err := writeCustomConfigToFile(tc.req, configPath)

			if (err != nil) != tc.wantError {
				t.Errorf("%v: writeCustomConfigToFile got error: %v, want error: %v", tc.name, err, tc.wantError)
			}

			// Read the content of the created file
			contentBytes, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("%s: failed to read the config.yaml file content: %v", tc.name, err)
			}
			gotContent := string(contentBytes)

			if gotContent != tc.wantContent {
				t.Errorf("%v: writeCustomConfigToFile write content: %s to file\n, want content: %s", tc.name, gotContent, tc.wantContent)
			}
		})
	}
}
