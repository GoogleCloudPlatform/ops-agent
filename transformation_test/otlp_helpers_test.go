package transformation_test

import (
	"context"
	"net"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"google.golang.org/grpc"
)

func (transformationConfig transformationTest) generateOTelOTLPExporterConfig(ctx context.Context, t *testing.T, name string, addr string) (string, error) {
	abs, err := filepath.Abs(filepath.Join("testdata", name, transformationInput))
	if err != nil {
		t.Fatal(err)
	}
	pi := transformationConfig.pipelineInstance(abs)
	pi.RID = "my-log-name"
	pi.Backend = confgenerator.BackendOTel

	rps, pls, err := pi.OTelComponents(ctx)
	if err != nil {
		return "", err
	}
	confgenerator.ConvertPipelinesToOtlp(rps)

	return otel.ModularConfig{
		DisableMetrics:    true,
		JSONLogs:          true,
		LogLevel:          "debug",
		ReceiverPipelines: rps,
		Pipelines:         pls,
		Exporters: map[otel.ExporterType]otel.ExporterComponents{
			otel.OTLP_Logs: {
				ProcessorsByType: map[string][]otel.Component{
					// Batch with 1.5s timeout to group in the same log request
					// all late entries flushed from a multiline parser after 1s.
					"logs": {
						otel.GCPProjectID("fake-project"),
						otel.DisableOtlpRoundTrip(),
						otel.PreserveInstrumentationScope(),
						otel.CopyServiceResourceLabels(),
						otel.BatchProcessor(500, 500, "1500ms"),
					},
				},
				Exporter: otel.Component{
					Type: "otlp_grpc",
					Config: map[string]any{
						"endpoint": addr,
						"tls": map[string]any{
							"insecure": true, // We must use insecure TLS because our mock server on localhost does not have certificates installed.
						},
					},
				},
			},
		},
	}.Generate(ctx)
}

type mockOTLPServer struct {
	plogotlp.UnimplementedGRPCServer
	srv       *grpc.Server
	requestCh chan<- plogotlp.ExportRequest
}

func (s *mockOTLPServer) Export(ctx context.Context, request plogotlp.ExportRequest) (plogotlp.ExportResponse, error) {
	s.requestCh <- request
	return plogotlp.NewExportResponse(), nil
}

func (s *mockOTLPServer) GracefulStop() {
	s.srv.GracefulStop()
	close(s.requestCh)
}

func otlpOnGRPCServer(ln net.Listener) (*mockOTLPServer, <-chan plogotlp.ExportRequest) {
	ch := make(chan plogotlp.ExportRequest)
	s := &mockOTLPServer{
		srv:       grpc.NewServer(),
		requestCh: ch,
	}

	plogotlp.RegisterGRPCServer(s.srv, s)
	go func() {
		_ = s.srv.Serve(ln)
	}()

	return s, ch
}

func (transformationConfig transformationTest) runOtelOTLPExporterTest(t *testing.T, name string) {
	got := transformationConfig.runOTelTestInner(t, name, true)
	checkOutput(t, filepath.Join(name, "output_otel_otlpexporter.yaml"), got)
}

func sanitizeOTLPExportRequest(t *testing.T, r plogotlp.ExportRequest, testStartTime time.Time) map[string]any {
	marshaler := &plog.JSONMarshaler{}
	bytes, err := marshaler.MarshalLogs(r.Logs())
	if err != nil {
		t.Logf("failed to marshal request: %v", err)
		return nil
	}

	var req map[string]any
	if err := yaml.Unmarshal(bytes, &req); err != nil {
		t.Log(string(bytes))
		t.Fatal(err)
	}

	sanitizeTimestampAndSort(req, testStartTime)
	coalesceResourceLogs(req)

	return req
}

