// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"buf.build/go/protoyaml" // Import the protoyaml-go package
	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"

	pb "github.com/GoogleCloudPlatform/ops-agent/cmd/ops_agent_uap_plugin/google_guest_agent/plugin"
	spb "google.golang.org/protobuf/types/known/structpb"
)

func customLogPathByOsType(ctx context.Context) string {
	osType := platform.FromContext(ctx).Name()
	if osType == "linux" {
		return "/var/log"
	}
	return `C:\mylog`
}
func TestWriteCustomConfigToFile(t *testing.T) {
	yamlConfig := fmt.Sprintf(`logging:
  receivers:
    mylog_source:
      type: files
      include_paths:
      - %s
  exporters:
    google:
      type: google_cloud_logging
  processors:
    my_exclude:
      type: exclude_logs
      match_any:
      - jsonPayload.missing_field = "value"
      - jsonPayload.message =~ "test pattern"
  service:
    pipelines:
      my_pipeline:
        receivers: [mylog_source]
        processors: [my_exclude]
        exporters: [google]`, customLogPathByOsType(context.Background()))
	structConfig := &spb.Struct{}
	err := protoyaml.Unmarshal([]byte(yamlConfig), structConfig)
	if err != nil {
		t.Fatalf("Failed to unmarshal YAML into structpb.Struct: %v", err)
	}

	tests := []struct {
		name string
		req  *pb.StartRequest
	}{
		{
			name: "Received a valid StringConfig from UAP, the output should be a valid Ops agent yaml",
			req: &pb.StartRequest{
				ServiceConfig: &pb.StartRequest_StringConfig{
					StringConfig: yamlConfig,
				},
			},
		},
		{
			name: "Received a valid StructConfig from UAP, the output should be a valid Ops agent yaml",
			req: &pb.StartRequest{
				ServiceConfig: &pb.StartRequest_StructConfig{
					StructConfig: structConfig,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temporary directory for the test file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "ops-agent-config", fmt.Sprintf("%sconfig.yaml", tc.name))

			err := writeCustomConfigToFile(tc.req, configPath)

			if err != nil {
				t.Errorf("%v: writeCustomConfigToFile got error: %v, want nil error", tc.name, err)
			}

			_, err = confgenerator.MergeConfFiles(context.Background(), configPath, apps.BuiltInConfStructs)
			if err != nil {
				t.Errorf("%v: conf generator fails to validate the output Ops agent yaml: %v", tc.name, err)
			}
		})
	}
}

func TestWriteCustomConfigToFile_receivedEmptyCustomConfig(t *testing.T) {
	tests := []struct {
		name string
		req  *pb.StartRequest
	}{
		{
			name: "The ops agent config.yaml file should not be modified if UAP does not send any StringConfig",
			req:  &pb.StartRequest{},
		},
		{
			name: "The ops agent config.yaml file should not be modified if UAP does not send any StructConfig",
			req:  &pb.StartRequest{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			configFile, err := os.CreateTemp("", "config.yaml")
			if err != nil {
				t.Fatalf("%v: failed to create the config.yaml file at location: %s, error: %v", tc.name, configFile.Name(), err)
			}
			configPath := configFile.Name()
			wantFileContent := "1234"
			if _, err := configFile.WriteString(wantFileContent); err != nil {
				t.Fatalf("%v: failed to write to the config.yaml file at location: %s, error: %v", tc.name, configPath, err)
			}

			err = writeCustomConfigToFile(tc.req, configPath)
			if err != nil {
				t.Errorf("%v: writeCustomConfigToFile got error: %v, want nil error", tc.name, err)
			}

			gotContent, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("%s: failed to read the config.yaml file content: %v", tc.name, err)
			}
			if string(gotContent) != wantFileContent {
				t.Errorf("%s: got config.yaml content: %v, want: %v", tc.name, string(gotContent), wantFileContent)
			}
			configFile.Close()
			os.Remove(configPath)
		})
	}
}
