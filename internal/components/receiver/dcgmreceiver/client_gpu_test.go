// Copyright 2023 Google LLC
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

// Note: DCGM daemon needs to be running for all GPU tests

//go:build gpu && has_gpu
// +build gpu,has_gpu

package dcgmreceiver

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v3"
	"gotest.tools/v3/golden"
)

const testdataDir = "testdata"

// modelSupportedFields can be used to track supported fields for a given GPU
type modelSupportedFields struct {
	// The model of the GPU device, for example, Tesla P4
	Model string `yaml:"model"`
	// List of supported fields
	SupportedFields []string `yaml:"supported_fields"`
	// List of unsupported fields
	UnsupportedFields []string `yaml:"unsupported_fields"`
}

func defaultClientSettings() *dcgmClientSettings {
	requestedFields := discoverRequestedFields(createDefaultConfig().(*Config))
	return &dcgmClientSettings{
		endpoint:         defaultEndpoint,
		pollingInterval:  1 * time.Second,
		retryBlankValues: true,
		maxRetries:       5,
		fields:           requestedFields,
	}
}

// TestSupportedProfilingFieldsWithGolden tests getSupportedRegularFields() and
// getSupportedProfilingFields() against the golden files for the current GPU
// model
func TestSupportedProfilingFieldsWithGolden(t *testing.T) {
	clientSettings := defaultClientSettings()
	client, err := newClient(clientSettings, zaptest.NewLogger(t))
	require.Nil(t, err, "cannot initialize DCGM. Install and run DCGM before running tests.")
	defer client.cleanup()

	allFields := toFieldIDs(clientSettings.fields)
	supportedProfilingFields, err := getSupportedProfilingFields()
	require.Nil(t, err)
	enabledFields, unavailableFields := filterSupportedFields(allFields, supportedProfilingFields)

	var enabledFieldsString []string
	var unavailableFieldsString []string
	for _, f := range enabledFields {
		name := dcgmIDToName[f]
		if !strings.HasPrefix(name, "DCGM_FI_DEV_") {
			enabledFieldsString = append(enabledFieldsString, name)
		}
	}
	for _, f := range unavailableFields {
		name := dcgmIDToName[f]
		if !strings.HasPrefix(name, "DCGM_FI_DEV_") {
			unavailableFieldsString = append(unavailableFieldsString, name)
		}
	}
	sort.Strings(enabledFieldsString)
	sort.Strings(unavailableFieldsString)
	_, err = client.collect()
	require.Nil(t, err)
	require.NotEmpty(t, client.devices)
	gpuModel := client.devices[0].ModelName

	want := LoadExpectedMetrics(t, gpuModel)
	want.SupportedFields = slices.DeleteFunc(want.SupportedFields, func(name string) bool {
		return strings.HasPrefix(name, "DCGM_FI_DEV_")
	})
	want.UnsupportedFields = slices.DeleteFunc(want.UnsupportedFields, func(name string) bool {
		return strings.HasPrefix(name, "DCGM_FI_DEV_")
	})
	assert.ElementsMatch(t, enabledFieldsString, want.SupportedFields, "supported profiling fields")
	assert.ElementsMatch(t, unavailableFieldsString, want.UnsupportedFields)
}

