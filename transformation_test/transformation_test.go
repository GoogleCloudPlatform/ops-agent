package transformation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/encoding/gzip"
	"gotest.tools/v3/golden"
)

const (
	flbMainConf          = "fluent_bit_main.conf"
	flbParserConf        = "fluent_bit_parser.conf"
	transformationInput  = "input.log"
	transformationOutput = "output_fluentbit.yaml"
	flbTag               = "transformation_test"
)

var (
	flbPath        = flag.String("flb", os.Getenv("FLB"), "path to fluent-bit")
	otelopscolPath = flag.String("otelopscol", os.Getenv("OTELOPSCOL"), "path to otelopscol")
)

type transformationTest []loggingProcessor
type loggingProcessor struct {
	confgenerator.LoggingProcessor
}

func (l *loggingProcessor) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	return confgenerator.LoggingProcessorTypes.UnmarshalComponentYaml(ctx, &l.LoggingProcessor, unmarshal)
}

func TestTransformationTests(t *testing.T) {
	allTests, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}

	for _, dir := range allTests {
		dir := dir
		if !dir.IsDir() {
			continue
		}
		t.Run(dir.Name(), func(t *testing.T) {
			t.Parallel()
			// Unmarshal transformation_config.yaml
			var transformationConfig transformationTest
			transformationConfig, err = readTransformationConfig(dir.Name())
			if err != nil {
				t.Fatal("failed to unmarshal config:", err)
			}
			t.Run("fluent-bit", func(t *testing.T) {
				t.Parallel()
				transformationConfig.runFluentBitTest(t, dir.Name())
			})
			t.Run("otel", func(t *testing.T) {
				t.Parallel()
				transformationConfig.runOTelTest(t, dir.Name())
			})
		})
	}
}

func (transformationConfig transformationTest) runFluentBitTest(t *testing.T, name string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Generate config files
	genFiles, err := generateFluentBitConfigs(ctx, name, transformationConfig)
	if err != nil {
		t.Fatalf("failed to generate config files: %v", err)
	}

	if len(*flbPath) == 0 {
		t.Skip("--flb not supplied")
	}

	// Write config files in temp directory
	tempPath := t.TempDir()
	for k, v := range genFiles {
		err := confgenerator.WriteConfigFile([]byte(v), filepath.Join(tempPath, k))

		if err != nil {
			t.Fatal(err)
		}
		t.Logf("generated file %q\n%s", k, v)
	}

	testStartTime := time.Now()

	// Start Fluent-bit
	cmd := exec.Command(
		fmt.Sprintf("%s", *flbPath),
		"-v",
		fmt.Sprintf("--config=%s", filepath.Join(tempPath, flbMainConf)),
		fmt.Sprintf("--parser=%s", filepath.Join(filepath.Join(tempPath, flbParserConf))))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatal("Failed to run command:", err)
	}
	t.Logf("stderr: %s\n", stderr.Bytes())

	// unmarshal output
	var data []map[string]any

	out := stdout.Bytes()
	if err := yaml.Unmarshal(out, &data); err != nil {
		t.Log(string(out))
		t.Fatal(err)
	}

	// transform timestamp of actual results
	for i, d := range data {
		if date, ok := d["date"].(float64); ok {
			date := time.UnixMicro(int64(date * 1e6)).UTC()
			if date.After(testStartTime) {
				data[i]["date"] = "now"
			} else {
				data[i]["date"] = date.UTC().Format(time.RFC3339Nano)
			}
		}
	}
	checkOutput(t, filepath.Join(name, transformationOutput), data)
}

func checkOutput(t *testing.T, name string, got []map[string]any) {
	t.Helper()
	wantBytes := golden.Get(t, name)
	gotBytes, err := yaml.Marshal(got)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	t.Logf("%s", string(gotBytes))
	if golden.FlagUpdate() {
		golden.AssertBytes(t, gotBytes, name)
		return
	}
	var want []map[string]any
	if err := yaml.Unmarshal(wantBytes, &want); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Fatalf("got(-)/want(+):\n%s", diff)
	}
}
func readTransformationConfig(dir string) (transformationTest, error) {
	var transformationTestData []byte
	var config transformationTest

	transformationTestData, err := os.ReadFile(filepath.Join("testdata", dir, "config.yaml"))
	if err != nil {
		return config, err
	}
	transformationTestData = bytes.ReplaceAll(transformationTestData, []byte("\r\n"), []byte("\n"))

	err = yaml.UnmarshalWithOptions(transformationTestData, &config, yaml.DisallowUnknownField())
	if err != nil {
		return config, err
	}

	return config, nil
}

