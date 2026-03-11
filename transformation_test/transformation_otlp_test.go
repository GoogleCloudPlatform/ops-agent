package transformation_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
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
	"golang.org/x/sync/errgroup"
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
	got := transformationConfig.runOtelOTLPExporterTestInner(t, name)
	checkOutput(t, filepath.Join(name, "output_otel_otlpexporter.yaml"), got)
}

func (transformationConfig transformationTest) runOtelOTLPExporterTestInner(t *testing.T, name string) []map[string]any {
	ctx, cancel := context.WithCancel(testContext())
	defer cancel()

	// Start an OTLP-compatible receiver.
	ln, err := net.Listen("tcp", "localhost:")
	if err != nil {
		t.Fatalf("Failed to find an available address to run the gRPC server: %v", err)
	}
	s, requestCh := otlpOnGRPCServer(ln)

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		<-ctx.Done()
		s.GracefulStop()
		return nil
	})

	got := []map[string]any{}

	config, err := transformationConfig.generateOTelOTLPExporterConfig(ctx, t, name, ln.Addr().String())
	if err != nil {
		got = append(got, map[string]any{"config_error": err.Error()})
		return got
	}

	t.Logf("otelopscol config:\n%s", config)

	if len(*otelopscolPath) == 0 {
		t.Skip("--otelopscol not supplied")
	}

	testStartTime := time.Now()

	// Start otelopscol
	cmd := exec.Command(
		*otelopscolPath,
		"--config=env:OTELOPSCOL_CONFIG",
	)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("OTELOPSCOL_CONFIG=%s", config),
		// Run all tests in a non-UTC timezone to test timezone handling.
		"TZ=America/Los_Angeles",
	)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal("Failed to create stderr pipe:", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatal("Failed to start command:", err)
	}

	var errors []any
	var exitErr error

	// Read from stderr until EOF and put any errors in `errors`.
	eg.Go(func() error {
		// Wait for the process to exit.
		defer eg.Go(func() error {
			if err := cmd.Wait(); err != nil {
				if _, ok := err.(*exec.ExitError); ok {
					exitErr = err
					t.Logf("process terminated with error: %v", err)
				} else {
					return fmt.Errorf("process failed: %w", err)
				}
			}
			cancel()
			return nil
		})

		consumingCount := 0
		r := bufio.NewReader(stderr)
		d := json.NewDecoder(r)
		for {
			log := map[string]any{}
			if err := d.Decode(&log); err == io.EOF {
				return nil
			} else if err != nil {
				buf, err2 := io.ReadAll(d.Buffered())
				if err2 != nil {
					return err
				}
				buf2, err2 := io.ReadAll(r)
				if err2 != nil {
					return err
				}
				stderr := fmt.Sprintf("%s%s", string(buf), string(buf2))
				t.Logf("collector stderr:\n%s", stderr)
				stderr = sanitizeOtelStacktrace(t, stderr)
				errors = append(errors, map[string]any{"stderr": stderr})
				return nil
			}
			b, err := json.Marshal(log)
			if err != nil {
				t.Errorf("failed to marshal otel log: %v", err)
			} else {
				t.Logf("collector log output: %s", b)
			}
			delete(log, "ts")
			level, _ := log["level"].(string)
			if level != "info" && level != "debug" && level != "None" {
				errors = append(errors, log)
			}
			msg, _ := log["msg"].(string)
			if strings.HasPrefix(msg, "Consuming files") {
				consumingCount += 1
				if consumingCount == 3 {
					// We've processed the entire input file. Signal the collector to stop.
					if err := cmd.Process.Signal(os.Interrupt); err != nil {
						t.Errorf("failed to signal process: %v", err)
					}
				}
			}
			stacktrace, ok := log["stacktrace"].(string)
			if ok {
				log["stacktrace"] = sanitizeOtelStacktrace(t, stacktrace)
			}
			if resource, ok := log["resource"].(map[string]any); ok {
				if _, ok := resource["service.instance.id"].(string); ok {
					resource["service.instance.id"] = "test-service-instance-id"
					log["resource"] = resource
				}
			}
		}
	})

	// Read and sanitize requests.
	eg.Go(func() error {
		for r := range requestCh {
			got = append(got, sanitizeOTLPExportRequest(t, r, testStartTime))
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		t.Errorf("errgroup failed: %v", err)
	}

	if exitErr != nil {
		got = append(got, map[string]any{"exit_error": exitErr.Error()})
	}
	if len(errors) != 0 {
		got = append(got, map[string]any{"collector_errors": errors})
	}
	return got
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