// LoadExpectedMetrics read the supported metrics of a GPU model from the golden
// file, given a GPU model string
func LoadExpectedMetrics(t *testing.T, model string) modelSupportedFields {
	t.Helper()
	goldenPath := getModelGoldenFilePath(t, model)
	goldenFile, err := ioutil.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	var m modelSupportedFields
	err = yaml.Unmarshal(goldenFile, &m)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// getModelGoldenFilePath returns golden file path given a GPU model string
func getModelGoldenFilePath(t *testing.T, model string) string {
	t.Helper()
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return path.Join(testDir, testdataDir, fmt.Sprintf("%s.yaml", strings.ReplaceAll(model, " ", "_")))
}

func TestNewDcgmClientWithGpuPresent(t *testing.T) {
	client, err := newClient(defaultClientSettings(), zaptest.NewLogger(t))
	require.Nil(t, err, "cannot initialize DCGM. Install and run DCGM before running tests.")

	assert.NotNil(t, client)
	assert.NotNil(t, client.handleCleanup)
	client.cleanup()
}

func TestCollectGpuProfilingMetrics(t *testing.T) {
	clientSettings := defaultClientSettings()
	client, err := newClient(clientSettings, zaptest.NewLogger(t))
	defer client.cleanup()
	require.Nil(t, err, "cannot initialize DCGM. Install and run DCGM before running tests.")
	var maxCollectionInterval = 60 * time.Second
	var before, after int64
	for {
		before = time.Now().UnixMicro() - maxCollectionInterval.Microseconds()
		duration, err := client.collect()
		after = time.Now().UnixMicro()
		assert.Greater(t, duration, time.Duration(0))
		assert.Nil(t, err)
		var metricCount int
		for _, device := range client.devices {
			for _, metric := range device.Metrics {
				if metric.lastFieldValue != nil {
					metricCount++
				}
			}
		}
		if metricCount > 0 {
			break
		}
		time.Sleep(client.pollingInterval)
	}
	deviceMetrics := client.devices

	lastFloat64 := func(metric *metricStats) float64 {
		assert.Equal(t, dcgm.DCGM_FT_DOUBLE, metric.lastFieldValue.FieldType, "Unexpected metric type: %+v", metric.lastFieldValue)
		value, ok := asFloat64(*metric.lastFieldValue)
		require.True(t, ok, "Unexpected metric type: %+v", metric.lastFieldValue)
		return value
	}
	lastInt64 := func(metric *metricStats) int64 {
		assert.Equal(t, dcgm.DCGM_FT_INT64, metric.lastFieldValue.FieldType, "Unexpected metric type: %+v", metric.lastFieldValue)
		value, ok := asInt64(*metric.lastFieldValue)
		require.True(t, ok, "Unexpected metric type: %+v", metric.lastFieldValue)
		return value
	}

	seenMetric := make(map[string]int)
	assert.GreaterOrEqual(t, len(deviceMetrics), 0)
	assert.LessOrEqual(t, len(deviceMetrics), 32)
	for _, device := range deviceMetrics {
		for name, metric := range device.Metrics {
			switch name {
			case "DCGM_FI_PROF_GR_ENGINE_ACTIVE":
				fallthrough
			case "DCGM_FI_PROF_SM_ACTIVE":
				fallthrough
			case "DCGM_FI_PROF_SM_OCCUPANCY":
				fallthrough
			case "DCGM_FI_PROF_PIPE_TENSOR_ACTIVE":
				fallthrough
			case "DCGM_FI_PROF_PIPE_FP64_ACTIVE":
				fallthrough
			case "DCGM_FI_PROF_PIPE_FP32_ACTIVE":
				fallthrough
			case "DCGM_FI_PROF_PIPE_FP16_ACTIVE":
				fallthrough
			case "DCGM_FI_PROF_DRAM_ACTIVE":
				value := lastFloat64(metric)
				assert.GreaterOrEqual(t, value, float64(0.0))
				assert.LessOrEqual(t, value, float64(1.0))
			case "DCGM_FI_DEV_GPU_UTIL":
				fallthrough
			case "DCGM_FI_DEV_MEM_COPY_UTIL":
				fallthrough
			case "DCGM_FI_DEV_ENC_UTIL":
				fallthrough
			case "DCGM_FI_DEV_DEC_UTIL":
				value := lastInt64(metric)
				assert.GreaterOrEqual(t, value, int64(0))
				assert.LessOrEqual(t, value, int64(100))
			case "DCGM_FI_DEV_FB_FREE":
				fallthrough
			case "DCGM_FI_DEV_FB_USED":
				fallthrough
			case "DCGM_FI_DEV_FB_RESERVED":
				// arbitrary max of 10 TiB
				value := lastInt64(metric)
				assert.GreaterOrEqual(t, value, int64(0))
				assert.LessOrEqual(t, value, int64(10485760))
			case "DCGM_FI_PROF_PCIE_TX_BYTES":
				fallthrough
			case "DCGM_FI_PROF_PCIE_RX_BYTES":
				fallthrough
			case "DCGM_FI_PROF_NVLINK_TX_BYTES":
				fallthrough
			case "DCGM_FI_PROF_NVLINK_RX_BYTES":
				// arbitrary max of 10 TiB/sec
				value := lastInt64(metric)
				assert.GreaterOrEqual(t, value, int64(0))
				assert.LessOrEqual(t, value, int64(10995116277760))
			case "DCGM_FI_DEV_BOARD_LIMIT_VIOLATION":
				fallthrough
			case "DCGM_FI_DEV_LOW_UTIL_VIOLATION":
				fallthrough
			case "DCGM_FI_DEV_POWER_VIOLATION":
				fallthrough
			case "DCGM_FI_DEV_RELIABILITY_VIOLATION":
				fallthrough
			case "DCGM_FI_DEV_SYNC_BOOST_VIOLATION":
				fallthrough
			case "DCGM_FI_DEV_THERMAL_VIOLATION":
				fallthrough
			case "DCGM_FI_DEV_TOTAL_APP_CLOCKS_VIOLATION":
				fallthrough
			case "DCGM_FI_DEV_TOTAL_BASE_CLOCKS_VIOLATION":
				value := lastInt64(metric)
				assert.GreaterOrEqual(t, value, int64(0))
				assert.LessOrEqual(t, value, time.Now().UnixNano(), name)
			case "DCGM_FI_DEV_ECC_DBE_VOL_TOTAL":
				fallthrough
			case "DCGM_FI_DEV_ECC_SBE_VOL_TOTAL":
				// arbitrary max of 100000000 errors
				value := lastInt64(metric)
				assert.GreaterOrEqual(t, value, int64(0))
				assert.LessOrEqual(t, value, int64(100000000))
			case "DCGM_FI_DEV_GPU_TEMP":
				// arbitrary max of 100000 °C
				value := lastInt64(metric)
				assert.GreaterOrEqual(t, value, int64(0))
				assert.LessOrEqual(t, value, int64(100000))
			case "DCGM_FI_DEV_SM_CLOCK":
				// arbitrary max of 100000 MHz
				value := lastInt64(metric)
				assert.GreaterOrEqual(t, value, int64(0))
				assert.LessOrEqual(t, value, int64(100000))
			case "DCGM_FI_DEV_TOTAL_ENERGY_CONSUMPTION":
				value := lastInt64(metric)
				assert.GreaterOrEqual(t, value, int64(0))
				// TODO
			case "DCGM_FI_DEV_POWER_USAGE":
				value := lastFloat64(metric)
				assert.GreaterOrEqual(t, value, float64(0.0))
				// TODO
			default:
				t.Errorf("Unexpected metric '%s'", name)
			}

			assert.GreaterOrEqual(t, metric.lastFieldValue.Ts, before)
			assert.LessOrEqual(t, metric.lastFieldValue.Ts, after)

			seenMetric[name]++
		}
	}

	for name, count := range seenMetric {
		assert.Equalf(t, count, len(deviceMetrics), "metric %q found on an unexpected number of GPUs", name)
	}

	allFields := clientSettings.fields

	var enabledFieldsString []string
	var unavailableFieldsString []string
	for _, f := range allFields {
		if seenMetric[f] > 0 {
			enabledFieldsString = append(enabledFieldsString, f)
		} else {
			unavailableFieldsString = append(unavailableFieldsString, f)
		}
	}
	sort.Strings(enabledFieldsString)
	sort.Strings(unavailableFieldsString)
	gpuModel := client.devices[0].ModelName
	m := modelSupportedFields{
		Model:             gpuModel,
		SupportedFields:   enabledFieldsString,
		UnsupportedFields: unavailableFieldsString,
	}
	actual, err := yaml.Marshal(&m)
	if err != nil {
		t.Fatal(err)
	}
	goldenPath := getModelGoldenFilePath(t, gpuModel)
	golden.Assert(t, string(actual), goldenPath)
}
