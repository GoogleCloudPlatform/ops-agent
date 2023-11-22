package transformation_test

import (
	"bytes"
	"context"
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
	"go.opentelemetry.io/collector/pdata/plog"
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
	if len(*flbPath) == 0 {
		t.Skip("--flb not supplied")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Generate config files
	genFiles, err := generateFluentBitConfigs(ctx, name, transformationConfig)

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

	var stdout, stderr io.ReadCloser
	stdout, err = cmd.StdoutPipe()
	if err != nil {
		t.Fatal("stdout pipe failure", err)
	}
	stderr, err = cmd.StderrPipe()
	if err != nil {
		t.Fatal("stderr pipe failure", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatal("Failed to start command:", err)
	}

	// stderr and stdout need to be read in parallel to prevent deadlock
	var eg errgroup.Group
	eg.Go(func() error {
		// read stderr
		slurp, _ := io.ReadAll(stderr)
		t.Logf("stderr: %s\n", slurp)
		return nil
	})

	// read and unmarshal output
	var data []map[string]any
	out, _ := io.ReadAll(stdout)
	_ = eg.Wait()

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
				data[i]["date"] = date.Format(time.RFC3339Nano)
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
		t.Fatal(err)
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

func (transformationConfig transformationTest) generateOTelConfig(t *testing.T, name string, otlpAddr string) (string, error) {
	abs, err := filepath.Abs(filepath.Join("testdata", name, transformationInput))
	if err != nil {
		return "", err
	}
	return otel.ModularConfig{
		DisableMetrics: true,
		ReceiverPipelines: map[string]otel.ReceiverPipeline{
			"input": otel.ReceiverPipeline{
				Receiver: otel.Component{
					Type: "filelog",
					Config: map[string]interface{}{
						"include": []string{
							abs,
						},
						"start_at": "beginning",
					},
				},
				Processors:    map[string][]otel.Component{"logs": nil},
				ExporterTypes: map[string]otel.ExporterType{"logs": otel.System},
			},
		},
		Pipelines: map[string]otel.Pipeline{
			"input": otel.Pipeline{
				Type:                 "logs",
				ReceiverPipelineName: "input",
				Processors:           nil, // FIXME
			},
		},
		Exporters: map[otel.ExporterType]otel.Component{
			otel.System: otel.Component{
				Type: "otlp",
				Config: map[string]interface{}{
					"endpoint": otlpAddr,
					"tls": map[string]interface{}{
						"insecure": true,
					},
				},
			},
		},
	}.Generate()
}

type mockLogsReceiver struct {
	plogotlp.UnimplementedGRPCServer
	srv  *grpc.Server
	mtx  sync.Mutex
	logs []plog.Logs
}

func (r *mockLogsReceiver) Export(ctx context.Context, req plogotlp.ExportRequest) (plogotlp.ExportResponse, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.logs = append(r.logs, req.Logs())
	return plogotlp.NewExportResponse(), nil
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
	if len(*otelopscolPath) == 0 {
		t.Skip("--otelopscol not supplied")
	}

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

	config, err := transformationConfig.generateOTelConfig(t, name, ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("otelopscol config:\n%s", config)

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

	var stdout, stderr io.ReadCloser
	stdout, err = cmd.StdoutPipe()
	if err != nil {
		t.Fatal("stdout pipe failure", err)
	}
	stderr, err = cmd.StderrPipe()
	if err != nil {
		t.Fatal("stderr pipe failure", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatal("Failed to start command:", err)
	}

	// stderr and stdout need to be read in parallel to prevent deadlock
	var eg errgroup.Group
	eg.Go(func() error {
		// read stderr
		slurp, _ := io.ReadAll(stderr)
		t.Logf("stderr: %s\n", slurp)
		return nil
	})

	go func() {
		// TODO: Figure out how to wait for the collector to process the logs before signaling shutdown.
		time.Sleep(5 * time.Second)
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			t.Logf("failed to signal process: %v", err)
		}
	}()

	// read and unmarshal output
	var data []map[string]any
	out, _ := io.ReadAll(stdout)
	_ = eg.Wait()

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
				data[i]["date"] = date.Format(time.RFC3339Nano)
			}
		}
	}
	checkOutput(t, filepath.Join(name, "output_otel.yml"), data)
}
