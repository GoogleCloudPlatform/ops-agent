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
	"os"
	"path/filepath"
	"testing"

	"buf.build/go/protoyaml" // Import the protoyaml-go package
	"google.golang.org/protobuf/types/known/structpb"

	pb "github.com/GoogleCloudPlatform/ops-agent/cmd/ops_agent_uap_plugin/google_guest_agent/plugin"
)

func TestWriteCustomConfigToFile(t *testing.T) {
	yamlConfig := `logging:
  receivers:
    files_1:
      type: files
      include_paths: ""
      wildcard_refresh_interval: 30s
  processors:
    multiline_parser_1:
      type: parse_multiline
      match_any:
      - type: language_exceptions
        language: go
      - type: language_exceptions
        language: java
      - type: language_exceptions
        language: python
  service:
    pipelines:
      p1:
        receivers: [files_1]
        processors: [multiline_parser_1]`
	structConfig := &structpb.Struct{}
	err := protoyaml.Unmarshal([]byte(yamlConfig), structConfig)
	if err != nil {
		t.Fatalf("Failed to unmarshal YAML into structpb.Struct: %v", err)
	}

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
					StructConfig: structConfig,
				},
			},
			wantContent: yamlConfig,
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
			if len(contentBytes) == 0 {
				t.Errorf("%s: expected content in file but got empty content", tc.name)
			}
		})
	}
}

func TestWriteCustomConfigToFile_receivedEmptyCustomConfig(t *testing.T) {
	tests := []struct {
		name      string
		req       *pb.StartRequest
		wantError bool
	}{
		{
			name: "empty StringConfig",
			req:  &pb.StartRequest{},
		},
		{
			name: "empty StructConfig",
			req:  &pb.StartRequest{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temporary directory for the test file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")

			file, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
			if err != nil {
				t.Fatalf("%v: failed to open the config.yaml file at location: %s, error: %v", tc.name, configPath, err)
			}
			wantFileContent := "1234"
			if _, err := file.WriteString(wantFileContent); err != nil {
				t.Fatalf("%v: failed to write to the config.yaml file at location: %s, error: %v", tc.name, configPath, err)
			}

			err = writeCustomConfigToFile(tc.req, configPath)

			if (err != nil) != tc.wantError {
				t.Errorf("%v: writeCustomConfigToFile got error: %v, want error: %v", tc.name, err, tc.wantError)
			}
			// Read the content of the created file
			gotContent, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("%s: failed to read the config.yaml file content: %v", tc.name, err)
			}
			if string(gotContent) != wantFileContent {
				t.Errorf("%s: got config.yaml content: %v, want: %v", tc.name, string(gotContent), wantFileContent)
			}
		})
	}
}
