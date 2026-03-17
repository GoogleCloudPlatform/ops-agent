// Copyright 2026 Google LLC
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

//go:build integration_test

package ops_agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"strconv"

	loggingapiv2 "cloud.google.com/go/logging/apiv2"
	"cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/api/transport/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/structpb"
)

func createOtlpClient(ctx context.Context) (plogotlp.GRPCClient, error) {
	conn, err := grpc.Dial(ctx,
		option.WithEndpoint("telemetry.googleapis.com:443"),
		option.WithScopes("https://www.googleapis.com/auth/logging.write"),
	)
	if err != nil {
		return nil, err
	}
	return plogotlp.NewGRPCClient(conn), nil
}

func readAndPrepareOtlpLogs(path string, projectID string, logName string) (plog.Logs, int, error) {
	yamlBytes, err := os.ReadFile(path)
	if err != nil {
		return plog.NewLogs(), 0, err
	}

	var reqList []map[string]any
	if err := yaml.Unmarshal(yamlBytes, &reqList); err != nil {
		return plog.NewLogs(), 0, err
	}

	var allResourceLogs []any
	for _, item := range reqList {
		if rl, ok := item["resourceLogs"].([]any); ok {
			allResourceLogs = append(allResourceLogs, rl...)
		}
	}
	standardMap := map[string]any{
		"resourceLogs": allResourceLogs,
	}
	jsonBytes, err := json.Marshal(standardMap)
	if err != nil {
		return plog.NewLogs(), 0, err
	}

	jsonBytes = []byte(strings.ReplaceAll(string(jsonBytes), `"now"`, `"0"`))

	unmarshaler := &plog.JSONUnmarshaler{}
	logs, err := unmarshaler.UnmarshalLogs(jsonBytes)
	if err != nil {
		return plog.NewLogs(), 0, err
	}

	totalCount := 0
	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		rl := logs.ResourceLogs().At(i)
		attrs := rl.Resource().Attributes()
		attrs.PutStr("gcp.project_id", projectID)
		attrs.PutStr("host.id", "1234567890123456789")
		attrs.PutStr("cloud.project", projectID)
		attrs.PutStr("cloud.availability_zone", "us-central1-a")
		attrs.PutStr("cloud.region", "us-central1")
		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)
			for k := 0; k < sl.LogRecords().Len(); k++ {
				lr := sl.LogRecords().At(k)
				now := time.Now()
				lr.SetTimestamp(pcommon.NewTimestampFromTime(now))
				lr.SetObservedTimestamp(pcommon.NewTimestampFromTime(now))
				lr.Attributes().PutStr("gcp.log_name", logName)
				totalCount++
			}
		}
	}

	return logs, totalCount, nil
}

