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
)

const (
	validTestdataDir         = "testdata/valid"
	invalidTestdataDir       = "testdata/invalid"
	defaultGoldenPath        = "default_config"
	defaultWindowsGoldenPath = "windows_default_config"
	defaultLogsDir           = "/var/log/google-cloud-ops-agent/subagents"
	defaultStateDir          = "/var/lib/google-cloud-ops-agent/fluent-bit"
)

var (
	// Usage:
	//   ops-agent$ go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator -update_golden
	// Add "-v" to show details for which files are updated with what:
	//   ops-agent$ go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator -update_golden -v
	updateGolden                     = flag.Bool("update_golden", false, "Whether to update the expected golden confs if they differ from the actual generated confs.")
	validUnifiedConfigFilePathFormat = validTestdataDir + "/%s/input.yaml"
	goldenMainPath                   = validTestdataDir + "/%s/golden_fluent_bit_main.conf"
	goldenParserPath                 = validTestdataDir + "/%s/golden_fluent_bit_parser.conf"
	goldenCollectdPath               = validTestdataDir + "/%s/golden_collectd.conf"
	goldenOtelPath                   = validTestdataDir + "/%s/golden_otel.conf"
	invalidTestdataFilePathFormat    = invalidTestdataDir + "/%s"
)

func TestGenerateConfsWithValidInput(t *testing.T) {
	dirPath := validTestdataDir
	dirs, err := ioutil.ReadDir(dirPath)
	if err != nil {
		t.Fatal(err)
	}

	for _, d := range dirs {
		testName := d.Name()
		t.Run(testName, func(t *testing.T) {
			unifiedConfigFilePath := fmt.Sprintf(validUnifiedConfigFilePathFormat, testName)
			// Special-case the default config.  It lives directly in the
			// confgenerator directory.  The golden files are still in the
			// testdata directory.
			if d.Name() == "default_config" {
				unifiedConfigFilePath = "default-config.yaml"
			} else if d.Name() == "windows_default_config" {
				unifiedConfigFilePath = "windows-default-config.yaml"
			}

			unifiedConfig, err := ioutil.ReadFile(unifiedConfigFilePath)
			if err != nil {
				t.Fatalf("test %q: expect no error, get error %s", testName, err)
			}

			isWindows := strings.Contains(d.Name(), "windows")

			// Retrieve the expected golden conf files.
			expectedMainConfig := expectedConfig(testName, goldenMainPath, t, isWindows)
			expectedParserConfig := expectedConfig(testName, goldenParserPath, t, isWindows)
			// Generate the actual conf files.
			mainConf, parserConf, err := GenerateFluentBitConfigs(unifiedConfig, defaultLogsDir, defaultStateDir)
			if err != nil {
				t.Fatalf("test %q: expect no error, got error running GenerateFluentBitConfigs : %s", testName, err)
			}
			// Compare the expected and actual and error out in case of diff.
			updateOrCompareGolden(testName, expectedMainConfig, mainConf, goldenMainPath, t)
			updateOrCompareGolden(testName, expectedParserConfig, parserConf, goldenParserPath, t)

			if isWindows {
				expectedOtelConfig := expectedConfig(testName, goldenOtelPath, t, true)
				otelConf, err := GenerateOtelConfig(unifiedConfig)
				if err != nil {
					t.Fatalf("test %q: expect no error, get error running GenerateOtelConfig : %s", testName, err)
				}
				// Compare the expected and actual and error out in case of diff.
				updateOrCompareGolden(testName, expectedOtelConfig, otelConf, goldenOtelPath, t)
			} else {
				expectedCollectdConfig := expectedConfig(testName, goldenCollectdPath, t, false)
				collectdConf, err := GenerateCollectdConfig(unifiedConfig, defaultLogsDir)
				if err != nil {
					t.Fatalf("test %q: expect no error, get error running GenerateCollectdConfig : %s", testName, err)
				}
				// Compare the expected and actual and error out in case of diff.
				updateOrCompareGolden(testName, expectedCollectdConfig, collectdConf, goldenCollectdPath, t)
			}
		})
	}
}

func expectedConfig(testName string, validFilePathFormat string, t *testing.T, isWindows bool) string {
	goldenPath := fmt.Sprintf(validFilePathFormat, testName)
	var defaultPath string
	if isWindows {
		defaultPath = fmt.Sprintf(validFilePathFormat, defaultWindowsGoldenPath)
	} else {
		defaultPath = fmt.Sprintf(validFilePathFormat, defaultGoldenPath)
	}
	rawExpectedConfig, err := ioutil.ReadFile(goldenPath)
	if err != nil {
		t.Logf("test %q: Golden conf not detected at %s. Using the default at %s instead.", testName, goldenPath, defaultPath)
		if rawExpectedConfig, err = ioutil.ReadFile(defaultPath); err != nil {
			t.Fatalf("test %q: error reading the default golden conf from %s : %s", testName, defaultPath, err)
		}
	}
	return string(rawExpectedConfig)
}

func updateOrCompareGolden(testName string, expected string, actual string, path string, t *testing.T) {
	if diff := cmp.Diff(expected, actual); diff != "" {
		if *updateGolden {
			// Update the expected to match the actual.
			goldenPath := fmt.Sprintf(path, testName)
			t.Logf("test %q: Detected -update_golden flag. Rewriting the %q golden file to apply the following diff\n%s.", testName, goldenPath, diff)
			if err := ioutil.WriteFile(goldenPath, []byte(actual), 0644); err != nil {
				t.Fatalf("test %q: error updating golden file at %q : %s", testName, goldenPath, err)
			}
		} else {
			t.Fatalf("test %q: conf mismatch (-want +got):\n%s", testName, diff)
		}
	}
}

func TestGenerateConfigsWithInvalidInput(t *testing.T) {
	filePath := invalidTestdataDir
	files, err := ioutil.ReadDir(filePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		testName := f.Name()
		t.Run(testName, func(t *testing.T) {
			unifiedConfigFilePath := fmt.Sprintf(invalidTestdataFilePathFormat, testName)
			unifiedConfig, err := ioutil.ReadFile(unifiedConfigFilePath)
			if err != nil {
				t.Errorf("test %q: expect no error, get error %s", testName, err)
				return
			}
			// TODO(lingshi): Figure out some more robust way to distinguish logging and metrics.
			if strings.HasPrefix(testName, "all-") || strings.HasPrefix(testName, "logging-") {
				if _, _, err := GenerateFluentBitConfigs(unifiedConfig, defaultLogsDir, defaultStateDir); err == nil {
					t.Errorf("test %q: GenerateFluentBitConfigs succeeded, want error. file:\n%s", testName, unifiedConfig)
				}
			} else if strings.HasPrefix(testName, "all-") || strings.HasPrefix(testName, "metrics-") {
				if _, err := GenerateCollectdConfig(unifiedConfig, defaultLogsDir); err == nil {
					t.Errorf("test %q: GenerateCollectdConfig succeeded, want error. file:\n%s", testName, unifiedConfig)
				}
			} else {
				t.Errorf("test %q: Unsupported test type. Must start with 'logging-' or 'metrics-'.", testName)
			}
		})
	}
}
