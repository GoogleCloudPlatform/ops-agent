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

	loggingapiv2 "cloud.google.com/go/logging/apiv2"
	"cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/goccy/go-yaml"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/api/transport/grpc"
	"google.golang.org/grpc/status"
	"github.com/google/go-cmp/cmp"
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

func entryToMap(entry *loggingpb.LogEntry) map[string]any {
	m := map[string]any{}
	if entry.HttpRequest != nil {
		req := entry.HttpRequest
		reqMap := map[string]any{}
		if req.Protocol != "" { reqMap["protocol"] = req.Protocol }
		if req.RemoteIp != "" { reqMap["remoteIp"] = req.RemoteIp }
		if req.RequestMethod != "" { reqMap["requestMethod"] = req.RequestMethod }
		if req.RequestUrl != "" { reqMap["requestUrl"] = req.RequestUrl }
		if req.ResponseSize != 0 { reqMap["responseSize"] = req.ResponseSize }
		if req.RequestSize != 0 { reqMap["requestSize"] = req.RequestSize }
		if req.Status != 0 { reqMap["status"] = uint64(req.Status) }
		if req.UserAgent != "" { reqMap["userAgent"] = req.UserAgent }
		if req.Referer != "" { reqMap["referer"] = req.Referer }
		if req.ServerIp != "" { reqMap["serverIp"] = req.ServerIp }
		if req.CacheLookup { reqMap["cacheLookup"] = req.CacheLookup }
		if req.CacheHit { reqMap["cacheHit"] = req.CacheHit }
		if req.CacheValidatedWithOriginServer { reqMap["cacheValidatedWithOriginServer"] = req.CacheValidatedWithOriginServer }
		if req.CacheFillBytes != 0 { reqMap["cacheFillBytes"] = req.CacheFillBytes }
		if req.Latency != nil { reqMap["latency"] = req.Latency.AsDuration().String() }
		m["httpRequest"] = reqMap
	}
	if entry.GetJsonPayload() != nil {
		m["jsonPayload"] = entry.GetJsonPayload().AsMap()
	} else if entry.GetTextPayload() != "" {
		m["textPayload"] = entry.GetTextPayload()
	}
	if len(entry.Labels) > 0 {
		m["labels"] = entry.Labels
	}
	if entry.LogName != "" {
		m["logName"] = entry.LogName
	}
	if entry.Resource != nil {
		labels := map[string]any{}
		for k, v := range entry.Resource.Labels {
			labels[k] = v
		}
		m["resource"] = map[string]any{
			"type":   entry.Resource.Type,
			"labels": labels,
		}
	}
	if entry.Trace != "" {
		m["trace"] = entry.Trace
	}
	if entry.SpanId != "" {
		m["spanId"] = entry.SpanId
	}
	if entry.SourceLocation != nil {
		slMap := map[string]any{}
		if entry.SourceLocation.File != "" { slMap["file"] = entry.SourceLocation.File }
		if entry.SourceLocation.Line != 0 { slMap["line"] = entry.SourceLocation.Line }
		if entry.SourceLocation.Function != "" { slMap["function"] = entry.SourceLocation.Function }
		m["sourceLocation"] = slMap
	}
	if entry.Split != nil {
		splitMap := map[string]any{}
		if entry.Split.Uid != "" { splitMap["uid"] = entry.Split.Uid }
		if entry.Split.Index != 0 { splitMap["index"] = entry.Split.Index }
		if entry.Split.TotalSplits != 0 { splitMap["totalSplits"] = entry.Split.TotalSplits }
		m["split"] = splitMap
	}
	if entry.Severity != 0 {
		m["severity"] = strings.ToUpper(entry.Severity.String())
	}
	return m
}

func normalizeMapForComparison(m map[string]any) {
	delete(m, "insertId")
	delete(m, "timestamp")
	delete(m, "receivedTimestamp")
	delete(m, "logName")
	if res, ok := m["resource"].(map[string]any); ok {
		if labels, ok := res["labels"].(map[string]any); ok {
			labels["instance_id"] = ""
			labels["zone"] = ""
			labels["project_id"] = ""
		}
	}
	if labels, ok := m["labels"].(map[string]any); ok {
		strLabels := map[string]string{}
		for k, v := range labels {
			strLabels[k] = fmt.Sprintf("%v", v)
		}
		m["labels"] = strLabels
	}
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

func readExpectedEntries(path string) ([]any, error) {
	goldenBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var goldenData []map[string]any
	if err := yaml.Unmarshal(goldenBytes, &goldenData); err != nil {
		return nil, err
	}

	var expectedEntries []any
	for _, item := range goldenData {
		if es, ok := item["entries"].([]any); ok {
			expectedEntries = append(expectedEntries, es...)
		}
		if errs, ok := item["collector_errors"].([]any); ok {
			expectedEntries = append(expectedEntries, errs...)
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

			// Compare
			for idx, actualEntry := range actualEntries {
				actualMap := entryToMap(actualEntry)
				expectedMap := expectedEntries[idx].(map[string]any)

				normalizeMapForComparison(actualMap)
				normalizeMapForComparison(expectedMap)

				if diff := cmp.Diff(expectedMap, actualMap); diff != "" {
					t.Errorf("Mismatch in entry %d (-want +got):\n%s", idx, diff)
				}
			}
		})
	}
}