func findMatchingLogs(ctx context.Context, projectID string, logNameRegex string, window time.Duration, query string) ([]*loggingpb.LogEntry, error) {
	client, err := loggingapiv2.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	start := time.Now().Add(-window)
	t := start.Format(time.RFC3339)
	filter := fmt.Sprintf(`logName=~"projects/%s/logs/%s" AND timestamp > "%s"`, projectID, logNameRegex, t)
	if query != "" {
		filter += fmt.Sprintf(` AND %s`, query)
	}

	req := &loggingpb.ListLogEntriesRequest{
		ResourceNames: []string{fmt.Sprintf("projects/%s", projectID)},
		Filter:        filter,
	}
	it := client.ListLogEntries(ctx, req)
	var entries []*loggingpb.LogEntry
	for {
		entry, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func waitForLogs(ctx context.Context, projectID string, logName string, expectedCount int) ([]*loggingpb.LogEntry, error) {
	var actualEntries []*loggingpb.LogEntry
	var err error
	delay := 2 * time.Second
	for attempt := 0; attempt < 20; attempt++ {
		actualEntries, err = findMatchingLogs(ctx, projectID, logName, 10*time.Minute, "")
		if err == nil && len(actualEntries) == expectedCount {
			return actualEntries, nil
		}
		time.Sleep(delay)
		delay *= 2
		if delay > 20*time.Second {
			delay = 20*time.Second
		}
	}
	if err != nil {
		return actualEntries, err
	}
	return actualEntries, fmt.Errorf("timeout waiting for %d logs, found %d", expectedCount, len(actualEntries))
}

func readExpectedEntries(path string) ([]*loggingpb.LogEntry, error) {
	goldenBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var goldenData []map[string]any
	if err := yaml.Unmarshal(goldenBytes, &goldenData); err != nil {
		return nil, err
	}

	var expectedEntries []*loggingpb.LogEntry
	for _, item := range goldenData {
		if es, ok := item["entries"].([]any); ok {
			for _, e := range es {
				if entryMap, ok := e.(map[string]any); ok {
					if ts, ok := entryMap["timestamp"]; ok && ts == "now" {
						delete(entryMap, "timestamp")
					}
				}
			}

			jsonBytes, err := json.Marshal(item)
			if err != nil {
				return nil, err
			}

			req := &loggingpb.WriteLogEntriesRequest{}
			opts := protojson.UnmarshalOptions{DiscardUnknown: true}
			if err := opts.Unmarshal(jsonBytes, req); err != nil {
				return nil, err
			}

			expectedEntries = append(expectedEntries, req.Entries...)
		}
	}
	return expectedEntries, nil
}


// This test compares LogEntries generated by the telemetry.googleapis.com endpoint 
// against those from the Google Cloud exporter, given the same input.
//
// The test reads logs in OTLP format from 'output_otel_otlpexporter.yaml' file under each transformation test case directory, 
// constructs a request to the telemetry.googleapis.com endpoint to send these logs, 
// and then queries Cloud Logging for the resulting LogEntries.
//
// Finally, it compares these results with the expected LogEntries found in 'output_otel.yaml'.
func TestCompareLogEntries(t *testing.T) {
	t.Parallel()
	projectID := os.Getenv("PROJECT")
	if projectID == "" {
		t.Skip("PROJECT environment variable is not set")
	}

	ctx := context.Background()
	client, err := createOtlpClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create OTLP client: %v", err)
	}

	testdataDir := "../../transformation_test/testdata"
	dirs, err := os.ReadDir(testdataDir)
	if err != nil {
		t.Fatalf("Failed to read testdata dir: %v", err)
	}

	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		caseName := dir.Name()
		caseDir := filepath.Join(testdataDir, caseName)

		otlpExporterPath := filepath.Join(caseDir, "output_otel_otlpexporter.yaml")
		outputOtelPath := filepath.Join(caseDir, "output_otel.yaml")

		if _, err := os.Stat(otlpExporterPath); os.IsNotExist(err) {
			continue
		}
		if _, err := os.Stat(outputOtelPath); os.IsNotExist(err) {
			continue
		}

		t.Run(caseName, func(t *testing.T) {
			t.Parallel()
			logName := fmt.Sprintf("transform-test-%d-%s", time.Now().UnixNano(), caseName)

			logs, sentCount, err := readAndPrepareOtlpLogs(otlpExporterPath, projectID, logName)
			if err != nil {
				t.Fatalf("Failed to read and prepare logs: %v", err)
			}

			// Send logs to telemetry.googleapis.com
			req := plogotlp.NewExportRequestFromLogs(logs)
			if _, err := client.Export(ctx, req); err != nil {
				st, _ := status.FromError(err)
				marshaler := &plog.JSONMarshaler{}
				reqBytes, _ := marshaler.MarshalLogs(logs)
				t.Fatalf("Failed to export logs: %v\nDetails: %v\nRequest: %s", err, st.Details(), string(reqBytes))
			}

			expectedEntries, err := readExpectedEntries(outputOtelPath)
			if err != nil {
				t.Fatalf("Failed to read expected entries: %v", err)
			}

			actualEntries, err := waitForLogs(ctx, projectID, logName, sentCount)
			if err != nil {
				t.Fatalf("Failed waiting for logs: %v", err)
			}
			// Compare slices directly
			if diff := cmp.Diff(expectedEntries, actualEntries,
				protocmp.Transform(),
				protocmp.IgnoreFields(&loggingpb.LogEntry{}, "timestamp", "insert_id", "log_name", "receive_timestamp", "resource"),
				protocmp.IgnoreUnknown(),
			); diff != "" {
				t.Errorf("Mismatch in entries (-want +got):\n%s", diff)
			}
		})
	}
}


