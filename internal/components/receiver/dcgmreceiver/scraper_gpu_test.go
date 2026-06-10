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

//go:build gpu && has_gpu
// +build gpu,has_gpu

package dcgmreceiver

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/NVIDIA/go-dcgm/pkg/dcgm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/scraper/scraperhelper"
	"go.uber.org/zap/zaptest"

	"github.com/GoogleCloudPlatform/opentelemetry-operations-collector/components/otelopscol/receiver/dcgmreceiver/internal/metadata"
	"github.com/GoogleCloudPlatform/opentelemetry-operations-collector/components/otelopscol/receiver/dcgmreceiver/testprofilepause"
)

func collectScraperResult(t *testing.T, ctx context.Context, scraper *dcgmScraper) (pmetric.Metrics, error) {
	for {
		metrics, err := scraper.scrape(ctx)
		assert.NoError(t, err)
		if metrics.MetricCount() > 0 {
			// We expect cumulative metrics to be missing on the first scrape.
			time.Sleep(scrapePollingInterval)
			return scraper.scrape(ctx)
		}
		time.Sleep(scrapePollingInterval)
	}
}

func TestScrapeWithGpuPresent(t *testing.T) {
	var settings receiver.Settings
	settings.Logger = zaptest.NewLogger(t)

	scraper := newDcgmScraper(createDefaultConfig().(*Config), settings)
	require.NotNil(t, scraper)

	err := scraper.start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	metrics, err := collectScraperResult(t, context.Background(), scraper)
	assert.NoError(t, err)

	assert.NoError(t, scraper.stop(context.Background()))

	validateScraperResult(t, metrics)
}

func TestScrapeCollectionInterval(t *testing.T) {
	var settings receiver.Settings
	settings.Logger = zaptest.NewLogger(t)

	var fetchCount int

	realDcgmGetValuesSince := dcgmGetValuesSince
	defer func() { dcgmGetValuesSince = realDcgmGetValuesSince }()
	dcgmGetValuesSince = func(g dcgm.GroupHandle, f dcgm.FieldHandle, t time.Time) ([]dcgm.FieldValue_v2, time.Time, error) {
		fetchCount++
		return realDcgmGetValuesSince(g, f, t)
	}

	scraper := newDcgmScraper(createDefaultConfig().(*Config), settings)
	require.NotNil(t, scraper)

	err := scraper.start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	// We expect to scrape every maxKeepSamples * scrapePollingInterval / 2.
	// Wait long enough that we expect three scrapes.
	const sleepTime = 3.5 * maxKeepSamples * scrapePollingInterval / 2

	time.Sleep(sleepTime)

	metrics, err := collectScraperResult(t, context.Background(), scraper)
	assert.NoError(t, err)

	assert.NoError(t, scraper.stop(context.Background()))

	// We should have seen 1 initial scrape + 3 timed scrapes + 2 scrapes triggered by `collectScraperResult`.
	assert.Less(t, fetchCount, 7, "too many fetches")

	validateScraperResult(t, metrics)
}

func TestScrapeWithDelayedDcgmService(t *testing.T) {
	realDcgmInit := dcgmInit
	defer func() { dcgmInit = realDcgmInit }()
	failures := 2
	dcgmInit = func(args ...string) (func(), error) {
		if failures > 0 {
			failures--
			return nil, fmt.Errorf("No DCGM client library *OR* No DCGM connection")
		}
		return realDcgmInit(args...)
	}

	var settings receiver.Settings
	settings.Logger = zaptest.NewLogger(t)

	scraper := newDcgmScraper(createDefaultConfig().(*Config), settings)
	require.NotNil(t, scraper)

	scraper.initRetryDelay = 0 // retry immediately

	err := scraper.start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	// Simulate DCGM becomes available after 3 attempts
	// scrape should block until DCGM is available
	metrics, err := collectScraperResult(t, context.Background(), scraper)
	assert.NoError(t, err)

	assert.NoError(t, scraper.stop(context.Background()))

	assert.Equal(t, 0, failures)

	validateScraperResult(t, metrics)
}

