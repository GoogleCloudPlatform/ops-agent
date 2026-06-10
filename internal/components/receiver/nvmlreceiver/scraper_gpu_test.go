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

//go:build gpu && has_gpu
// +build gpu,has_gpu

package nvmlreceiver

import (
	"context"
	"strings"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"

	"github.com/GoogleCloudPlatform/opentelemetry-operations-collector/components/otelopscol/receiver/nvmlreceiver/testcudakernel"
)

func TestScrapeWithGpuPresent(t *testing.T) {
	scraper := newNvmlScraper(createDefaultConfig().(*Config), receiver.Settings{})
	require.NotNil(t, scraper)

	err := scraper.start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	metrics, err := scraper.scrape(context.Background())
	validateScraperResult(t, metrics, []string{"nvml.gpu.utilization", "nvml.gpu.memory.bytes_used"})
}

func TestScrapeOnGpuUtilizationUnsupported(t *testing.T) {
	realNvmlGetSamples := nvmlDeviceGetSamples
	defer func() { nvmlDeviceGetSamples = realNvmlGetSamples }()
	nvmlDeviceGetSamples = func(
		device nvml.Device, _type nvml.SamplingType, LastSeenTimeStamp uint64) (nvml.ValueType, []nvml.Sample, nvml.Return) {
		return nvml.VALUE_TYPE_SIGNED_LONG_LONG, nil, nvml.ERROR_NOT_SUPPORTED
	}

	scraper := newNvmlScraper(createDefaultConfig().(*Config), receiver.Settings{})
	require.NotNil(t, scraper)

	err := scraper.start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	metrics, err := scraper.scrape(context.Background())
	validateScraperResult(t, metrics, []string{"nvml.gpu.memory.bytes_used"})
}

func TestScrapeOnGpuMemoryInfoUnsupported(t *testing.T) {
	realNvmlDeviceGetMemoryInfo := nvmlDeviceGetMemoryInfo
	defer func() { nvmlDeviceGetMemoryInfo = realNvmlDeviceGetMemoryInfo }()
	nvmlDeviceGetMemoryInfo = func(device nvml.Device) (nvml.Memory, nvml.Return) {
		return nvml.Memory{}, nvml.ERROR_NOT_SUPPORTED
	}

	scraper := newNvmlScraper(createDefaultConfig().(*Config), receiver.Settings{})
	require.NotNil(t, scraper)

	err := scraper.start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	metrics, err := scraper.scrape(context.Background())
	validateScraperResult(t, metrics, []string{"nvml.gpu.utilization"})
}

func TestScrapeWithGpuProcessAccounting(t *testing.T) {
	logger := zaptest.NewLogger(t)
	logger.Sugar().Warnf("This test requires superuser privileges.")

	scraper := newNvmlScraper(createDefaultConfig().(*Config), receiver.Settings{})
	require.NotNil(t, scraper)

	err := scraper.start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	testcudakernel.SubmitCudaTestKernel()

	metrics, err := scraper.scrape(context.Background())
	validateScraperResult(t, metrics, []string{
		"nvml.gpu.utilization",
		"nvml.gpu.memory.bytes_used",
		"nvml.gpu.processes.utilization",
		"nvml.gpu.processes.max_bytes_used",
	})
}

func TestScrapeWithGpuProcessAccountingError(t *testing.T) {
	realNvmlDeviceGetAccountingPids := nvmlDeviceGetAccountingPids
	defer func() { nvmlDeviceGetAccountingPids = realNvmlDeviceGetAccountingPids }()
	nvmlDeviceGetAccountingPids = func(device nvml.Device) ([]int, nvml.Return) {
		return nil, nvml.ERROR_UNKNOWN
	}

	scraper := newNvmlScraper(createDefaultConfig().(*Config), receiver.Settings{})
	require.NotNil(t, scraper)

	err := scraper.start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	testcudakernel.SubmitCudaTestKernel()

	metrics, err := scraper.scrape(context.Background())
	validateScraperResult(t, metrics, []string{
		"nvml.gpu.utilization",
		"nvml.gpu.memory.bytes_used",
	})
}