func generateFluentBitConfigs(ctx context.Context, name string, transformationTest transformationTest) (map[string]string, error) {
	abs, err := filepath.Abs(filepath.Join("testdata", name, transformationInput))
	if err != nil {
		return nil, err
	}
	var components []fluentbit.Component
	input := fluentbit.Component{
		Kind: "INPUT",
		Config: map[string]string{
			"Name":           "tail",
			"Tag":            flbTag,
			"Read_from_Head": "True",
			"Exit_On_Eof":    "True",
			"Path":           abs,
			"Key":            "message",
		},
	}

	components = append(components, input)

	for i, t := range transformationTest {
		components = append(components, t.Components(ctx, flbTag, strconv.Itoa(i))...)
	}

	output := fluentbit.Component{
		Kind: "OUTPUT",
		Config: map[string]string{
			"Name":   "stdout",
			"Match":  "*",
			"Format": "json",
		},
	}
	components = append(components, output)
	return fluentbit.ModularConfig{
		Components: components,
	}.Generate()
}

func (transformationConfig transformationTest) generateOTelConfig(ctx context.Context, t *testing.T, name string, otlpAddr string) (string, error) {
	abs, err := filepath.Abs(filepath.Join("testdata", name, transformationInput))
	if err != nil {
		return "", err
	}
	var components []otel.Component
	for _, p := range transformationConfig {
		if op, ok := p.LoggingProcessor.(confgenerator.OTelProcessor); ok {
			components = append(components, op.Processors()...)
		} else {
			t.Fatalf("not an OTel processor: %v", p)
		}
	}

	rp := confgenerator.LoggingReceiverFilesMixin{
		IncludePaths: []string{
			abs,
		},
	}.Pipelines(ctx)

	return otel.ModularConfig{
		DisableMetrics: true,
		JSONLogs:       true,
		ReceiverPipelines: map[string]otel.ReceiverPipeline{
			"input": rp[0],
		},
		Pipelines: map[string]otel.Pipeline{
			"input": otel.Pipeline{
				Type:                 "logs",
				ReceiverPipelineName: "input",
				Processors:           components,
			},
		},
		Exporters: map[otel.ExporterType]otel.Component{
			otel.OTel: otel.Component{
				Type: "otlp",
				Config: map[string]interface{}{
					"endpoint": otlpAddr,
					"tls": map[string]interface{}{
						"insecure": true,
					},
				},
			},
		},
	}.Generate(ctx)
}

type mockLogsReceiver struct {
	plogotlp.UnimplementedGRPCServer
	srv      *grpc.Server
	mtx      sync.Mutex
	requests []plogotlp.ExportRequest
}

func (r *mockLogsReceiver) Export(ctx context.Context, req plogotlp.ExportRequest) (plogotlp.ExportResponse, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.requests = append(r.requests, req)
	return plogotlp.NewExportResponse(), nil
}

func (r *mockLogsReceiver) Requests() []plogotlp.ExportRequest {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return append([]plogotlp.ExportRequest{}, r.requests...)
}

func otlpLogsReceiverOnGRPCServer(ln net.Listener) *mockLogsReceiver {
	rcv := &mockLogsReceiver{
		srv: grpc.NewServer(),
	}

	// Now run it as a gRPC server
	plogotlp.RegisterGRPCServer(rcv.srv, rcv)
	go func() {
		_ = rcv.srv.Serve(ln)
	}()

	return rcv
}

