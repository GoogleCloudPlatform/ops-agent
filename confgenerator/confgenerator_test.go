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

package confgenerator

import (
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/shirou/gopsutil/host"
)

const (
	validTestdataDir   = "testdata/valid"
	invalidTestdataDir = "testdata/invalid"
)

var (
	// Usage:
	//   ops-agent$ go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator -update_golden
	// Add "-v" to show details for which files are updated with what:
	//   ops-agent$ go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator -update_golden -v
	updateGolden       = flag.Bool("update_golden", false, "Whether to update the expected golden confs if they differ from the actual generated confs.")
	goldenMainPath     = validTestdataDir + "/%s/%s/golden_fluent_bit_main.conf"
	goldenParserPath   = validTestdataDir + "/%s/%s/golden_fluent_bit_parser.conf"
	goldenCollectdPath = validTestdataDir + "/%s/%s/golden_collectd.conf"
	goldenOtelPath     = validTestdataDir + "/%s/%s/golden_otel.conf"
	goldenErrorPath    = invalidTestdataDir + "/%s/%s/golden_error"
	invalidInputPath   = invalidTestdataDir + "/%s/%s/input.yaml"
)

type platformConfig struct {
	defaultLogsDir  string
	defaultStateDir string
	defaultConfig   string
	host.InfoStat
}

var platforms = []platformConfig{
	platformConfig{
		defaultLogsDir:  "/var/log/google-cloud-ops-agent/subagents",
		defaultStateDir: "/var/lib/google-cloud-ops-agent/fluent-bit",
		defaultConfig:   "default-config.yaml",
		InfoStat: host.InfoStat{
			OS:              "linux",
			Platform:        "linux_platform",
			PlatformVersion: "linux_platform_version",
		},
	},
	platformConfig{
		defaultLogsDir:  `C:\ProgramData\Google\Cloud Operations\Ops Agent\log`,
		defaultStateDir: `C:\ProgramData\Google\Cloud Operations\Ops Agent\run`,
		defaultConfig:   "windows-default-config.yaml",
		InfoStat: host.InfoStat{
			OS:              "windows",
			Platform:        "win_platform",
			PlatformVersion: "win_platform_version",
		},
	},
}

