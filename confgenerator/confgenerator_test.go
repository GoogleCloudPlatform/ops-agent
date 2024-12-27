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
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp"
	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
	"github.com/goccy/go-yaml"
	"github.com/shirou/gopsutil/host"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/golden"
)

const (
	builtinTestdataDirName = "builtin"
	goldenDir              = "golden"
	errorGolden            = goldenDir + "/error"
	inputFileName          = "input.yaml"
	builtinConfigFileName  = "builtin_conf.yaml"
)

type platformConfig struct {
	name            string
	defaultLogsDir  string
	defaultStateDir string
	platform        platform.Platform
}

var winlogv1channels = []string{
	"Application",
	"Security",
	"Setup",
	"System",
}

var (
	// Set up the test environment with mocked data.
	testResource = resourcedetector.GCEResource{
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
		Metadata:      map[string]string{"test-key": "test-value", "test-escape": "$foo", "test-escape-parentheses": "${foo:bar}"},
		Label:         map[string]string{"test-label-key": "test-label-value"},
		InterfaceIPv4: map[string]string{"test-interface": "test-interface-ipv4"},
		ManagedInstanceGroup: gcp.ManagedInstanceGroup{
			Name:     "test-mig",
			Type:     gcp.Zone,
			Location: "test-zone",
		},
	}
	linuxTestPlatform = platformConfig{
		name:            "linux",
		defaultLogsDir:  "/var/log/google-cloud-ops-agent",
		defaultStateDir: "/var/lib/google-cloud-ops-agent/fluent-bit",
		platform: platform.Platform{
			Type: platform.Linux,
			HostInfo: &host.InfoStat{
				OS:              "linux",
				Platform:        "linux_platform",
				PlatformVersion: "linux_platform_version",
			},
			TestGCEResourceOverride: testResource,
		},
	}
	testPlatforms = []platformConfig{
		linuxTestPlatform,
		{
			name:            "linux-gpu",
			defaultLogsDir:  "/var/log/google-cloud-ops-agent",
			defaultStateDir: "/var/lib/google-cloud-ops-agent/fluent-bit",
			platform: platform.Platform{
				Type: platform.Linux,
				HostInfo: &host.InfoStat{
					OS:              "linux",
					Platform:        "linux_platform",
					PlatformVersion: "linux_platform_version",
				},
				TestGCEResourceOverride: testResource,
				HasNvidiaGpu:            true,
			},
		},
		{
			name:            "windows",
			defaultLogsDir:  `C:\ProgramData\Google\Cloud Operations\Ops Agent\log`,
			defaultStateDir: `C:\ProgramData\Google\Cloud Operations\Ops Agent\run`,
			platform: platform.Platform{
				Type:               platform.Windows,
				WindowsBuildNumber: "1", // Is2012 == false, Is2016 == false
				WinlogV1Channels:   winlogv1channels,
				HostInfo: &host.InfoStat{
					OS:              "windows",
					Platform:        "win_platform",
					PlatformVersion: "win_platform_version",
				},
				TestGCEResourceOverride: testResource,
			},
		},
		{
			name:            "windows-2012",
			defaultLogsDir:  `C:\ProgramData\Google\Cloud Operations\Ops Agent\log`,
			defaultStateDir: `C:\ProgramData\Google\Cloud Operations\Ops Agent\run`,
			platform: platform.Platform{
				Type:               platform.Windows,
				WindowsBuildNumber: "9200", // Windows Server 2012
				WinlogV1Channels:   winlogv1channels,
				HostInfo: &host.InfoStat{
					OS:              "windows",
					Platform:        "win_platform",
					PlatformVersion: "win_platform_version",
				},
				TestGCEResourceOverride: testResource,
			},
		},
	}
)

