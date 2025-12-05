// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	"regexp"
	"strings"
	"testing"
	"time"

	logpb "cloud.google.com/go/logging/apiv2/loggingpb"
	_ "github.com/GoogleCloudPlatform/ops-agent/apps"
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

	multilineTestPatterns = newTestMatchPatterns([]string{
		".*flink.*",
		".*elasticsearch.*",
		".*mysql.*",
		".*hbase.*",
		".*cassandra.*",
		".*couchdb.*",
		".*hadoop.*",
		".*kafka.*",
		".*postgresql.*",
		".*rabbitmq.*",
		".*wildfly.*",
		".*zookeeper.*",
		".*vault.*",
		".*solr.*",
		".*oracledb.*",
		".*saphana.*",
		".*tomcat.*",
	})
)

func isMultilineTest(s string) bool {
	return multilineTestPatterns.testMatch(s)
}

const flbMultilineTestKey = "fluent_bit_long_flush"

func contextWithFlbMultilineTest(ctx context.Context) context.Context {
	return context.WithValue(ctx, flbMultilineTestKey, true)
}

func contextHasFlbMulttilineTest(ctx context.Context) bool {
	return ctx.Value(flbMultilineTestKey) == true
}

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
	ctx, cancel := context.WithCancel(testContext())
	defer cancel()

	if isMultilineTest(name) {
		ctx = contextWithFlbMultilineTest(ctx)
	}

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
		*flbPath,
		"-v",
		fmt.Sprintf("--config=%s", filepath.Join(tempPath, flbMainConf)),
		fmt.Sprintf("--parser=%s", filepath.Join(filepath.Join(tempPath, flbParserConf))))

	// unmarshal output
	data := []map[string]any{}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Log(stderr.String())
		t.Log("Failed to run command:", err)
		data = append(data, map[string]any{"exit_error": err.Error()})
		sanitizedStderr := sanitizeFluentBitStderr(t, stderr.String())
		data = append(data, map[string]any{"collector_errors": map[string]any{"stderr": sanitizedStderr}})
	}
	t.Logf("stderr: %s\n", stderr.Bytes())

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
	gotBytes, err := yaml.MarshalWithOptions(got, yaml.UseLiteralStyleIfMultiline(true))
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

type inputReceiver struct {
	confgenerator.LoggingReceiverFilesMixin
}

func (inputReceiver) Type() string {
	return "transformation_test"
}

func (t transformationTest) pipelineInstance(path string) confgenerator.PipelineInstance {
	var processors []struct {
		ID string
		confgenerator.Component
	}
	for i, p := range t {
		processors = append(processors, struct {
			ID string
			confgenerator.Component
		}{
			fmt.Sprintf("processor%d", i), // only used for error messages
			p.LoggingProcessor,
		})
	}
	return confgenerator.PipelineInstance{
		PipelineType: "logs",
		PID:          flbTag,
		RID:          flbTag,
		Receiver: &inputReceiver{confgenerator.LoggingReceiverFilesMixin{
			IncludePaths: []string{
				path,
			},
			TransformationTest: true,
		}},
		Processors: processors,
	}
}