// coalesceResourceLogs merges multiple ResourceLogs entries that share identical
// resource attributes, and merges ScopeLogs entries that share identical scopes.
//
// In OpenTelemetry Collector's pdata model, logs emitted in separate chunks (for
// example, early lines read immediately vs. trailing multiline entries flushed after
// force_flush_period: 1000ms) enter the pipeline BatchProcessor at different times.
// When BatchProcessor groups incoming chunks into a batch, it appends their
// ResourceLogs slices rather than merging identical resource attribute maps.
//
// On slower CPU runners or emulated architectures (such as ARM64 under high CPU load
// during Debian/RPM package builds), execution timing jitter can cause chunks to arrive
// across multiple cycles or timer ticks. While backend receivers (and the OTLP spec)
// treat multiple ResourceLogs with identical attributes as semantically identical to a
// single ResourceLogs with all log records, transformation tests assert against golden
// YAML files that expect a single canonical ResourceLogs entry.
//
// Coalescing identical resources ensures test assertions are 100% deterministic and
// immune to CPU timing or chunking variations across architectures.
func coalesceResourceLogs(req map[string]any) {
	rls, ok := req["resourceLogs"].([]any)
	if !ok || len(rls) <= 1 {
		return
	}

	mergedRLs := make([]any, 0, len(rls))
	for _, rawRL := range rls {
		rl, ok := rawRL.(map[string]any)
		if !ok {
			mergedRLs = append(mergedRLs, rawRL)
			continue
		}

		res := rl["resource"]
		var targetRL map[string]any
		for _, rawTarget := range mergedRLs {
			target, ok := rawTarget.(map[string]any)
			if ok && cmp.Equal(target["resource"], res) {
				targetRL = target
				break
			}
		}

		if targetRL == nil {
			coalesceScopeLogs(rl)
			mergedRLs = append(mergedRLs, rl)
			continue
		}

		targetSLs, _ := targetRL["scopeLogs"].([]any)
		srcSLs, _ := rl["scopeLogs"].([]any)
		for _, rawSrcSL := range srcSLs {
			srcSL, ok := rawSrcSL.(map[string]any)
			if !ok {
				targetSLs = append(targetSLs, rawSrcSL)
				continue
			}

			srcScope := srcSL["scope"]
			var targetSL map[string]any
			for _, rawExistingSL := range targetSLs {
				existingSL, ok := rawExistingSL.(map[string]any)
				if ok && cmp.Equal(existingSL["scope"], srcScope) {
					targetSL = existingSL
					break
				}
			}

			if targetSL == nil {
				targetSLs = append(targetSLs, srcSL)
			} else {
				targetRecords, _ := targetSL["logRecords"].([]any)
				srcRecords, _ := srcSL["logRecords"].([]any)
				targetSL["logRecords"] = append(targetRecords, srcRecords...)
			}
		}
		targetRL["scopeLogs"] = targetSLs
	}

	req["resourceLogs"] = mergedRLs
}

func coalesceScopeLogs(rl map[string]any) {
	sls, ok := rl["scopeLogs"].([]any)
	if !ok || len(sls) <= 1 {
		return
	}

	mergedSLs := make([]any, 0, len(sls))
	for _, rawSL := range sls {
		sl, ok := rawSL.(map[string]any)
		if !ok {
			mergedSLs = append(mergedSLs, rawSL)
			continue
		}

		scope := sl["scope"]
		var targetSL map[string]any
		for _, rawTarget := range mergedSLs {
			target, ok := rawTarget.(map[string]any)
			if ok && cmp.Equal(target["scope"], scope) {
				targetSL = target
				break
			}
		}

		if targetSL == nil {
			mergedSLs = append(mergedSLs, sl)
		} else {
			targetRecords, _ := targetSL["logRecords"].([]any)
			srcRecords, _ := sl["logRecords"].([]any)
			targetSL["logRecords"] = append(targetRecords, srcRecords...)
		}
	}
	rl["scopeLogs"] = mergedSLs
}

func coalesceOTLPRequests(target, src map[string]any) {
	targetRLs, _ := target["resourceLogs"].([]any)
	srcRLs, _ := src["resourceLogs"].([]any)
	target["resourceLogs"] = append(targetRLs, srcRLs...)
	coalesceResourceLogs(target)
}

func coalesceWriteLogEntriesRequests(target, src map[string]any) {
	targetEntries, _ := target["entries"].([]any)
	srcEntries, _ := src["entries"].([]any)
	target["entries"] = append(targetEntries, srcEntries...)
}

