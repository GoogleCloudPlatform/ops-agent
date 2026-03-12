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

	ctx = confgenerator.ContextWithExperiments(ctx, map[string]bool{"otlp_exporter": true})

	rps, pls, err := pi.OTelComponents(ctx)
	if err != nil {
		return "", err
	}

	return otel.ModularConfig{
		DisableMetrics:    true,
		JSONLogs:          true,
		LogLevel:          "debug",
		ReceiverPipelines: rps,
		Pipelines:         pls,
		Exporters: map[otel.ExporterType]otel.ExporterComponents{
			otel.OTLP: {
				ProcessorsByType: map[string][]otel.Component{
					// Batch with 1.5s timeout to group in the same log request
					// all late entries flushed from a multiline parser after 1s.
					"logs": {
						otel.GCPProjectID("fake-project"),
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

	return req
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
