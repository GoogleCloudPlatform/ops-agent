// Copyright 2022 Google LLC
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
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
	"github.com/goccy/go-yaml"
	"github.com/shirou/gopsutil/host"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/golden"
)

const (
	validTestdataDirName   = "valid"
	invalidTestdataDirName = "invalid"
	builtinTestdataDirName = "builtin"
	goldenDir              = "golden"
	errorGolden            = goldenDir + "/error"
	inputFileName          = "input.yaml"
	builtinConfigFileName  = "builtin_conf.yaml"
)

type platformConfig struct {
	defaultLogsDir  string
	defaultStateDir string
	*host.InfoStat
}

var (
	platforms = []platformConfig{
		{
			defaultLogsDir:  "/var/log/google-cloud-ops-agent/subagents",
			defaultStateDir: "/var/lib/google-cloud-ops-agent/fluent-bit",
			InfoStat: &host.InfoStat{
				OS:              "linux",
				Platform:        "linux_platform",
				PlatformVersion: "linux_platform_version",
			},
		},
		{
			defaultLogsDir:  `C:\ProgramData\Google\Cloud Operations\Ops Agent\log`,
			defaultStateDir: `C:\ProgramData\Google\Cloud Operations\Ops Agent\run`,
			InfoStat: &host.InfoStat{
				OS:              "windows",
				Platform:        "win_platform",
				PlatformVersion: "win_platform_version",
			},
		},
	}
)

func TestGoldens(t *testing.T) {
	t.Parallel()

	// Iterate platforms inside test directories so the test name hierarchy matches the testdata hierarchy.
	for _, test := range []struct {
		dirName      string
		errAssertion func(t *testing.T, err error, got map[string]string)
	}{
		{
			validTestdataDirName,
			func(t *testing.T, err error, got map[string]string) {
				assert.NilError(t, err)
				delete(got, builtinConfigFileName)
			},
		},
		{
			invalidTestdataDirName,
			func(t *testing.T, err error, got map[string]string) {
				assert.Assert(t, err != nil, "expected test config to be invalid, but was successful")
				// Error is checked by runTestsInDir
				delete(got, builtinConfigFileName)
			},
		},
		{
			builtinTestdataDirName,
			nil,
		},
	} {
		test := test
		t.Run(test.dirName, func(t *testing.T) {
			t.Parallel()
			for _, platform := range platforms {
				platform := platform
				t.Run(platform.OS, func(t *testing.T) {
					t.Parallel()
					runTestsInDir(
						t,
						platform,
						test.dirName,
						test.errAssertion,
					)
				})
			}
		})
	}
}

func runTestsInDir(
	t *testing.T,
	platform platformConfig,
	testTypeDir string,
	errAssertion func(*testing.T, error, map[string]string),
) {
	platformTestDir := filepath.Join(testTypeDir, platform.OS)
	testNames := getTestsInDir(t, platformTestDir)

	for _, testName := range testNames {
		// https://github.com/golang/go/wiki/CommonMistakes#using-goroutines-on-loop-iterator-variables
		testName := testName
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			testDir := filepath.Join(platformTestDir, testName)
			got, err := generateConfigs(platform, testDir)
			if errAssertion != nil {
				errAssertion(t, err, got)
			}
			if err := testGeneratedFiles(t, got, testDir); err != nil {
				t.Errorf("Failed to check generated configs: %v", err)
			}
		})
	}
}

func getTestsInDir(t *testing.T, testDir string) []string {
	t.Helper()

	testdataDir := filepath.Join("testdata", testDir)
	testDirEntries, err := os.ReadDir(testdataDir)
	assert.NilError(t, err, "couldn't read directory %s: %v", testdataDir, err)
	testNames := []string{}
	for _, testDirEntry := range testDirEntries {
		if !testDirEntry.IsDir() {
			continue
		}
		userSpecifiedConfPath := filepath.Join(testdataDir, testDirEntry.Name(), inputFileName)
		if _, err := os.Stat(userSpecifiedConfPath + ".missing"); err == nil {
			// Intentionally missing
		} else if _, err := os.Stat(userSpecifiedConfPath); errors.Is(err, os.ErrNotExist) {
			// Empty directory; probably a leftover with backup files.
			continue
		}
		testNames = append(testNames, testDirEntry.Name())
	}
	return testNames
}

