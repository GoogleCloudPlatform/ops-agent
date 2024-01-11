package transformation_test

import (
	"bufio"
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
	"strings"
	"sync"
	"testing"
	"time"

	logpb "cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/shirou/gopsutil/host"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/protobuf/encoding/protojson"
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
		t.Log(stderr.String())
		t.Fatal("Failed to run command:", err)
	}
	t.Logf("stderr: %s\n", stderr.Bytes())

	// unmarshal output
	data := []map[string]any{}

	dec := json.NewDecoder(strings.NewReader(stdout.String()))
	for {
		var req map[string]any
		// decode an array value (Message)
		if err := dec.Decode(&req); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		data = append(data, req)
	}

	// transform timestamp of actual results
	for _, req := range data {
		// Only search for entries if stdout is not null
		if val, ok := req["entries"].([]any); ok {
			for _, e := range val {
				entry := e.(map[string]interface{})
				date := entry["timestamp"].(string)
				timestamp, err := time.Parse(time.RFC3339Nano, date)
				if err != nil {
					t.Fatal(err)
				}
				if timestamp.After(testStartTime) {
					entry["timestamp"] = "now"
				}
			}
		}
	}

	checkOutput(t, filepath.Join(name, transformationOutput), data)
}

func checkOutput(t *testing.T, name string, got []map[string]any) {
	t.Helper()
	gotBytes, err := yaml.Marshal(got)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	t.Logf("%s", string(gotBytes))
	if golden.FlagUpdate() {
		golden.AssertBytes(t, gotBytes, name)
		return
	}
	wantBytes := golden.Get(t, name)
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
			"Match":                         "*",
			"Name":                          "stackdriver",
			"Retry_Limit":                   "3",
			"http_request_key":              "logging.googleapis.com/httpRequest",
			"net.connect_timeout_log_error": "False",
			"resource":                      "gce_instance",
			"stackdriver_agent":             "Google-Cloud-Ops-Agent-Logging/latest (BuildDistro=build_distro;Platform=linux;ShortName=linux_platform;ShortVersion=linux_platform_version)",
			"storage.total_limit_size":      "2G",
			"tls":                           "On",
			"tls.verify":                    "Off",
			"workers":                       "8",
			"test_log_entry_format":         "true",
			"export_to_project_id":          "my-project",
		},
	}
	components = append(components, output)
	return fluentbit.ModularConfig{
		Components: components,
	}.Generate()
}

func (transformationConfig transformationTest) generateOTelConfig(ctx context.Context, t *testing.T, name string, addr string) (string, error) {
	pl := platform.Platform{
		Type: platform.Linux,
		HostInfo: &host.InfoStat{
			Hostname:        "hostname",
			OS:              "linux",
			Platform:        "linux_platform",
			PlatformVersion: "linux_platform_version",
		},
		ResourceOverride: resourcedetector.GCEResource{
			Project:    "my-project",
			Zone:       "test-zone",
			InstanceID: "test-instance-id",
		},
	}
	ctx = pl.TestContext(ctx)

	abs, err := filepath.Abs(filepath.Join("testdata", name, transformationInput))
	if err != nil {
		return "", err
	}
	var components []otel.Component
	for _, p := range transformationConfig {
		if op, ok := p.LoggingProcessor.(confgenerator.OTelProcessor); ok {
			components = append(components, op.Processors()...)
		} else {
			t.Fatalf("not an OTel processor: %#v", p.LoggingProcessor)
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
				Type: "googlecloud",
				Config: map[string]any{
					"project": "my-project",
					"sending_queue": map[string]any{
						"enabled": false,
					},
					"log": map[string]any{
						"default_log_name": "my-log-name",
						"endpoint":         addr,
						"use_insecure":     true,
					},
				},
			},
		},
	}.Generate(ctx)
}

type mockLoggingServer struct {
	logpb.UnimplementedLoggingServiceV2Server
	srv      *grpc.Server
	mtx      sync.Mutex
	requests []*logpb.WriteLogEntriesRequest
}

func (s *mockLoggingServer) WriteLogEntries(
	ctx context.Context,
	request *logpb.WriteLogEntriesRequest,
) (*logpb.WriteLogEntriesResponse, error) {

	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.requests = append(s.requests, request)
	return &logpb.WriteLogEntriesResponse{}, nil
}

func (s *mockLoggingServer) Requests() []*logpb.WriteLogEntriesRequest {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return append([]*logpb.WriteLogEntriesRequest{}, s.requests...)
}

func cloudLoggingOnGRPCServer(ln net.Listener) *mockLoggingServer {
	s := &mockLoggingServer{
		srv: grpc.NewServer(),
	}

	// Now run it as a gRPC server
	logpb.RegisterLoggingServiceV2Server(s.srv, s)
	go func() {
		_ = s.srv.Serve(ln)
	}()

	return s
}

func (transformationConfig transformationTest) runOTelTest(t *testing.T, name string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start an OTLP-compatible receiver.
	ln, err := net.Listen("tcp", "localhost:")
	if err != nil {
		t.Fatalf("Failed to find an available address to run the gRPC server: %v", err)
	}
	s := cloudLoggingOnGRPCServer(ln)
	// Also closes the connection.
	defer s.srv.GracefulStop()

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
		r := bufio.NewReader(stderr)
		d := json.NewDecoder(r)
		for {
			log := map[string]any{}
			if err := d.Decode(&log); err == io.EOF {
				return nil
			} else if err != nil {
				// Not valid JSON, just print the raw stderr.
				// This happens when the config is invalid.
				buf, err2 := io.ReadAll(d.Buffered())
				if err2 != nil {
					return err
				}
				buf2, err2 := io.ReadAll(r)
				if err2 != nil {
					return err
				}
				return fmt.Errorf("stderr: %s%s", string(buf), string(buf2))
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
	reqs := s.Requests()
	var data []map[string]any
	for _, r := range reqs {
		b, err := protojson.Marshal(r)
		if err != nil {
			t.Logf("failed to marshal request: %v", err)
			continue
		}
		var req map[string]any
		if err := yaml.Unmarshal(b, &req); err != nil {
			t.Log(string(b))
			t.Fatal(err)
		}
		// Replace entries[].timestamp with a human-readable timestamp
		if v, ok := req["entries"].([]any); ok {
			for _, v := range v {
				v1, _ := v.(map[string]any)
				// Convert timestamp to "now" or a human-readable timestamp
				if dateStr, ok := v1["timestamp"].(string); ok {
					date, err := time.Parse(time.RFC3339Nano, dateStr)
					if err != nil {
						t.Logf("failed to parse %q: %v", dateStr, err)
						continue
					}
					if date.After(testStartTime) {
						v1["timestamp"] = "now"
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