func generateFluentBitConfigs(ctx context.Context, name string, transformationTest transformationTest) (map[string]string, error) {
	abs, err := filepath.Abs(filepath.Join("testdata", name, transformationInput))
	if err != nil {
		return nil, err
	}

	components := []fluentbit.Component{}

	if contextHasFlbMulttilineTest(ctx) {
		service := fluentbit.Component{
			Kind: "SERVICE",
			Config: map[string]string{
				// The combination of Exit_On_Eof on a tail receiver with a multiline parser causes
				// the last log in a file to be dropped. See :
				// - https://github.com/fluent/fluent-bit/issues/8623
				// - https://github.com/fluent/fluent-bit/issues/8353
				// - https://github.com/fluent/fluent-bit/issues/3926
				// Some attempts of a solution have been implemented :
				// - https://github.com/fluent/fluent-bit/pull/8545
				// On newer fluent-bit 4.0.x versions, last log in a file maybe (non-deterministically)
				// dropped (~%85 retries) or sent (~15% retries) causing flaky tests.
				// Set shutdown "Grace" period to 0s to avoid any unreliable logs to be sent after Exit_On_Eof.
				// This forces the last log line from an multiline parser to always be dropped.
				"Grace": "0",
				"Flush": "10",
			},
		}

		components = append(components, service)
	}

	pi := transformationTest.pipelineInstance(abs)
	fbSource, err := pi.FluentBitComponents(ctx)
	if err != nil {
		return nil, err
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
	components = append(components, fbSource.Components...)
	components = append(components, output)
	return fluentbit.ModularConfig{
		Components: components,
	}.Generate()
}

func testContext() context.Context {
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
	return pl.TestContext(context.Background())
}

// generateOTelConfig attempts to generate an OTel config file for the test case.
// It calls t.Fatal if there is something wrong with the test case, or returns an error if the config is invalid.
func (transformationConfig transformationTest) generateOTelConfig(ctx context.Context, t *testing.T, name string, addr string) (string, error) {
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

	return otel.ModularConfig{
		DisableMetrics:    true,
		JSONLogs:          true,
		LogLevel:          "debug",
		ReceiverPipelines: rps,
		Pipelines:         pls,
		Exporters: map[otel.ExporterType]otel.Component{
			otel.OTel: {
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
	}.Generate(ctx, false)
}

type mockLoggingServer struct {
	logpb.UnimplementedLoggingServiceV2Server
	srv       *grpc.Server
	requestCh chan<- *logpb.WriteLogEntriesRequest
}

func (s *mockLoggingServer) WriteLogEntries(
	ctx context.Context,
	request *logpb.WriteLogEntriesRequest,
) (*logpb.WriteLogEntriesResponse, error) {
	s.requestCh <- request
	return &logpb.WriteLogEntriesResponse{}, nil
}

func (s *mockLoggingServer) GracefulStop() {
	// Also closes the connection.
	s.srv.GracefulStop()
	close(s.requestCh)
}

func cloudLoggingOnGRPCServer(ln net.Listener) (*mockLoggingServer, <-chan *logpb.WriteLogEntriesRequest) {
	ch := make(chan *logpb.WriteLogEntriesRequest)
	s := &mockLoggingServer{
		srv:       grpc.NewServer(),
		requestCh: ch,
	}

	// Now run it as a gRPC server
	logpb.RegisterLoggingServiceV2Server(s.srv, s)
	go func() {
		_ = s.srv.Serve(ln)
	}()

	return s, ch
}

func (transformationConfig transformationTest) runOTelTest(t *testing.T, name string) {
	got := transformationConfig.runOTelTestInner(t, name)

	checkOutput(t, filepath.Join(name, "output_otel.yaml"), got)
}

func (transformationConfig transformationTest) runOTelTestInner(t *testing.T, name string) []map[string]any {
	ctx, cancel := context.WithCancel(testContext())
	defer cancel()

	// Start an OTLP-compatible receiver.
	ln, err := net.Listen("tcp", "localhost:")
	if err != nil {
		t.Fatalf("Failed to find an available address to run the gRPC server: %v", err)
	}
	s, requestCh := cloudLoggingOnGRPCServer(ln)

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		<-ctx.Done()
		s.GracefulStop()
		return nil
	})

	got := []map[string]any{}

	config, err := transformationConfig.generateOTelConfig(ctx, t, name, ln.Addr().String())
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
			// Set "service.instance.id" to "test-service-instance-id" since it is a generated "uuid".
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
			got = append(got, sanitizeWriteLogEntriesRequest(t, r, testStartTime))
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		t.Errorf("errgroup failed: %v", err)
	}

	// Package up errors to be included in the golden output.
	if exitErr != nil {
		got = append(got, map[string]any{"exit_error": exitErr.Error()})
	}
	if len(errors) != 0 {
		got = append(got, map[string]any{"collector_errors": errors})
	}
	return got
}

func sanitizeWriteLogEntriesRequest(t *testing.T, r *logpb.WriteLogEntriesRequest, testStartTime time.Time) map[string]any {
	b, err := protojson.Marshal(r)
	if err != nil {
		t.Logf("failed to marshal request: %v", err)
		return nil
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
					return nil
				}
				if date.After(testStartTime) {
					v1["timestamp"] = "now"
				}
			}
		}
	}
	return req
}

func sanitizeFluentBitStderr(t *testing.T, input string) string {
	// We need to remove non-deterministic information from fluent-bit stderr so the goldens don't keep changing.
	// Only keep "[error]" lines.
	result := strings.Join(regexp.MustCompile(`(?m)^.*\[error\].*$`).FindAllString(input, -1), "\n")
	// Remove timestamps
	result = regexp.MustCompile(`\d{4}/\d{2}/\d{2}\s\d{2}:\d{2}:\d{2}`).ReplaceAllString(result, "YYYY/MM/DD HH:MM:SS")

	result = strings.ReplaceAll(result, "\t", "  ")
	return result
}

func sanitizeOtelStacktrace(t *testing.T, input string) string {
	// We need to remove non-deterministic information from stacktraces so the goldens don't keep changing.
	// Remove $GOPATH
	result := regexp.MustCompile(`(?m)^\t(.*?)pkg/mod/`).ReplaceAllString(input, "  ")
	// Remove function arguments
	result = regexp.MustCompile(`(?m)^(.*)\(.+\)$`).ReplaceAllString(result, "$1(...)")
	// Remove anything that looks like an address
	result = regexp.MustCompile(`0x[0-9a-f]+`).ReplaceAllString(result, "0xX")
	// Remove goroutine numbers
	result = regexp.MustCompile(`goroutine \d+`).ReplaceAllString(result, "goroutine N")
	// Remove timestamps
	result = regexp.MustCompile(`\d{4}/\d{2}/\d{2}\s\d{2}:\d{2}:\d{2}`).ReplaceAllString(result, "YYYY/MM/DD HH:MM:SS")

	result = strings.ReplaceAll(result, "\t", "  ")
	return result
}

type testMatchPatterns []*regexp.Regexp

func newTestMatchPatterns(patterns []string) testMatchPatterns {
	regexes := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		regexes = append(regexes, regexp.MustCompile(pattern))
	}
	return regexes
}

func (t testMatchPatterns) testMatch(s string) bool {
	for _, r := range t {
		if r.MatchString(s) {
			return true
		}
	}
	return false
}

func init() {
	// The processors registered here are only meant to be used in transformation tests.
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.LoggingProcessor { return &confgenerator.LoggingProcessorWindowsEventLogV1{} })
}
