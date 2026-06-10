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

//go:build gpu
// +build gpu

package nvmlreceiver

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/receiver"

	"github.com/GoogleCloudPlatform/opentelemetry-operations-collector/components/otelopscol/receiver/nvmlreceiver/internal/metadata"
)

type nvmlScraper struct {
	config   *Config
	settings receiver.Settings
	client   *nvmlClient
	mb       *metadata.MetricsBuilder
}

func newNvmlScraper(config *Config, settings receiver.Settings) *nvmlScraper {
	return &nvmlScraper{config: config, settings: settings}
}

func (s *nvmlScraper) start(_ context.Context, _ component.Host) error {
	var err error
	s.client, err = newClient(s.config, s.settings.Logger)
	if err != nil {
		return err
	}

	startTime := pcommon.NewTimestampFromTime(time.Now())
	mbConfig := metadata.DefaultMetricsBuilderConfig()
	mbConfig.Metrics = s.config.Metrics
	s.mb = metadata.NewMetricsBuilder(
		mbConfig, s.settings, metadata.WithStartTime(startTime))

	return nil
}

func (s *nvmlScraper) stop(_ context.Context) error {
	if s.client != nil {
		return s.client.cleanup()
	}
	return nil
}

func (s *nvmlScraper) scrape(_ context.Context) (pmetric.Metrics, error) {
	deviceMetrics, err := s.client.collectDeviceMetrics()

	for _, metric := range deviceMetrics {
		timestamp := pcommon.NewTimestampFromTime(metric.time)
		model := s.client.getDeviceModelName(metric.gpuIndex)
		UUID := s.client.getDeviceUUID(metric.gpuIndex)
		gpuIndex := fmt.Sprintf("%d", metric.gpuIndex)
		switch metric.name {
		case "nvml.gpu.utilization":
			s.mb.RecordNvmlGpuUtilizationDataPoint(
				timestamp, metric.asFloat64(), model, gpuIndex, UUID)
		case "nvml.gpu.memory.bytes_used":
			s.mb.RecordNvmlGpuMemoryBytesUsedDataPoint(
				timestamp, metric.asInt64(), model, gpuIndex, UUID, metadata.AttributeMemoryStateUsed)
		case "nvml.gpu.memory.bytes_free":
			s.mb.RecordNvmlGpuMemoryBytesUsedDataPoint(
				timestamp, metric.asInt64(), model, gpuIndex, UUID, metadata.AttributeMemoryStateFree)
		}
	}

	processMetrics := s.client.collectProcessMetrics()
	for _, metric := range processMetrics {
		timestamp := pcommon.NewTimestampFromTime(metric.time)
		model := s.client.getDeviceModelName(metric.gpuIndex)
		UUID := s.client.getDeviceUUID(metric.gpuIndex)
		gpuIndex := fmt.Sprintf("%d", metric.gpuIndex)

		s.mb.RecordNvmlGpuProcessesUtilizationDataPoint(
			timestamp, float64(metric.lifetimeGpuUtilization)/100.0, model, gpuIndex, UUID, int64(metric.processPid),
			metric.processName, metric.command, metric.commandLine, metric.owner)

		s.mb.RecordNvmlGpuProcessesMaxBytesUsedDataPoint(
			timestamp, int64(metric.lifetimeGpuMaxMemory), model, gpuIndex, UUID, int64(metric.processPid),
			metric.processName, metric.command, metric.commandLine, metric.owner)
	}

	return s.mb.Emit(), err
}
