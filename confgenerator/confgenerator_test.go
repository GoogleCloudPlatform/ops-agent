// Copyright 2020 Google LLC
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

package confgenerator_test

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/google/go-cmp/cmp"
	"github.com/shirou/gopsutil/host"
)

const (
	// Relative paths to the confgenerator folder.
	validTestdataDir   = "testdata/valid"
	invalidTestdataDir = "testdata/invalid"
	// Test name inside the confgenerator/testdata/valid/{linux|windows} folders.
	builtInConfTestName = "all-built_in_config"
)

var (
	// Usage:
	//   ops-agent$ go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator -update_golden
	// Add "-v" to show details for which files are updated with what:
	//   ops-agent$ go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator -update_golden -v
	updateGolden      = flag.Bool("update_golden", false, "Whether to update the expected golden confs if they differ from the actual generated confs.")
	goldenMainPath    = validTestdataDir + "/%s/%s/golden_fluent_bit_main.conf"
	goldenParserPath  = validTestdataDir + "/%s/%s/golden_fluent_bit_parser.conf"
	goldenOtelPath    = validTestdataDir + "/%s/%s/golden_otel.conf"
	goldenBuiltInPath = validTestdataDir + "/%s/%s/golden_built_in.yaml"
	goldenErrorPath   = invalidTestdataDir + "/%s/%s/golden_error"
	invalidInputPath  = invalidTestdataDir + "/%s/%s/input.yaml"
	mergedInputPath   = invalidTestdataDir + "/%s/%s/merged-config.yaml"
)

type platformConfig struct {
	defaultLogsDir  string
	defaultStateDir string
	*host.InfoStat
}

var platforms = []platformConfig{
	platformConfig{
		defaultLogsDir:  "/var/log/google-cloud-ops-agent/subagents",
		defaultStateDir: "/var/lib/google-cloud-ops-agent/fluent-bit",
		InfoStat: &host.InfoStat{
			OS:              "linux",
			Platform:        "linux_platform",
			PlatformVersion: "linux_platform_version",
		},
	},
	platformConfig{
		defaultLogsDir:  `C:\ProgramData\Google\Cloud Operations\Ops Agent\log`,
		defaultStateDir: `C:\ProgramData\Google\Cloud Operations\Ops Agent\run`,
		InfoStat: &host.InfoStat{
			OS:              "windows",
			Platform:        "win_platform",
			PlatformVersion: "win_platform_version",
		},
	},
}

func TestGenerateConfsWithValidInput(t *testing.T) {
	t.Parallel()
	for _, platform := range platforms {
		platform := platform
		t.Run(platform.OS, func(t *testing.T) {
			t.Parallel()
			testGenerateConfsWithValidInput(t, platform)
		})
	}
}

func testGenerateConfsWithValidInput(t *testing.T, platform platformConfig) {
	confgenerator.FindJarPath = func() (string, error) {
		return "/path/to/executables/opentelemetry-java-contrib-jmx-metrics.jar", nil
	}

	dirPath := filepath.Join(validTestdataDir, platform.OS)
	dirs, err := ioutil.ReadDir(dirPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		testName := d.Name()
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			confDebugFolder := filepath.Join(dirPath, testName)
			userSpecifiedConfPath := filepath.Join(confDebugFolder, "/input.yaml")
			builtInConfPath := filepath.Join(confDebugFolder, "/built-in-config.yaml")
			mergedConfPath := filepath.Join(confDebugFolder, "/merged-config.yaml")
			if err = confgenerator.MergeConfFiles(userSpecifiedConfPath, confDebugFolder, platform.OS, apps.BuiltInConfStructs); err != nil {
				t.Fatalf("MergeConfFiles(%q, %q) got: %v", userSpecifiedConfPath, confDebugFolder, err)
			}

			data, err := ioutil.ReadFile(mergedConfPath)
			if err != nil {
				t.Fatalf("ReadFile(%q) got: %v", userSpecifiedConfPath, err)
			}
			t.Logf("merged config:\n%s", data)
			uc, err := confgenerator.ParseUnifiedConfigAndValidate(data, platform.OS)
			if err != nil {
				t.Fatalf("ParseUnifiedConfigAndValidate got: %v", err)
			}

			// Retrieve the expected golden conf files.
			expectedMainConfig := readFileContent(t, testName, platform.OS, goldenMainPath, true)
			expectedParserConfig := readFileContent(t, testName, platform.OS, goldenParserPath, true)

			// Generate the actual conf files.
			mainConf, parserConf, err := uc.GenerateFluentBitConfigs(platform.defaultLogsDir, platform.defaultStateDir, platform.InfoStat)
			if err != nil {
				t.Fatalf("GenerateFluentBitConfigs got: %v", err)
			}
			// Compare the expected and actual and error out in case of diff.
			updateOrCompareGolden(t, testName, platform.OS, expectedMainConfig, mainConf, goldenMainPath)
			updateOrCompareGolden(t, testName, platform.OS, expectedParserConfig, parserConf, goldenParserPath)

			expectedOtelConfig := readFileContent(t, testName, platform.OS, goldenOtelPath, true)
			otelConf, err := uc.GenerateOtelConfig(platform.InfoStat)
			if err != nil {
				t.Fatalf("GenerateOtelConfig got: %v", err)
			}
			// Compare the expected and actual and error out in case of diff.
			updateOrCompareGolden(t, testName, platform.OS, expectedOtelConfig, otelConf, goldenOtelPath)

			// Compare the expected and generated built-in config and error out in case of diff.
			if testName == builtInConfTestName {
				expectedBuiltInConfig := readFileContent(t, testName, platform.OS, goldenBuiltInPath, true)
				generatedBuiltInConfig, err := ioutil.ReadFile(builtInConfPath)
				if err != nil {
					t.Fatalf("test %q: error reading %s: %v", testName, builtInConfPath, err)
				}
				updateOrCompareGolden(t, testName, platform.OS, expectedBuiltInConfig, string(generatedBuiltInConfig), goldenBuiltInPath)
			}
			if err = os.Remove(builtInConfPath); err != nil {
				t.Fatalf("DeleteFile(%q) got: %v", builtInConfPath, err)
			}
			if err = os.Remove(mergedConfPath); err != nil {
				t.Fatalf("DeleteFile(%q) got: %v", mergedConfPath, err)
			}
		})
	}
}

