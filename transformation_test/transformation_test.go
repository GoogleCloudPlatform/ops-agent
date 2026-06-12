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

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/shirou/gopsutil/host"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"golang.org/x/sync/errgroup"
	"gotest.tools/v3/golden"
)

const (
	transformationInput = "input.log"
	flbTag              = "transformation_test"
)

var (
	otelopscolPath = flag.String("otelopscol", os.Getenv("OTELOPSCOL"), "path to otelopscol")

	multilineTestPatterns = newTestMatchPatterns([]string{
		".*cassandra.*",
		".*couchdb.*",
		".*elasticsearch.*",
		".*flink.*",
		".*hadoop.*",
		".*hbase.*",
		".*kafka.*",
		".*mysql.*",
		".*oracledb.*",
		".*postgresql.*",
		".*rabbitmq.*",
		".*saphana.*",
		".*solr.*",
		".*tomcat.*",
		".*vault.*",
		".*wildfly.*",
		".*zookeeper.*",
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

			t.Run("otel", func(t *testing.T) {
				t.Parallel()
				transformationConfig.runOTelTest(t, dir.Name())
			})
		})
	}
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

func (transformationConfig transformationTest) runOTelTestInner(t *testing.T, name string) []map[string]any {
	ctx, cancel := context.WithCancel(testContext())
	defer cancel()

	// Start an OTLP-compatible receiver.
	ln, err := net.Listen("tcp", "localhost:")
	if err != nil {
		t.Fatalf("Failed to find an available address to run the gRPC server: %v", err)
	}

	var requestChOTLP <-chan plogotlp.ExportRequest

	mockS, requestChOTLP := otlpOnGRPCServer(ln)

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		<-ctx.Done()
		mockS.GracefulStop()
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
		for r := range requestChOTLP {
			got = append(got, sanitizeOTLPExportRequest(t, r, testStartTime))
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
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.LoggingProcessor { return &confgenerator.LoggingProcessorWindowsEventLogV2{} })
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.LoggingProcessor { return &confgenerator.LoggingProcessorWindowsEventLogRawXML{} })
	confgenerator.ReplaceLoggingProcessorMacro[apps.LoggingProcessorMacroIisAccess]()
}
