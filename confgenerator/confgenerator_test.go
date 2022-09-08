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
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/shirou/gopsutil/host"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/golden"
)

const (
	validTestdataDirName   = "valid"
	invalidTestdataDirName = "invalid"
	builtinTestdataDirName = "builtin"
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

	flbConfigGolden = "golden_" + fluentbit.MainConfigFileName
	flbParserGolden = "golden_" + fluentbit.ParserConfigFileName
	otelYamlGolden  = "golden_otel.yaml"
	errorGolden     = "golden_error"
)

func TestGoldens(t *testing.T) {
	t.Parallel()

	for _, platform := range platforms {
		t.Run(platform.OS, func(t *testing.T) {
			testPlatformGenerateConfTests(t, platform)
		})
	}
}

func testPlatformGenerateConfTests(t *testing.T, platform platformConfig) {
	t.Parallel()

	t.Run("builtin", func(t *testing.T) {
		t.Parallel()

		testDir := filepath.Join(builtinTestdataDirName, platform.OS)
		builtinConfBytes, _, err := confgenerator.MergeConfFiles(
			filepath.Join("testdata", testDir, inputFileName),
			platform.OS,
			apps.BuiltInConfStructs,
		)
		assert.NilError(t, err)
		golden.Assert(t, string(builtinConfBytes), filepath.Join(testDir, builtinConfigFileName))
		err = testGenerateConf(t, platform, testDir)
		assert.NilError(t, err)
	})

	t.Run("valid", func(t *testing.T) {
		t.Parallel()

		runTestsInDir(
			t,
			platform,
			validTestdataDirName,
			func(t *testing.T, err error, _ string) {
				assert.NilError(t, err)
			},
		)
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()

		runTestsInDir(
			t,
			platform,
			invalidTestdataDirName,
			func(t *testing.T, err error, testDir string) {
				assert.Assert(t, err != nil, "expected test config to be invalid, but was successful")
				goldenErrorPath := filepath.Join(testDir, errorGolden)
				golden.Assert(t, err.Error(), goldenErrorPath)
			},
		)
	})
}

func runTestsInDir(
	t *testing.T,
	platform platformConfig,
	testTypeDir string,
	errAssertion func(*testing.T, error, string),
) {
	platformTestDir := filepath.Join(testTypeDir, platform.OS)
	testNames := getTestsInDir(t, platformTestDir)

	for _, testName := range testNames {
		// https://github.com/golang/go/wiki/CommonMistakes#using-goroutines-on-loop-iterator-variables
		testName := testName
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			testDir := filepath.Join(platformTestDir, testName)
			err := testGenerateConf(t, platform, testDir)
			errAssertion(t, err, testDir)
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
		testNames = append(testNames, testDirEntry.Name())
	}
	return testNames
}

func testGenerateConf(t *testing.T, platform platformConfig, testDir string) error {
	// Merge Config
	_, confBytes, err := confgenerator.MergeConfFiles(
		filepath.Join("testdata", testDir, inputFileName),
		platform.OS,
		apps.BuiltInConfStructs,
	)
	if err != nil {
		return err
	}
	uc, err := confgenerator.ParseUnifiedConfigAndValidate(confBytes, platform.OS)
	if err != nil {
		return err
	}

	// Fluent Bit configs
	flbGeneratedConfigs, err := uc.GenerateFluentBitConfigs(
		platform.defaultLogsDir,
		platform.defaultStateDir,
		platform.InfoStat,
	)
	if err != nil {
		return err
	}
	golden.Assert(
		t,
		flbGeneratedConfigs[fluentbit.MainConfigFileName],
		filepath.Join(testDir, flbConfigGolden),
	)
	golden.Assert(
		t,
		flbGeneratedConfigs[fluentbit.ParserConfigFileName],
		filepath.Join(testDir, flbParserGolden),
	)
	err = testGeneratedLuaFiles(t, flbGeneratedConfigs, testDir)
	if err != nil {
		// This error shouldn't result in a golden_error.
		// An error here represents a failure to read something
		// in the filesystem, and is an error state for the test.
		t.Errorf("Testing generated lua files failed: %v", err)
	}

	// Otel configs
	otelGeneratedConfig, err := uc.GenerateOtelConfig(platform.InfoStat)
	if err != nil {
		return err
	}
	golden.Assert(
		t,
		otelGeneratedConfig,
		filepath.Join(testDir, otelYamlGolden),
	)

	return nil
}

func testGeneratedLuaFiles(t *testing.T, generatedFiles map[string]string, testDir string) error {
	// Find all lua files currently in this test directory
	existingLuaFiles := map[string]struct{}{}
	err := filepath.Walk(
		filepath.Join("testdata", testDir),
		func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if filepath.Ext(info.Name()) == ".lua" {
				existingLuaFiles[info.Name()] = struct{}{}
			}
			return nil
		},
	)
	if err != nil {
		return err
	}

	// Assert the goldens of all the generated files. Either the generated file
	// matches a file already present in the directory, or the lua file is new.
	// If the lua file is new, the test will fail if not currently doing a golden
	// update (`-update` flag).
	for file, content := range generatedFiles {
		if filepath.Ext(file) != ".lua" {
			continue
		}
		golden.Assert(t, content, filepath.Join(testDir, file))
		delete(existingLuaFiles, file)
	}

	// If there are any files left in the existing file map, then that means the
	// test generated new files and we're currently in an update run. We now need
	// to clean up the existing lua files left aren't being generated anymore.
	for file := range existingLuaFiles {
		err := os.Remove(filepath.Join("testdata", testDir, file))
		if err != nil {
			return err
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
