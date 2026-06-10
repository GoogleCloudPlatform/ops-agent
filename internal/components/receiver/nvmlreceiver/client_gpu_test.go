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
	"math"
	"os"
	"testing"
	"time"
	"unsafe"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/GoogleCloudPlatform/opentelemetry-operations-collector/components/otelopscol/receiver/nvmlreceiver/testcudakernel"
)

func TestNewNvmlClientWithGpuPresent(t *testing.T) {
	client, _ := newClient(createDefaultConfig().(*Config), zaptest.NewLogger(t))
	require.NotNil(t, client)
	assert.Equal(t, client.disable, false)
	assert.Greater(t, len(client.devices), 0)
}

func TestNewNvmlClientOnAccountingModeUnsupported(t *testing.T) {
	realNvmlDeviceSetAccountingMode := nvmlDeviceSetAccountingMode
	defer func() { nvmlDeviceSetAccountingMode = realNvmlDeviceSetAccountingMode }()
	nvmlDeviceSetAccountingMode = func(Device nvml.Device, Mode nvml.EnableState) nvml.Return {
		return nvml.ERROR_NOT_SUPPORTED
	}

	client, _ := newClient(createDefaultConfig().(*Config), zaptest.NewLogger(t))
	require.NotNil(t, client)
	require.Equal(t, client.disable, false)
	assert.Greater(t, len(client.devices), 0)

	for _, isAccountingEnabled := range client.deviceToAccountingIsEnabled {
		assert.Equal(t, isAccountingEnabled, false)
	}
}

func TestGpuModelNameExists(t *testing.T) {
	client, _ := newClient(createDefaultConfig().(*Config), zaptest.NewLogger(t))
	require.NotNil(t, client)
	require.Greater(t, len(client.devices), 0)

	for gpuIndex := 0; gpuIndex < len(client.devices); gpuIndex++ {
		model := client.getDeviceModelName(uint(gpuIndex))
		assert.GreaterOrEqual(t, len(model), 2)
	}
}

func TestCollectGpuUtilization(t *testing.T) {
	client, _ := newClient(createDefaultConfig().(*Config), zaptest.NewLogger(t))
	require.NotNil(t, client)

	before := time.Now()
	metrics := client.collectDeviceUtilization()
	after := time.Now()

	assert.GreaterOrEqual(t, len(metrics), 1)
	for _, metric := range metrics {
		assert.Equal(t, metric.name, "nvml.gpu.utilization")
		assert.GreaterOrEqual(t, metric.gpuIndex, uint(0))
		assert.LessOrEqual(t, metric.gpuIndex, uint(32))
		assert.GreaterOrEqual(t, metric.asFloat64(), 0.0)
		assert.LessOrEqual(t, metric.asFloat64(), 1.0)
		assert.GreaterOrEqual(t, metric.time, before)
		assert.LessOrEqual(t, metric.time, after)
	}
}

func TestCollectGpuMemoryUsed(t *testing.T) {
	client, _ := newClient(createDefaultConfig().(*Config), zaptest.NewLogger(t))
	require.NotNil(t, client)

	var requiredNames = map[string]bool{
		"nvml.gpu.memory.bytes_used": false,
		"nvml.gpu.memory.bytes_free": false,
	}

	before := time.Now()
	metrics := client.collectDeviceMemoryInfo()
	after := time.Now()

	assert.GreaterOrEqual(t, len(metrics), 2)
	for _, metric := range metrics {
		assert.Contains(t, requiredNames, metric.name)
		requiredNames[metric.name] = true
		assert.GreaterOrEqual(t, metric.gpuIndex, uint(0))
		assert.LessOrEqual(t, metric.gpuIndex, uint(32))
		assert.GreaterOrEqual(t, metric.asInt64(), int64(0))
		assert.LessOrEqual(t, metric.asInt64(), int64(10995116277760)) // 10 TiB
		assert.GreaterOrEqual(t, metric.time, before)
		assert.LessOrEqual(t, metric.time, after)
	}

	for _, seen := range requiredNames {
		assert.Equal(t, seen, true)
	}
}