func TestGoldens(t *testing.T) {
	t.Parallel()

	goldensDir := "goldens"
	testNames := getTestsInDir(t, goldensDir)

	for _, testName := range testNames {
		// https://github.com/golang/go/wiki/CommonMistakes#using-goroutines-on-loop-iterator-variables
		testName := testName
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			for _, pc := range testPlatforms {
				pc := pc
				t.Run(pc.name, func(t *testing.T) {
					testDir := filepath.Join(goldensDir, testName)
					got, err := generateConfigs(pc, testDir)
					if strings.HasPrefix(testName, "invalid-") {
						assert.Assert(t, err != nil, "expected test config to be invalid, but was successful")
					}
					if testName != "builtin" {
						delete(got, builtinConfigFileName)
					}
					if err := testGeneratedFiles(t, got, filepath.Join(testDir, goldenDir, pc.name)); err != nil {
						t.Errorf("Failed to check generated configs: %v", err)
					}
				})
			}
		})
	}
}

func TestDataprocDefaults(t *testing.T) {
	t.Parallel()

	goldensDir := "goldens"
	testName := "builtin"
	dataprocMetadata := map[string]string{
		"dataproc-cluster-name": "test-cluster",
		"dataproc-cluster-uuid": "test-uuid",
		"dataproc-region":       "test-region",
	}

	t.Run(testName, func(t *testing.T) {
		t.Parallel()
		pc := linuxTestPlatform
		// Update mocked resource to include Dataproc labels.
		dataprocResource := testResource
		newMetadata := map[string]string{}
		for k, v := range testResource.Metadata {
			newMetadata[k] = v
		}
		for k, v := range dataprocMetadata {
			newMetadata[k] = v
		}
		dataprocResource.Metadata = newMetadata
		pc.platform.TestGCEResourceOverride = dataprocResource
		t.Run(pc.name, func(t *testing.T) {
			testDir := filepath.Join(goldensDir, testName)
			got, err := generateConfigs(pc, testDir)
			assert.NilError(t, err, "Failed to generate configs: %v", err)
			if err := testGeneratedFiles(t, got, filepath.Join(testDir, goldenDir, "linux-dataproc")); err != nil {
				t.Errorf("Failed to check generated configs: %v", err)
			}
		})
	})
}

func getTestsInDir(t *testing.T, testDir string) []string {
	t.Helper()

	testdataDir := filepath.Join("testdata", testDir)
	testDirEntries, err := os.ReadDir(testdataDir)
	if os.IsNotExist(err) {
		// No tests for this combination.
		return nil
	}
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

func generateConfigs(pc platformConfig, testDir string) (got map[string]string, err error) {
	ctx := pc.platform.TestContext(context.Background())

	if features, err := os.ReadFile(filepath.Join("testdata", testDir, "EXPERIMENTAL_FEATURES")); err == nil {
		ctx = confgenerator.ContextWithExperiments(ctx, confgenerator.ParseExperimentalFeatures(string(features)))
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	got = make(map[string]string)
	defer func() {
		if err != nil {
			got["error"] = err.Error()
		}
	}()

	uc, err := confgenerator.MergeConfFiles(
		ctx,
		filepath.Join("testdata", testDir, inputFileName),
		apps.BuiltInConfStructs,
	)
	if err != nil {
		return
	}
	got[builtinConfigFileName] = apps.BuiltInConfStructs[pc.platform.Name()].String()

	// Fluent Bit configs
	flbGeneratedConfigs, err := uc.GenerateFluentBitConfigs(ctx,
		pc.defaultLogsDir,
		pc.defaultStateDir,
	)
	for k, v := range flbGeneratedConfigs {
		got[k] = v
	}
	if err != nil {
		return
	}

	// Otel configs
	otelGeneratedConfig, err := uc.GenerateOtelConfig(ctx)
	if err != nil {
		return
	}
	got["otel.yaml"] = otelGeneratedConfig

	inputBytes, err := os.ReadFile(filepath.Join("testdata", testDir, inputFileName))

	userConf, err := confgenerator.UnmarshalYamlToUnifiedConfig(ctx, inputBytes)
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
	goldenPath := filepath.Join("testdata", testDir)
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
		if err := os.MkdirAll(goldenPath, 0777); err != nil {
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
		golden.Assert(t, content, filepath.Join(testDir, file))
		delete(existingFiles, file)
	}

	// If there are any files left in the existing file map, then that means the
	// test generated new files and we're currently in an update run. We now need
	// to clean up the existing lua files left aren't being generated anymore.
	for file := range existingFiles {
		if golden.FlagUpdate() {
			err := os.Remove(filepath.Join("testdata", testDir, file))
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