func (transformationConfig transformationTest) runOTelTest(t *testing.T, name string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start an OTLP-compatible receiver.
	ln, err := net.Listen("tcp", "localhost:")
	if err != nil {
		t.Fatalf("Failed to find an available address to run the gRPC server: %v", err)
	}
	rcv := otlpLogsReceiverOnGRPCServer(ln)
	// Also closes the connection.
	defer rcv.srv.GracefulStop()

	config, err := transformationConfig.generateOTelConfig(ctx, t, name, ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("otelopscol config:\n%s", config)

	if len(*otelopscolPath) == 0 {
		t.Skip("--otelopscol not supplied")
	}

	testStartTime := time.Now()

	// Start otelopscol
	cmd := exec.Command(
		fmt.Sprintf("%s", *otelopscolPath),
		"--config=env:OTELOPSCOL_CONFIG",
	)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("OTELOPSCOL_CONFIG=%s", config),
	)

	// TODO
	_ = ctx

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

	var eg errgroup.Group
	eg.Go(func() error {
		// TODO: Figure out how to wait for the collector to process the logs before signaling shutdown.
		time.Sleep(5 * time.Second)
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			t.Errorf("failed to signal process: %v", err)
		}
		return nil
	})
	eg.Go(func() error {
		d := json.NewDecoder(stderr)
		for {
			log := map[string]any{}
			if err := d.Decode(&log); err == io.EOF {
				return nil
			} else if err != nil {
				return err
			}
			delete(log, "ts")
			level, _ := log["level"].(string)
			if level != "info" && level != "debug" && level != "None" {
				errors = append(errors, log)
			}
			b, err := json.Marshal(log)
			if err != nil {
				t.Errorf("failed to marshal otel log: %v", err)
			} else {
				t.Logf("collector log output: %s", b)
			}
		}
	})

	if err := eg.Wait(); err != nil {
		t.Errorf("errgroup failed: %v", err)
	}

	if err := cmd.Wait(); err != nil {
		t.Errorf("process failed: %v", err)
	}

	// read and unmarshal output
	reqs := rcv.Requests()
	var data []map[string]any
	for _, r := range reqs {
		b, err := r.MarshalJSON()
		if err != nil {
			t.Logf("failed to marshal request: %v", err)
			continue
		}
		var req map[string]any
		if err := yaml.Unmarshal(b, &req); err != nil {
			t.Log(string(b))
			t.Fatal(err)
		}
		// Replace resourceLogs[].scopeLogs[].logRecords[].observedTimeUnixNano with a human-readable timestamp
		if v, ok := req["resourceLogs"].([]any); ok {
			for _, v := range v {
				v1, _ := v.(map[string]any)
				v2, _ := v1["scopeLogs"].([]any)
				for _, v := range v2 {
					v1, _ := v.(map[string]any)
					v2, _ := v1["logRecords"].([]any)
					for _, v := range v2 {
						v1 := v.(map[string]any)
						// Convert timestamp to "now" or a human-readable timestamp
						for _, name := range []string{"observedTimeUnixNano", "timeUnixNano"} {
							if dateStr, ok := v1[name].(string); ok {
								dateInt, err := strconv.ParseInt(dateStr, 10, 64)
								if err != nil {
									t.Logf("failed to parse %q: %v", dateStr, err)
									continue
								}
								date := time.Unix(0, dateInt)
								if date.After(testStartTime) {
									v1[name] = "now"
								} else {
									v1[name] = date.UTC().Format(time.RFC3339Nano)
								}
							}
						}
						// Convert kvlistValue to a map
						if body, ok := v1["body"].(map[string]any); ok {
							if kv, ok := body["kvlistValue"].(map[string]any); ok {
								kvOut := map[string]any{}
								for _, kv := range kv["values"].([]any) {
									kv1 := kv.(map[string]any)
									key := kv1["key"].(string)
									value := kv1["value"]
									kvOut[key] = value
								}
								v1["body"] = kvOut
							}
						}
					}
				}
			}
		}

		data = append(data, req)
	}
	// Package up collector errors to be included in the golden output.
	if len(errors) != 0 {
		data = append(data, map[string]any{"collector_errors": errors})
	}
	checkOutput(t, filepath.Join(name, "output_otel.yaml"), data)
}