func generateConfigs(platform platformConfig, testDir string) (got map[string]string, err error) {
	got = make(map[string]string)
	defer func() {
		if err != nil {
			got["error"] = err.Error()
		}
	}()

	uc, err := confgenerator.MergeConfFiles(
		filepath.Join("testdata", testDir, inputFileName),
		platform.OS,
		apps.BuiltInConfStructs,
	)
	if err != nil {
		return
	}
	got[builtinConfigFileName] = apps.BuiltInConfStructs[platform.OS].String()

	// Fluent Bit configs
	flbGeneratedConfigs, err := uc.GenerateFluentBitConfigs(
		platform.defaultLogsDir,
		platform.defaultStateDir,
		platform.InfoStat,
	)
	for k, v := range flbGeneratedConfigs {
		got[k] = v
	}
	if err != nil {
		return
	}

	// Otel configs
	otelGeneratedConfig, err := uc.GenerateOtelConfig(platform.InfoStat)
	if err != nil {
		return
	}
	got["otel.yaml"] = otelGeneratedConfig

	inputBytes, err := os.ReadFile(filepath.Join("testdata", testDir, inputFileName))

	userConf, err := confgenerator.UnmarshalYamlToUnifiedConfig(inputBytes, platform.OS)
	if err != nil {
		return
	}

	// Feature Tracking
	extractedFeatures, err := confgenerator.ExtractFeatures(userConf)
	if err != nil {
		return
	}

	type featureMetadata struct {
		Module  string
		Feature string
		Key     string
		Value   string
	}

	features := make([]*featureMetadata, 0)
	for _, f := range extractedFeatures {
		fm := featureMetadata{
			Module:  f.Module,
			Feature: fmt.Sprintf("%s:%s", f.Kind, f.Type),
			Key:     strings.Join(f.Key, "."),
			Value:   f.Value,
		}
		features = append(features, &fm)
	}
	featureBytes, err := yaml.Marshal(&features)

	got["features.yaml"] = string(featureBytes)
	return
}

func testGeneratedFiles(t *testing.T, generatedFiles map[string]string, testDir string) error {
	// Find all files currently in this test directory
	existingFiles := map[string]struct{}{}
	goldenPath := filepath.Join("testdata", testDir, goldenDir)
	err := filepath.Walk(
		goldenPath,
		func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				existingFiles[info.Name()] = struct{}{}
			}
			return nil
		},
	)
	if golden.FlagUpdate() && os.IsNotExist(err) {
		if err := os.Mkdir(goldenPath, 0777); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// Assert the goldens of all the generated files. Either the generated file
	// matches a file already present in the directory, or the file is new.
	// If the file is new, the test will fail if not currently doing a golden
	// update (`-update` flag).
	for file, content := range generatedFiles {
		golden.Assert(t, content, filepath.Join(testDir, goldenDir, file))
		delete(existingFiles, file)
	}

	// If there are any files left in the existing file map, then that means the
	// test generated new files and we're currently in an update run. We now need
	// to clean up the existing lua files left aren't being generated anymore.
	for file := range existingFiles {
		if golden.FlagUpdate() {
			err := os.Remove(filepath.Join("testdata", testDir, goldenDir, file))
			if err != nil {
				return err
			}
		} else {
			t.Errorf("unexpected existing file: %q", file)
		}
	}

	return nil
}

func TestMain(m *testing.M) {
	// Hardcode the path to the JMX JAR to make tests repeatable.
	confgenerator.FindJarPath = func() (string, error) {
		return "/path/to/executables/opentelemetry-java-contrib-jmx-metrics.jar", nil
	}
	os.Exit(m.Run())
}

func init() {
	testResource := resourcedetector.GCEResource{
		Project:       "test-project",
		Zone:          "test-zone",
		Network:       "test-network",
		Subnetwork:    "test-subnetwork",
		PublicIP:      "test-public-ip",
		PrivateIP:     "test-private-ip",
		InstanceID:    "test-instance-id",
		InstanceName:  "test-instance-name",
		Tags:          "test-tag",
		MachineType:   "test-machine-type",
		Metadata:      map[string]string{"test-key": "test-value"},
		Label:         map[string]string{"test-label-key": "test-label-value"},
		InterfaceIPv4: map[string]string{"test-interface": "test-interface-ipv4"},
	}

	// Set up the test environment with mocked data.
	confgenerator.MetadataResource = testResource

	// Enable experimental features.
	os.Setenv("EXPERIMENTAL_FEATURES", "otlp_receiver")
}