func TestScrapeWithEmptyMetricsConfig(t *testing.T) {
	var settings receiver.Settings
	settings.Logger = zaptest.NewLogger(t)
	emptyConfig := &Config{
		ControllerConfig: scraperhelper.ControllerConfig{
			CollectionInterval: defaultCollectionInterval,
		},
		TCPAddrConfig: confignet.TCPAddrConfig{
			Endpoint: defaultEndpoint,
		},
		Metrics: metadata.MetricsConfig{
			GpuDcgmClockFrequency: metadata.MetricConfig{
				Enabled: false,
			},
			GpuDcgmClockThrottleDurationTime: metadata.MetricConfig{
				Enabled: false,
			},
			GpuDcgmCodecDecoderUtilization: metadata.MetricConfig{
				Enabled: false,
			},
			GpuDcgmCodecEncoderUtilization: metadata.MetricConfig{
				Enabled: false,
			},
			GpuDcgmEccErrors: metadata.MetricConfig{
				Enabled: false,
			},
			GpuDcgmEnergyConsumption: metadata.MetricConfig{
				Enabled: false,
			},
			GpuDcgmMemoryBandwidthUtilization: metadata.MetricConfig{
				Enabled: false,
			},
			GpuDcgmMemoryBytesUsed: metadata.MetricConfig{
				Enabled: false,
			},
			GpuDcgmNvlinkIo: metadata.MetricConfig{
				Enabled: false,
			},
			GpuDcgmPcieIo: metadata.MetricConfig{
				Enabled: false,
			},
			GpuDcgmPipeUtilization: metadata.MetricConfig{
				Enabled: false,
			},
			GpuDcgmSmOccupancy: metadata.MetricConfig{
				Enabled: false,
			},
			GpuDcgmSmUtilization: metadata.MetricConfig{
				Enabled: false,
			},
			GpuDcgmTemperature: metadata.MetricConfig{
				Enabled: false,
			},
			GpuDcgmUtilization: metadata.MetricConfig{
				Enabled: false,
			},
			GpuDcgmXidErrors: metadata.MetricConfig{
				Enabled: false,
			},
		},
	}

	scraper := newDcgmScraper(emptyConfig, settings)
	require.NotNil(t, scraper)

	err := scraper.start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	metrics, err := scraper.scrape(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 0, metrics.MetricCount())

	assert.NoError(t, scraper.stop(context.Background()))
}

func TestScrapeOnPollingError(t *testing.T) {
	realDcgmGetValuesSince := dcgmGetValuesSince
	defer func() { dcgmGetValuesSince = realDcgmGetValuesSince }()
	dcgmGetValuesSince = func(_ dcgm.GroupHandle, _ dcgm.FieldHandle, _ time.Time) ([]dcgm.FieldValue_v2, time.Time, error) {
		return nil, time.Time{}, fmt.Errorf("DCGM polling error")
	}

	var settings receiver.Settings
	settings.Logger = zaptest.NewLogger(t)

	scraper := newDcgmScraper(createDefaultConfig().(*Config), settings)
	require.NotNil(t, scraper)

	err := scraper.start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	metrics, err := scraper.scrape(context.Background())

	assert.NoError(t, err)
	assert.Equal(t, 0, metrics.MetricCount())

	assert.NoError(t, scraper.stop(context.Background()))
}

