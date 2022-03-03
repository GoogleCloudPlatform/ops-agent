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
	updateGolden = flag.Bool("update_golden", false, "Whether to update the expected golden confs if they differ from the actual generated confs.")
	goldenPrefix = "golden_"
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
	testGenerateConfs(t, validTestdataDir)
}

func TestGenerateConfsWithInvalidInput(t *testing.T) {
	testGenerateConfs(t, invalidTestdataDir)
}

func testGenerateConfs(t *testing.T, dir string) {
	t.Parallel()
	for _, platform := range platforms {
		platform := platform
		t.Run(platform.OS, func(t *testing.T) {
			t.Parallel()
			testGenerateConfsPlatform(t, dir, platform)
		})
	}
}

func testGenerateConfsPlatform(t *testing.T, dir string, platform platformConfig) {
	dirPath := filepath.Join(dir, platform.OS)
	dirs, err := ioutil.ReadDir(dirPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		testName := d.Name()
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			// Retrieve the expected golden conf files.
			expectedFiles := readFileContents(t, testName, platform.OS, dir)

			var got map[string]string

			confDebugFolder := filepath.Join(dirPath, testName)
			userSpecifiedConfPath := filepath.Join(confDebugFolder, "/input.yaml")
			builtInConfPath := filepath.Join(confDebugFolder, "/built-in-config.yaml")
			mergedConfPath := filepath.Join(confDebugFolder, "/merged-config.yaml")
			if err = confgenerator.MergeConfFiles(userSpecifiedConfPath, confDebugFolder, platform.OS, apps.BuiltInConfStructs); err != nil {
				// TODO: Move this inside generateConfigs when we can do MergeConfFiles in-memory
				if _, ok := expectedFiles["error"]; ok {
					got = map[string]string{
						"error": err.Error(),
					}
				} else {
					t.Fatalf("MergeConfFiles(%q, %q) got: %v", userSpecifiedConfPath, confDebugFolder, err)
				}
			}

			defer func() {
				// Ignore errors because we don't know if MergeConfFiles generated them.
				os.Remove(builtInConfPath)
				os.Remove(mergedConfPath)
			}()

			if got == nil {
				data, err := ioutil.ReadFile(mergedConfPath)
				if err != nil {
					t.Fatalf("ReadFile(%q) got: %v", userSpecifiedConfPath, err)
				}
				t.Logf("merged config:\n%s", data)

				// Generate the actual conf files.
				got, err = generateConfigs(data, platform)

				if err != nil {
					t.Logf("config generation returned %v", err)
				}

				if testName == builtInConfTestName {
					// TODO: Fetch this without writing to disk first.
					generatedBuiltInConfig, err := ioutil.ReadFile(builtInConfPath)
					if err != nil {
						t.Errorf("test %q: error reading %s: %v", testName, builtInConfPath, err)
					} else {
						got["built-in-config.yaml"] = string(generatedBuiltInConfig)
					}
				}
			}

			// Compare the expected and actual and error out in case of diff.
			for name, content := range got {
				updateOrCompareGolden(t, testName, platform.OS, dir, name, content, expectedFiles[name])
				delete(expectedFiles, name)
			}

			for f := range expectedFiles {
				t.Errorf("missing expected file %q", f)
			}
		})
	}
}

func readFileContents(t *testing.T, testName, goos, dir string) map[string]string {
	t.Helper()
	glob := fmt.Sprintf("%s/%s/%s/%s*", dir, goos, testName, goldenPrefix)
	matches, err := filepath.Glob(glob)
	if err != nil {
		// No configs found, let the caller worry about it.
		return nil
	}
	out := make(map[string]string)
	for _, name := range matches {
		contents, err := ioutil.ReadFile(name)
		if err != nil {
			t.Errorf("failed to read %q: %v", name, err)
			continue
		}
		str := strings.ReplaceAll(string(contents), "\r\n", "\n")
		out[strings.TrimPrefix(filepath.Base(name), goldenPrefix)] = str
	}
	return out
}

func updateOrCompareGolden(t *testing.T, testName, goos, dir, name, got, want string) {
	t.Helper()
	goldenPath := fmt.Sprintf("%s/%s/%s/%s%s", dir, goos, testName, goldenPrefix, name)
	diff := cmp.Diff(want, got)
	if *updateGolden {
		// If there is a diff, or if the actual is empty (it may be due to the file
		// not existing), write the golden file with the expected content.
		if diff != "" || got == "" {
			// Update the expected to match the actual.
			t.Logf("Detected -update_golden flag. Rewriting the %q golden file to apply the following diff\n%s.", goldenPath, diff)
			if err := ioutil.WriteFile(goldenPath, []byte(got), 0644); err != nil {
				t.Fatalf("error updating golden file at %q : %s", goldenPath, err)
			}
		}
	} else if diff != "" {
		t.Errorf("test %q: golden file at %s mismatch (-want +got):\n%s", testName, goldenPath, diff)
	}
}

// Generate all config files for a given config file input.
// If an error occurs, it will be reported as a file called "error".
// The expected error could be triggered by:
// 1. Parsing phase of the agent config when the config is not YAML.
// 2. Config generation phase when the config is invalid.
// If at any point, an error is generated, immediately return it for validation.
func generateConfigs(configInput []byte, platform platformConfig) (got map[string]string, err error) {
	got = make(map[string]string)
	uc, err := confgenerator.ParseUnifiedConfigAndValidate(configInput, platform.OS)
	if err != nil {
		got["error"] = err.Error()
		return
	}

	fbConfs, err := uc.GenerateFluentBitConfigs(platform.defaultLogsDir, platform.defaultStateDir, platform.InfoStat)
	if err != nil {
		got["error"] = err.Error()
		return
	}
	for k, v := range fbConfs {
		got[k] = v
	}
	otelConf, err := uc.GenerateOtelConfig(platform.InfoStat)
	if err != nil {
		got["error"] = err.Error()
		return
	} else {
		got["otel.conf"] = otelConf
	}

	return
}

func TestMain(m *testing.M) {
	// Hardcode the path to the JMX JAR to make tests repeatable.
	confgenerator.FindJarPath = func() (string, error) {
		return "/path/to/executables/opentelemetry-java-contrib-jmx-metrics.jar", nil
	}
	os.Exit(m.Run())
}