func TestScrapeEmitsWarningsUptoThreshold(t *testing.T) {
	realNvmlGetSamples := nvmlDeviceGetSamples
	defer func() { nvmlDeviceGetSamples = realNvmlGetSamples }()
	nvmlDeviceGetSamples = func(
		device nvml.Device, _type nvml.SamplingType, LastSeenTimeStamp uint64) (nvml.ValueType, []nvml.Sample, nvml.Return) {
		return nvml.VALUE_TYPE_SIGNED_LONG_LONG, nil, nvml.ERROR_NOT_SUPPORTED
	}

	warnings := 0
	var settings receiver.Settings
	settings.Logger = zaptest.NewLogger(t, zaptest.WrapOptions(zap.Hooks(func(e zapcore.Entry) error {
		if e.Level == zap.WarnLevel && strings.Contains(e.Message, "Unable to query") {
			warnings = warnings + 1
		}
		return nil
	})))

	scraper := newNvmlScraper(createDefaultConfig().(*Config), settings)
	require.NotNil(t, scraper)

	err := scraper.start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	for i := 0; i < maxWarningsForFailedDeviceMetricQuery+10; i++ {
		scraper.scrape(context.Background())
	}

	require.Equal(t, warnings, maxWarningsForFailedDeviceMetricQuery)
}

func validateScraperResult(t *testing.T, metrics pmetric.Metrics, expectedMetrics []string) {
	expectedMetricToDataPointCount := map[string]int{
		"nvml.gpu.utilization":              1,
		"nvml.gpu.memory.bytes_used":        2,
		"nvml.gpu.processes.utilization":    1,
		"nvml.gpu.processes.max_bytes_used": 1,
	}

	metricWasSeen := make(map[string]bool)
	minExpectedDataPointCount := 0
	for _, s := range expectedMetrics {
		metricWasSeen[s] = false
		minExpectedDataPointCount += expectedMetricToDataPointCount[s]
	}

	assert.GreaterOrEqual(t, metrics.MetricCount(), len(expectedMetrics))
	assert.GreaterOrEqual(t, metrics.DataPointCount(), minExpectedDataPointCount)

	ilms := metrics.ResourceMetrics().At(0).ScopeMetrics()
	require.Equal(t, 1, ilms.Len())

	ms := ilms.At(0).Metrics()
	for i := 0; i < ms.Len(); i++ {
		m := ms.At(i)
		dps := m.Gauge().DataPoints()
		for j := 0; j < dps.Len(); j++ {
			assert.Regexp(t, ".*gpu_number:.*", dps.At(j).Attributes().AsRaw())
			assert.Regexp(t, ".*model:.*", dps.At(j).Attributes().AsRaw())
			assert.Regexp(t, ".*uuid:.*", dps.At(j).Attributes().AsRaw())
		}

		switch m.Name() {
		case "nvml.gpu.utilization":
			assert.Equal(t, expectedMetricToDataPointCount[m.Name()], dps.Len())
		case "nvml.gpu.memory.bytes_used":
			assert.Equal(t, expectedMetricToDataPointCount[m.Name()], dps.Len())
			for j := 0; j < dps.Len(); j++ {
				assert.Regexp(t, ".*memory_state:.*", dps.At(j).Attributes().AsRaw())
			}
		case "nvml.gpu.processes.utilization":
			fallthrough
		case "nvml.gpu.processes.max_bytes_used":
			assert.GreaterOrEqual(t, dps.Len(), expectedMetricToDataPointCount[m.Name()])
			for j := 0; j < dps.Len(); j++ {
				assert.Regexp(t, ".*pid:.*", dps.At(j).Attributes().AsRaw())
				assert.Regexp(t, ".*process:.*", dps.At(j).Attributes().AsRaw())
				assert.Regexp(t, ".*command:.*", dps.At(j).Attributes().AsRaw())
				assert.Regexp(t, ".*command_line:.*", dps.At(j).Attributes().AsRaw())
				assert.Regexp(t, ".*owner:.*", dps.At(j).Attributes().AsRaw())
			}
		default:
			t.Errorf("Unexpected metric %s", m.Name())
		}

		metricWasSeen[m.Name()] = true
	}

	for _, metric := range expectedMetrics {
		assert.Equal(t, metricWasSeen[metric], true)
	}
}
