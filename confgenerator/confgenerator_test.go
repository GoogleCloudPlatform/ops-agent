package confgenerator_test

import (
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
	inputFileName          = "input.yaml"
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

	t.Run("valid", func(t *testing.T) {
		t.Parallel()

		platformTestDir := filepath.Join(validTestdataDirName, platform.OS)
		testNames := getTestsInDir(t, platformTestDir)

		for _, testName := range testNames {
			t.Run(testName, func(t *testing.T) {
				testDir := filepath.Join(platformTestDir, testName)
				testGenerateConfValid(t, platform, testDir)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()

		platformTestDir := filepath.Join(invalidTestdataDirName, platform.OS)
		testNames := getTestsInDir(t, platformTestDir)

		for _, testName := range testNames {
			t.Run(testName, func(t *testing.T) {
				testDir := filepath.Join(platformTestDir, testName)
				testGenerateConfInvalid(t, platform, testDir)
			})
		}
	})
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

func testGenerateConfValid(
	t *testing.T,
	platform platformConfig,
	testDir string,
) {
	t.Parallel()

	_, confBytes, err := confgenerator.MergeConfFiles(
		filepath.Join("testdata", testDir, inputFileName),
		platform.OS,
		apps.BuiltInConfStructs,
	)
	assert.NilError(t, err, "expected to successfully merge config, failed with: %v", err)

	uc, err := confgenerator.ParseUnifiedConfigAndValidate(confBytes, platform.OS)
	assert.NilError(t, err, "expected config to parse, failed with: %v", err)

	flbGeneratedConfigs, err := uc.GenerateFluentBitConfigs(
		platform.defaultLogsDir,
		platform.defaultStateDir,
		platform.InfoStat,
	)
	assert.NilError(t, err, "expected generating fluent-bit config to pass, failed with: %v", err)
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

	otelGeneratedConfig, err := uc.GenerateOtelConfig(platform.InfoStat)
	assert.NilError(t, err, "expected generating otel config to pass, failed with: %v", err)
	golden.Assert(
		t,
		otelGeneratedConfig,
		filepath.Join(testDir, otelYamlGolden),
	)
}

func testGenerateConfInvalid(
	t *testing.T,
	platform platformConfig,
	testDir string,
) {
	t.Parallel()

	goldenErrorPath := filepath.Join(testDir, errorGolden)
	inputPath := filepath.Join("testdata", testDir, inputFileName)

	_, confBytes, err := confgenerator.MergeConfFiles(inputPath, platform.OS, apps.BuiltInConfStructs)
	if err != nil {
		golden.Assert(t, err.Error(), goldenErrorPath)
		return
	}

	uc, err := confgenerator.ParseUnifiedConfigAndValidate(confBytes, platform.OS)
	if err != nil {
		golden.Assert(t, err.Error(), goldenErrorPath)
		return
	}

	_, err = uc.GenerateFluentBitConfigs(
		platform.defaultLogsDir,
		platform.defaultStateDir,
		platform.InfoStat,
	)
	if err != nil {
		golden.Assert(t, err.Error(), goldenErrorPath)
		return
	}

	_, err = uc.GenerateOtelConfig(platform.InfoStat)
	if err != nil {
		golden.Assert(t, err.Error(), goldenErrorPath)
		return
	}

	t.Fatal("expected config to fail merge or validation")
}

func TestMain(m *testing.M) {
	// Hardcode the path to the JMX JAR to make tests repeatable.
	confgenerator.FindJarPath = func() (string, error) {
		return "/path/to/executables/opentelemetry-java-contrib-jmx-metrics.jar", nil
	}
	os.Exit(m.Run())
}
