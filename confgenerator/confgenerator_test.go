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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/shirou/gopsutil/host"
)

const (
	validTestdataDir       = "testdata/valid"
	invalidTestdataDir     = "testdata/invalid"
	defaultLogsDir         = "/var/log/google-cloud-ops-agent/subagents"
	defaultStateDir        = "/var/lib/google-cloud-ops-agent/fluent-bit"
	windowsDefaultLogsDir  = "C:\\ProgramData\\Google\\Cloud Operations\\Ops Agent\\log"
	windowsDefaultStateDir = "C:\\ProgramData\\Google\\Cloud Operations\\Ops Agent\\run"
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

var platform string
var hostInfo *host.InfoStat

func init() {
	hostInfo, _ = host.Info()
	if hostInfo.OS == "windows" {
		platform = "windows"
		//In order to make test data static, we put static value for platform-wise fields.
		hostInfo.Platform = "win_platform"
		hostInfo.PlatformVersion = "win_platform_version"
	} else {
		platform = "linux"
		//In order to make test data static, we put static value for platform-wise fields.
		hostInfo.Platform = "linux_platform"
		hostInfo.PlatformVersion = "linux_platform_version"
	}
}

func TestGenerateConfsWithValidInput(t *testing.T) {
	dirPath := validTestdataDir + "/" + platform
	logsDir := defaultLogsDir
	stateDir := defaultStateDir
	if platform == "windows" {
		logsDir = windowsDefaultLogsDir
		stateDir = windowsDefaultStateDir
	}
	dirs, err := ioutil.ReadDir(dirPath)
	if err != nil {
		t.Fatal(err)
	}

	for _, d := range dirs {
		testName := d.Name()
		t.Run(testName, func(t *testing.T) {
			unifiedConfigFilePath := fmt.Sprintf(dirPath+"/%s/input.yaml", testName)
			// Special-case the default config.  It lives directly in the
			// confgenerator directory.  The golden files are still in the
			// testdata directory.
			if d.Name() == "default_config" {
				unifiedConfigFilePath = "default-config.yaml"
			} else if d.Name() == "windows_default_config" {
				unifiedConfigFilePath = "windows-default-config.yaml"
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
			expectedMainConfig := readFileContent(testName, goldenMainPath, t, true)
			expectedParserConfig := readFileContent(testName, goldenParserPath, t, true)
			// Generate the actual conf files.
			mainConf, parserConf, err := uc.GenerateFluentBitConfigs(logsDir, stateDir, hostInfo)
			if err != nil {
				t.Fatalf("GenerateFluentBitConfigs got %v", err)
			}
			// Compare the expected and actual and error out in case of diff.
			updateOrCompareGolden(t, testName, expectedMainConfig, mainConf, goldenMainPath)
			updateOrCompareGolden(t, testName, expectedParserConfig, parserConf, goldenParserPath)

			if platform == "windows" {
				expectedOtelConfig := readFileContent(testName, goldenOtelPath, t, true)
				otelConf, err := uc.GenerateOtelConfig(hostInfo)
				if err != nil {
					t.Fatalf("GenerateOtelConfig got %v", err)
				}
				// Compare the expected and actual and error out in case of diff.
				updateOrCompareGolden(t, testName, expectedOtelConfig, otelConf, goldenOtelPath)
			} else {
				expectedCollectdConfig := readFileContent(testName, goldenCollectdPath, t, true)
				collectdConf, err := uc.GenerateCollectdConfig(defaultLogsDir)
				if err != nil {
					t.Fatalf("GenerateCollectdConfig got %v", err)
				}
				// Compare the expected and actual and error out in case of diff.
				updateOrCompareGolden(t, testName, expectedCollectdConfig, collectdConf, goldenCollectdPath)
			}
		})
	}
}

func readFileContent(testName string, filePathFormat string, t *testing.T, respectGolden bool) []byte {
	filePath := fmt.Sprintf(filePathFormat, platform, testName)
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

func updateOrCompareGolden(t *testing.T, testName string, expectedBytes []byte, actual string, path string) {
	t.Helper()
	expected := strings.ReplaceAll(string(expectedBytes), "\r\n", "\n")
	actual = strings.ReplaceAll(actual, "\r\n", "\n")
	goldenPath := fmt.Sprintf(path, platform, testName)
	if diff := cmp.Diff(actual, expected); diff != "" {
		if *updateGolden {
			// Update the expected to match the actual.
			t.Logf("Detected -update_golden flag. Rewriting the %q golden file to apply the following diff\n%s.", goldenPath, diff)
			if err := ioutil.WriteFile(goldenPath, []byte(actual), 0644); err != nil {
				t.Fatalf("error updating golden file at %q : %s", goldenPath, err)
			}
		} else {
			t.Fatalf("test %q: golden file at %s mismatch (-got +want):\n%s", testName, goldenPath, diff)
		}
	}
}

func TestGenerateConfigsWithInvalidInput(t *testing.T) {
	dirPath := invalidTestdataDir + "/" + platform
	dirs, err := ioutil.ReadDir(dirPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		testName := d.Name()
		t.Run(testName, func(t *testing.T) {
			invalidInput := readFileContent(testName, invalidInputPath, t, false)
			expectedError := readFileContent(testName, goldenErrorPath, t, true)
			// The expected error could be triggered by:
			// 1. Parsing phase of the agent config when the config is not YAML.
			// 2. Config generation phase when the config is invalid.
			uc, actualError := ParseUnifiedConfig(invalidInput)
			if actualError == nil {
				actualError = generateConfigs(uc, defaultLogsDir, defaultStateDir)
			}
			if actualError == nil {
				t.Errorf("test %q: generateConfigs succeeded, want error:\n%s\ninvalid input:\n%s", testName, expectedError, invalidInput)
			} else {
				updateOrCompareGolden(t, testName, expectedError, actualError.Error(), goldenErrorPath)
			}
		})
	}
}

func generateConfigs(uc UnifiedConfig, defaultLogsDir string, defaultStateDir string) (err error) {
	if _, _, err := uc.GenerateFluentBitConfigs(defaultLogsDir, defaultStateDir, hostInfo); err != nil {
		return err
	}
	if platform == "windows" {
		if _, err := uc.GenerateOtelConfig(hostInfo); err != nil {
			return err
		}
	} else {
		if _, err := uc.GenerateCollectdConfig(defaultLogsDir); err != nil {
			return err
		}
	}
	return nil
}