func readFileContent(t *testing.T, testName string, goos string, filePathFormat string, respectGolden bool) []byte {
	filePath := fmt.Sprintf(filePathFormat, goos, testName)
	rawExpectedConfig, err := ioutil.ReadFile(filePath)
	if err != nil {
		if *updateGolden && respectGolden && errors.Is(err, os.ErrNotExist) {
			// Tolerate the file not found error because we will overwrite it later anyway.
			return []byte("")
		} else {
			t.Fatalf("test %q: error reading the file from %s : %s", testName, filePath, err)
		}
	}
	return rawExpectedConfig
}

func updateOrCompareGolden(t *testing.T, testName string, goos string, expectedBytes []byte, actual string, path string) {
	t.Helper()
	expected := strings.ReplaceAll(string(expectedBytes), "\r\n", "\n")
	actual = strings.ReplaceAll(actual, "\r\n", "\n")
	goldenPath := fmt.Sprintf(path, goos, testName)
	diff := cmp.Diff(expected, actual)
	if *updateGolden {
		// If there is a diff, or if the actual is empty (it may be due to the file
		// not existing), write the golden file with the expected content.
		if diff != "" || actual == "" {
			// Update the expected to match the actual.
			t.Logf("Detected -update_golden flag. Rewriting the %q golden file to apply the following diff\n%s.", goldenPath, cmp.Diff(actual, expected))
			if err := ioutil.WriteFile(goldenPath, []byte(actual), 0644); err != nil {
				t.Fatalf("error updating golden file at %q : %s", goldenPath, err)
			}
		}
	} else if diff != "" {
		t.Errorf("test %q: golden file at %s mismatch (-want +got):\n%s", testName, goldenPath, diff)
	}
}

func TestGenerateConfigsWithInvalidInput(t *testing.T) {
	t.Parallel()
	for _, platform := range platforms {
		platform := platform
		t.Run(platform.OS, func(t *testing.T) {
			t.Parallel()
			testGenerateConfigsWithInvalidInput(t, platform)
		})
	}
}

func testGenerateConfigsWithInvalidInput(t *testing.T, platform platformConfig) {
	dirPath := filepath.Join(invalidTestdataDir, platform.OS)
	dirs, err := ioutil.ReadDir(dirPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		testName := d.Name()
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			confDebugFolder := filepath.Join(dirPath, testName)
			userSpecifiedConfPath := filepath.Join(confDebugFolder, "/input.yaml")
			builtInConfPath := filepath.Join(confDebugFolder, "/built-in-config.yaml")
			mergedConfPath := filepath.Join(confDebugFolder, "/merged-config.yaml")

			invalidInput := readFileContent(t, testName, platform.OS, invalidInputPath, false)
			expectedError := readFileContent(t, testName, platform.OS, goldenErrorPath, true)

			actualError := confgenerator.MergeConfFiles(userSpecifiedConfPath, confDebugFolder, platform.OS, apps.BuiltInConfStructs)
			if actualError == nil {
				mergedInput := readFileContent(t, testName, platform.OS, mergedInputPath, false)
				actualError = generateConfigs(mergedInput, platform)
			}
			if actualError == nil {
				t.Errorf("test %q: generateConfigs succeeded, want error:\n%s\ninvalid input:\n%s", testName, expectedError, invalidInput)
			} else {
				updateOrCompareGolden(t, testName, platform.OS, expectedError, actualError.Error(), goldenErrorPath)
			}

			// Clean up built-in and merged config now that the test passes.
			if err = os.Remove(builtInConfPath); err != nil {
				t.Fatalf("DeleteFile(%q) got: %v", builtInConfPath, err)
			}
			os.Remove(mergedConfPath)
		})
	}
}

// The expected error could be triggered by:
// 1. Parsing phase of the agent config when the config is not YAML.
// 2. Config generation phase when the config is invalid.
// If at any point, an error is generated, immediately return it for validation.
func generateConfigs(invalidInput []byte, platform platformConfig) (err error) {
	uc, err := confgenerator.ParseUnifiedConfigAndValidate(invalidInput, platform.OS)
	if err != nil {
		return err
	}

	if _, _, err := uc.GenerateFluentBitConfigs(platform.defaultLogsDir, platform.defaultStateDir, platform.InfoStat); err != nil {
		return err
	}

	if _, err = uc.GenerateOtelConfig(platform.InfoStat); err != nil {
		return err
	}
	return nil
}