func sanitizeTimestampAndSort(v interface{}, testStartTime time.Time) {
	switch v := v.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if strings.Contains(strings.ToLower(key), "timeunixnano") {
				if timeStr, ok := value.(string); ok {
					if nano, err := strconv.ParseInt(timeStr, 10, 64); err == nil && time.Unix(0, nano).After(testStartTime) {
						v[key] = "now"
					}
				}
			} else {
				sanitizeTimestampAndSort(value, testStartTime)
			}
		}
	case []interface{}:
		sort.SliceStable(v, func(i, j int) bool {
			m1, ok1 := v[i].(map[string]interface{})
			m2, ok2 := v[j].(map[string]interface{})
			if ok1 && ok2 {
				k1, ok1 := m1["key"].(string)
				k2, ok2 := m2["key"].(string)
				if ok1 && ok2 {
					return k1 < k2
				}
			}
			return false
		})
		for _, item := range v {
			sanitizeTimestampAndSort(item, testStartTime)
		}
	}
}

func TestCoalesceResourceLogs(t *testing.T) {
	req := map[string]any{
		"resourceLogs": []any{
			map[string]any{
				"resource": map[string]any{
					"attributes": []any{
						map[string]any{"key": "host.name", "value": "vm1"},
					},
				},
				"scopeLogs": []any{
					map[string]any{
						"scope": map[string]any{"name": "test-scope"},
						"logRecords": []any{
							map[string]any{"body": "line 1"},
							map[string]any{"body": "line 2"},
						},
					},
				},
			},
			map[string]any{
				"resource": map[string]any{
					"attributes": []any{
						map[string]any{"key": "host.name", "value": "vm1"},
					},
				},
				"scopeLogs": []any{
					map[string]any{
						"scope": map[string]any{"name": "test-scope"},
						"logRecords": []any{
							map[string]any{"body": "line 3"},
						},
					},
				},
			},
		},
	}

	coalesceResourceLogs(req)

	rls, ok := req["resourceLogs"].([]any)
	if !ok || len(rls) != 1 {
		t.Fatalf("expected 1 resourceLogs, got %d", len(rls))
	}
	rl := rls[0].(map[string]any)
	sls := rl["scopeLogs"].([]any)
	if len(sls) != 1 {
		t.Fatalf("expected 1 scopeLogs, got %d", len(sls))
	}
	sl := sls[0].(map[string]any)
	records := sl["logRecords"].([]any)
	if len(records) != 3 {
		t.Fatalf("expected 3 logRecords, got %d", len(records))
	}
	for i, want := range []string{"line 1", "line 2", "line 3"} {
		body := records[i].(map[string]any)["body"]
		if body != want {
			t.Errorf("records[%d] = %v, want %s", i, body, want)
		}
	}
}

func TestCoalesceOTLPRequests(t *testing.T) {
	req1 := map[string]any{
		"resourceLogs": []any{
			map[string]any{
				"resource": map[string]any{
					"attributes": []any{
						map[string]any{"key": "host.name", "value": "vm1"},
					},
				},
				"scopeLogs": []any{
					map[string]any{
						"scope": map[string]any{"name": "test-scope"},
						"logRecords": []any{
							map[string]any{"body": "batch 1 - line 1"},
						},
					},
				},
			},
		},
	}
	req2 := map[string]any{
		"resourceLogs": []any{
			map[string]any{
				"resource": map[string]any{
					"attributes": []any{
						map[string]any{"key": "host.name", "value": "vm1"},
					},
				},
				"scopeLogs": []any{
					map[string]any{
						"scope": map[string]any{"name": "test-scope"},
						"logRecords": []any{
							map[string]any{"body": "batch 2 - line 2"},
						},
					},
				},
			},
		},
	}

	coalesceOTLPRequests(req1, req2)

	rls, ok := req1["resourceLogs"].([]any)
	if !ok || len(rls) != 1 {
		t.Fatalf("expected 1 resourceLogs, got %d", len(rls))
	}
	rl := rls[0].(map[string]any)
	sls := rl["scopeLogs"].([]any)
	if len(sls) != 1 {
		t.Fatalf("expected 1 scopeLogs, got %d", len(sls))
	}
	sl := sls[0].(map[string]any)
	records := sl["logRecords"].([]any)
	if len(records) != 2 {
		t.Fatalf("expected 2 logRecords, got %d", len(records))
	}
}

func TestCoalesceWriteLogEntriesRequests(t *testing.T) {
	req1 := map[string]any{
		"entries": []any{
			map[string]any{"textPayload": "line 1"},
		},
	}
	req2 := map[string]any{
		"entries": []any{
			map[string]any{"textPayload": "line 2"},
		},
	}

	coalesceWriteLogEntriesRequests(req1, req2)

	entries, ok := req1["entries"].([]any)
	if !ok || len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}