// testJoin can be used to override filepathJoin in order
// to impersonate the behavior of an alternate OS.
func testJoin(goos string, elem ...string) string {
	separator := "/"
	if goos == "windows" {
		separator = `\`
	}
	return strings.Join(elem, separator)
}

func TestGenerateConfsWithValidInput(t *testing.T) {
	t.Parallel()
	filepathJoin = testJoin
	for _, platform := range platforms {
		t.Run(platform.OS, func(t *testing.T) {
			t.Parallel()
			testGenerateConfsWithValidInput(t, platform)
		})
	}
}

func testGenerateConfsWithValidInput(t *testing.T, platform platformConfig) {
	dirPath := filepath.Join(validTestdataDir, platform.OS)
	dirs, err := ioutil.ReadDir(dirPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		t.Run(d.Name(), func(t *testing.T) {
			t.Parallel()
			unifiedConfigFilePath := filepath.Join(dirPath, d.Name(), "/input.yaml")
			// Special-case the default config.  It lives directly in the
			// confgenerator directory.  The golden files are still in the
			// testdata directory.
			if d.Name() == "default_config" {
				unifiedConfigFilePath = platform.defaultConfig
			}

			data, err := ioutil.ReadFile(unifiedConfigFilePath)
			if err != nil {
				t.Fatalf("ReadFile(%q) got %v", unifiedConfigFilePath, err)
			}
			uc, err := ParseUnifiedConfig(data)
			if err != nil {
				t.Fatalf("ParseUnifiedConfig got %v", err)
			}

			// Retrieve the expected golden conf files.
			expectedMainConfig := readFileContent(t, platform.OS, d.Name(), goldenMainPath, true)
			expectedParserConfig := readFileContent(t, platform.OS, d.Name(), goldenParserPath, true)

			// Generate the actual conf files.
			mainConf, parserConf, err := uc.GenerateFluentBitConfigs(platform.defaultLogsDir, platform.defaultStateDir, &platform.InfoStat)
			if err != nil {
				t.Errorf("GenerateFluentBitConfigs got %v", err)
			} else {
				// Compare the expected and actual and error out in case of diff.
				updateOrCompareGolden(t, d.Name(), platform.OS, expectedMainConfig, mainConf, goldenMainPath)
				updateOrCompareGolden(t, d.Name(), platform.OS, expectedParserConfig, parserConf, goldenParserPath)
			}

			if platform.OS == "windows" {
				expectedOtelConfig := readFileContent(t, platform.OS, d.Name(), goldenOtelPath, true)
				otelConf, err := uc.GenerateOtelConfig(&platform.InfoStat)
				if err != nil {
					t.Errorf("GenerateOtelConfig got %v", err)
				} else {
					// Compare the expected and actual and error out in case of diff.
					updateOrCompareGolden(t, d.Name(), platform.OS, expectedOtelConfig, otelConf, goldenOtelPath)
				}
			} else {
				expectedCollectdConfig := readFileContent(t, platform.OS, d.Name(), goldenCollectdPath, true)
				collectdConf, err := uc.GenerateCollectdConfig(platform.defaultLogsDir)
				if err != nil {
					t.Errorf("GenerateCollectdConfig got %v", err)
				} else {
					// Compare the expected and actual and error out in case of diff.
					updateOrCompareGolden(t, d.Name(), platform.OS, expectedCollectdConfig, collectdConf, goldenCollectdPath)
				}
			}
		})
	}
}

func readFileContent(t *testing.T, goos string, testName string, filePathFormat string, respectGolden bool) []byte {
	filePath := fmt.Sprintf(filePathFormat, goos, testName)
	rawExpectedConfig, err := ioutil.ReadFile(filePath)
	if err != nil {
		if *updateGolden && respectGolden {
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
	if diff := cmp.Diff(actual, expected); diff != "" {
		if *updateGolden {
			// Update the expected to match the actual.
			t.Logf("Detected -update_golden flag. Rewriting the %q golden file to apply the following diff\n%s.", goldenPath, diff)
			if err := ioutil.WriteFile(goldenPath, []byte(actual), 0644); err != nil {
				t.Fatalf("error updating golden file at %q : %s", goldenPath, err)
			}
		} else {
			t.Errorf("test %q: golden file at %s mismatch (-got +want):\n%s", testName, goldenPath, diff)
		}
	}
}

func TestGenerateConfigsWithInvalidInput(t *testing.T) {
	t.Parallel()
	for _, platform := range platforms {
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
		t.Run(d.Name(), func(t *testing.T) {
			t.Parallel()
			invalidInput := readFileContent(t, platform.OS, d.Name(), invalidInputPath, false)
			expectedError := readFileContent(t, platform.OS, d.Name(), goldenErrorPath, true)
			actualError := generateConfigs(invalidInput, platform)

			if actualError == nil {
				t.Errorf("test %q: generateConfigs succeeded, want error:\n%s\ninvalid input:\n%s", d.Name(), expectedError, invalidInput)
			} else {
				updateOrCompareGolden(t, d.Name(), platform.OS, expectedError, actualError.Error(), goldenErrorPath)
			}
		})
	}
}

// The expected error could be triggered by:
// 1. Parsing phase of the agent config when the config is not YAML.
// 2. Config generation phase when the config is invalid.
// If at any point, an error is generated, immediately return it for validation.
func generateConfigs(invalidInput []byte, platform platformConfig) (err error) {
	uc, err := ParseUnifiedConfig(invalidInput)
	if err != nil {
		return err
	}

	if _, _, err := uc.GenerateFluentBitConfigs(platform.defaultLogsDir, platform.defaultStateDir, &platform.InfoStat); err != nil {
		return err
	}

	if platform.OS == "windows" {
		if _, err = uc.GenerateOtelConfig(&platform.InfoStat); err != nil {
			return err
		}
	} else {
		if _, err := uc.GenerateCollectdConfig(platform.defaultLogsDir); err != nil {
			return err
		}
	}
	return nil
}
