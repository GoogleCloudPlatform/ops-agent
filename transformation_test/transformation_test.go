package transformation_test

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/sync/errgroup"
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
	flbPath = flag.String("flb", os.Getenv("FLB"), "Path to Fluent-bit binary")
)

type transformationTest []loggingProcessor
type loggingProcessor struct {
	confgenerator.LoggingProcessor
}

func (l *loggingProcessor) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	return confgenerator.LoggingProcessorTypes.UnmarshalComponentYaml(ctx, &l.LoggingProcessor, unmarshal)
}

func TestTransformationTests(t *testing.T) {
	ctx := context.Background()
	if len(*flbPath) == 0 {
		t.Skip("--flb not supplied")
	}

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
			testStartTime := time.Now()
			// Unmarshal transformation_config.yaml
			var transformationConfig transformationTest
			transformationConfig, err = readTransformationConfig(dir.Name())
			if err != nil {
				t.Fatal("failed to unmarshal config:", err)
			}

			// Generate config files
			var genFiles map[string]string
			genFiles, err = generateFluentBitConfigs(ctx, transformationConfig, dir.Name())

			// Write config files in temp directory
			tempPath := t.TempDir()
			for k, v := range genFiles {
				err := confgenerator.WriteConfigFile([]byte(v), filepath.Join(tempPath, k))

				if err != nil {
					t.Fatal(err)
				}
				t.Logf("generated file %q\n%s", k, v)
			}

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

			err := yaml.Unmarshal(out, &data)
			if err != nil {
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
			checkOutput(t, filepath.Join(dir.Name(), transformationOutput), data)
		})
	}
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

func generateFluentBitConfigs(ctx context.Context, transformationTest transformationTest, dirPath string) (map[string]string, error) {
	abs, err := filepath.Abs(filepath.Join("testdata", dirPath, transformationInput))
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