func TestGpuUtilizationIsAveraged(t *testing.T) {
	client, _ := newClient(createDefaultConfig().(*Config), zaptest.NewLogger(t))
	require.NotNil(t, client)

	realNvmlGetSamples := nvmlDeviceGetSamples
	defer func() { nvmlDeviceGetSamples = realNvmlGetSamples }()
	nvmlDeviceGetSamples = func(
		device nvml.Device, _type nvml.SamplingType, LastSeenTimeStamp uint64) (nvml.ValueType, []nvml.Sample, nvml.Return) {
		sampleCount := 61
		samples := make([]nvml.Sample, sampleCount)
		for i, _ := range samples {
			x := float64(i) / float64(sampleCount) * math.Pi
			y := int64(100.0 * math.Sin(x) * math.Sin(x))
			*(*int64)(unsafe.Pointer(&samples[i].SampleValue[0])) = y
			samples[i].TimeStamp = uint64(time.Now().Unix())
		}

		return nvml.VALUE_TYPE_SIGNED_LONG_LONG, samples, nvml.SUCCESS
	}

	metrics := client.collectDeviceUtilization()
	require.GreaterOrEqual(t, len(metrics), 1)
	assert.InDelta(t, 0.5, metrics[0].asFloat64(), 0.01)
}

func TestNewNvmlClientWithGpuSupportsAccountingMode(t *testing.T) {
	logger := zaptest.NewLogger(t)
	logger.Sugar().Warnf("This test requires superuser privileges.")

	client, _ := newClient(createDefaultConfig().(*Config), logger)
	require.NotNil(t, client)
	assert.Equal(t, client.disable, false)
	assert.Greater(t, len(client.devices), 0)

	for _, isAccountingEnabled := range client.deviceToAccountingIsEnabled {
		assert.Equal(t, isAccountingEnabled, true)
	}
}

func TestCollectGpuProcessesAccounting(t *testing.T) {
	logger := zaptest.NewLogger(t)
	logger.Sugar().Warnf("This test requires superuser privileges.")

	client, _ := newClient(createDefaultConfig().(*Config), logger)
	require.NotNil(t, client)
	assert.Equal(t, client.disable, false)
	assert.Greater(t, len(client.devices), 0)

	for _, isAccountingEnabled := range client.deviceToAccountingIsEnabled {
		assert.Equal(t, isAccountingEnabled, true)
	}

	testcudakernel.SubmitCudaTestKernel()

	before := time.Now()
	metrics := client.collectProcessMetrics()
	after := time.Now()

	seenSelfPid := false
	for _, metric := range metrics {
		assert.GreaterOrEqual(t, metric.time, before)
		assert.LessOrEqual(t, metric.time, after)

		assert.GreaterOrEqual(t, metric.gpuIndex, uint(0))
		assert.LessOrEqual(t, metric.gpuIndex, uint(32))

		assert.GreaterOrEqual(t, metric.lifetimeGpuUtilization, uint64(0))
		assert.LessOrEqual(t, metric.lifetimeGpuUtilization, uint64(100))
		assert.GreaterOrEqual(t, metric.lifetimeGpuMaxMemory, uint64(0))
		assert.LessOrEqual(t, metric.lifetimeGpuMaxMemory, uint64(10995116277760))

		assert.GreaterOrEqual(t, metric.processPid, int(0))
		assert.LessOrEqual(t, metric.processPid, int(32768))
		assert.Greater(t, len(metric.processName), 0)
		assert.Greater(t, len(metric.command), 0)
		assert.Greater(t, len(metric.commandLine), 0)
		assert.Greater(t, len(metric.owner), 0)

		seenSelfPid = seenSelfPid || metric.processPid == os.Getpid()
	}

	assert.Equal(t, seenSelfPid, true)
}