func TestScrapeOnProfilingPaused(t *testing.T) {
	config := createDefaultConfig().(*Config)
	config.CollectionInterval = 10 * time.Millisecond

	var settings receiver.Settings
	settings.Logger = zaptest.NewLogger(t)

	scraper := newDcgmScraper(config, settings)
	require.NotNil(t, scraper)

	defer testprofilepause.ResumeProfilingMetrics(config.TCPAddrConfig.Endpoint)
	err := testprofilepause.PauseProfilingMetrics(config.TCPAddrConfig.Endpoint)
	if errors.Is(err, testprofilepause.FeatureNotSupportedError) {
		t.Skipf("Pausing profiling not supported")
	} else if err != nil {
		t.Fatalf("Pausing profiling failed with error %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	err = scraper.start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	metrics, err := collectScraperResult(t, context.Background(), scraper)

	assert.NoError(t, err)

	assert.NoError(t, scraper.stop(context.Background()))

	expectedMetrics := []string{
		"gpu.dcgm.utilization",
		"gpu.dcgm.codec.decoder.utilization",
		"gpu.dcgm.codec.encoder.utilization",
		"gpu.dcgm.memory.bytes_used",
		"gpu.dcgm.memory.bandwidth_utilization",
		"gpu.dcgm.energy_consumption",
		"gpu.dcgm.temperature",
		"gpu.dcgm.clock.frequency",
		"gpu.dcgm.clock.throttle_duration.time",
		"gpu.dcgm.ecc_errors",
	}

	require.Greater(t, metrics.ResourceMetrics().Len(), 0)

	ilms := metrics.ResourceMetrics().At(0).ScopeMetrics()
	require.Equal(t, 1, ilms.Len())

	ms := ilms.At(0).Metrics()
	metricWasSeen := make(map[string]bool)
	for i := 0; i < ms.Len(); i++ {
		metricWasSeen[ms.At(i).Name()] = true
	}

	for _, metric := range expectedMetrics {
		assert.True(t, metricWasSeen[metric], metric)
		delete(metricWasSeen, metric)
	}
	assert.Equal(t, len(expectedMetrics), ms.Len(), fmt.Sprintf("%v", metricWasSeen))
}

// loadExpectedScraperMetrics calls LoadExpectedMetrics to read the supported
// metrics from the golden file given a GPU model, and then convert the name
// from how they are defined in the dcgm client to scraper naming
func loadExpectedScraperMetrics(t *testing.T, model string) map[string]int {
	t.Helper()
	expectedMetrics := make(map[string]int)
	receiverMetricNameToScraperMetricName := map[string]string{
		"DCGM_FI_PROF_GR_ENGINE_ACTIVE": "gpu.dcgm.utilization",
		//"DCGM_FI_DEV_GPU_UTIL":          "gpu.dcgm.utilization",
		"DCGM_FI_PROF_SM_ACTIVE":          "gpu.dcgm.sm.utilization",
		"DCGM_FI_PROF_SM_OCCUPANCY":       "gpu.dcgm.sm.occupancy",
		"DCGM_FI_PROF_PIPE_TENSOR_ACTIVE": "gpu.dcgm.pipe.utilization",
		"DCGM_FI_PROF_PIPE_FP64_ACTIVE":   "gpu.dcgm.pipe.utilization",
		"DCGM_FI_PROF_PIPE_FP32_ACTIVE":   "gpu.dcgm.pipe.utilization",
		"DCGM_FI_PROF_PIPE_FP16_ACTIVE":   "gpu.dcgm.pipe.utilization",
		"DCGM_FI_DEV_ENC_UTIL":            "gpu.dcgm.codec.encoder.utilization",
		"DCGM_FI_DEV_DEC_UTIL":            "gpu.dcgm.codec.decoder.utilization",
		"DCGM_FI_DEV_FB_FREE":             "gpu.dcgm.memory.bytes_used",
		"DCGM_FI_DEV_FB_USED":             "gpu.dcgm.memory.bytes_used",
		"DCGM_FI_DEV_FB_RESERVED":         "gpu.dcgm.memory.bytes_used",
		"DCGM_FI_PROF_DRAM_ACTIVE":        "gpu.dcgm.memory.bandwidth_utilization",
		//"DCGM_FI_DEV_MEM_COPY_UTIL":               "gpu.dcgm.memory.bandwidth_utilization",
		"DCGM_FI_PROF_PCIE_TX_BYTES":           "gpu.dcgm.pcie.io",
		"DCGM_FI_PROF_PCIE_RX_BYTES":           "gpu.dcgm.pcie.io",
		"DCGM_FI_PROF_NVLINK_TX_BYTES":         "gpu.dcgm.nvlink.io",
		"DCGM_FI_PROF_NVLINK_RX_BYTES":         "gpu.dcgm.nvlink.io",
		"DCGM_FI_DEV_TOTAL_ENERGY_CONSUMPTION": "gpu.dcgm.energy_consumption",
		//"DCGM_FI_DEV_POWER_USAGE":                 "gpu.dcgm.energy_consumption",
		"DCGM_FI_DEV_GPU_TEMP":                    "gpu.dcgm.temperature",
		"DCGM_FI_DEV_SM_CLOCK":                    "gpu.dcgm.clock.frequency",
		"DCGM_FI_DEV_POWER_VIOLATION":             "gpu.dcgm.clock.throttle_duration.time",
		"DCGM_FI_DEV_THERMAL_VIOLATION":           "gpu.dcgm.clock.throttle_duration.time",
		"DCGM_FI_DEV_SYNC_BOOST_VIOLATION":        "gpu.dcgm.clock.throttle_duration.time",
		"DCGM_FI_DEV_BOARD_LIMIT_VIOLATION":       "gpu.dcgm.clock.throttle_duration.time",
		"DCGM_FI_DEV_LOW_UTIL_VIOLATION":          "gpu.dcgm.clock.throttle_duration.time",
		"DCGM_FI_DEV_RELIABILITY_VIOLATION":       "gpu.dcgm.clock.throttle_duration.time",
		"DCGM_FI_DEV_TOTAL_APP_CLOCKS_VIOLATION":  "gpu.dcgm.clock.throttle_duration.time",
		"DCGM_FI_DEV_TOTAL_BASE_CLOCKS_VIOLATION": "gpu.dcgm.clock.throttle_duration.time",
		"DCGM_FI_DEV_ECC_SBE_VOL_TOTAL":           "gpu.dcgm.ecc_errors",
		"DCGM_FI_DEV_ECC_DBE_VOL_TOTAL":           "gpu.dcgm.ecc_errors",
	}
	supportedFields := LoadExpectedMetrics(t, model)
	for _, em := range supportedFields.SupportedFields {
		scraperMetric := receiverMetricNameToScraperMetricName[em]
		if scraperMetric != "" {
			expectedMetrics[scraperMetric] += 1
		}
		// TODO: fallbacks.
	}
	return expectedMetrics
}

func validateScraperResult(t *testing.T, metrics pmetric.Metrics) {
	t.Helper()
	rms := metrics.ResourceMetrics()
	require.NotEmpty(t, rms.Len(), "missing ResourceMetrics")
	modelValue, ok := rms.At(0).Resource().Attributes().Get("gpu.model")
	require.True(t, ok, "missing gpu.model resource attribute")
	expectedMetrics := loadExpectedScraperMetrics(t, modelValue.Str())

	metricWasSeen := make(map[string]bool)
	expectedDataPointCount := 0
	for metric, expectedMetricDataPoints := range expectedMetrics {
		metricWasSeen[metric] = false
		expectedDataPointCount += expectedMetricDataPoints
	}

	assert.LessOrEqual(t, len(expectedMetrics), metrics.MetricCount(), "metric count")
	assert.LessOrEqual(t, expectedDataPointCount, metrics.DataPointCount(), "data point count")

	r := metrics.ResourceMetrics().At(0).Resource()
	assert.Contains(t, r.Attributes().AsRaw(), "gpu.number")
	assert.Contains(t, r.Attributes().AsRaw(), "gpu.uuid")
	assert.Contains(t, r.Attributes().AsRaw(), "gpu.model")

	ilms := metrics.ResourceMetrics().At(0).ScopeMetrics()
	require.Equal(t, 1, ilms.Len())

	ms := ilms.At(0).Metrics()
	for i := 0; i < ms.Len(); i++ {
		m := ms.At(i)
		var dps pmetric.NumberDataPointSlice

		switch m.Name() {
		case "gpu.dcgm.utilization":
			fallthrough
		case "gpu.dcgm.sm.utilization":
			fallthrough
		case "gpu.dcgm.sm.occupancy":
			fallthrough
		case "gpu.dcgm.pipe.utilization":
			fallthrough
		case "gpu.dcgm.codec.encoder.utilization":
			fallthrough
		case "gpu.dcgm.codec.decoder.utilization":
			fallthrough
		case "gpu.dcgm.memory.bytes_used":
			fallthrough
		case "gpu.dcgm.memory.bandwidth_utilization":
			fallthrough
		case "gpu.dcgm.temperature":
			fallthrough
		case "gpu.dcgm.clock.frequency":
			dps = m.Gauge().DataPoints()
		case "gpu.dcgm.energy_consumption":
			fallthrough
		case "gpu.dcgm.clock.throttle_duration.time":
			fallthrough
		case "gpu.dcgm.pcie.io":
			fallthrough
		case "gpu.dcgm.nvlink.io":
			fallthrough
		case "gpu.dcgm.ecc_errors":
			fallthrough
		case "gpu.dcgm.xid_errors":
			dps = m.Sum().DataPoints()
		default:
			t.Errorf("Unexpected metric %s", m.Name())
		}
		assert.LessOrEqual(t, expectedMetrics[m.Name()], dps.Len())

		switch m.Name() {
		case "gpu.dcgm.utilization":
		case "gpu.dcgm.sm.utilization":
		case "gpu.dcgm.sm.occupancy":
		case "gpu.dcgm.pipe.utilization":
			for j := 0; j < dps.Len(); j++ {
				assert.Contains(t, dps.At(j).Attributes().AsRaw(), "gpu.pipe")
			}
		case "gpu.dcgm.codec.encoder.utilization":
		case "gpu.dcgm.codec.decoder.utilization":
		case "gpu.dcgm.memory.bytes_used":
			for j := 0; j < dps.Len(); j++ {
				assert.Contains(t, dps.At(j).Attributes().AsRaw(), "gpu.memory.state")
			}
		case "gpu.dcgm.memory.bandwidth_utilization":
		case "gpu.dcgm.pcie.io":
			fallthrough
		case "gpu.dcgm.nvlink.io":
			for j := 0; j < dps.Len(); j++ {
				assert.Contains(t, dps.At(j).Attributes().AsRaw(), "network.io.direction")
			}
		case "gpu.dcgm.energy_consumption":
		case "gpu.dcgm.temperature":
		case "gpu.dcgm.clock.frequency":
		case "gpu.dcgm.clock.throttle_duration.time":
			for j := 0; j < dps.Len(); j++ {
				assert.Contains(t, dps.At(j).Attributes().AsRaw(), "gpu.clock.violation")
			}
		case "gpu.dcgm.ecc_errors":
			for j := 0; j < dps.Len(); j++ {
				assert.Contains(t, dps.At(j).Attributes().AsRaw(), "gpu.error.type")
			}
		// TODO
		//case "gpu.dcgm.xid_errors":
		//	for j := 0; j < dps.Len(); j++ {
		//		assert.Contains(t, dps.At(j).Attributes().AsRaw(), "gpu.error.xid")
		//	}
		default:
			t.Errorf("Unexpected metric %s", m.Name())
		}

		metricWasSeen[m.Name()] = true
	}

	for metric := range expectedMetrics {
		assert.True(t, metricWasSeen[metric], metric)
	}
}
