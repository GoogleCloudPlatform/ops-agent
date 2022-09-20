// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !windows
// +build !windows

package diskscraper // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal/scraper/diskscraper"

import (
	"context"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/receiver/scrapererror"
	"go.opentelemetry.io/collector/service/featuregate"

	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/coreinternal/processor/filterset"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal/scraper/diskscraper/internal/metadata"
)

const (
	standardMetricsLen = 5
	metricsLen         = standardMetricsLen + systemSpecificMetricsLen
)

// scraper for Disk Metrics
type scraper struct {
	settings  component.ReceiverCreateSettings
	config    *Config
	startTime pcommon.Timestamp
	mb        *metadata.MetricsBuilder
	includeFS filterset.FilterSet
	excludeFS filterset.FilterSet

	// for mocking
	bootTime                             func() (uint64, error)
	ioCounters                           func(names ...string) (map[string]disk.IOCountersStat, error)
	emitMetricsWithDirectionAttribute    bool
	emitMetricsWithoutDirectionAttribute bool
}

// newDiskScraper creates a Disk Scraper
func newDiskScraper(_ context.Context, settings component.ReceiverCreateSettings, cfg *Config) (*scraper, error) {
	scraper := &scraper{settings: settings, config: cfg, bootTime: host.BootTime, ioCounters: disk.IOCounters}
	scraper.emitMetricsWithDirectionAttribute = featuregate.GetRegistry().IsEnabled(internal.EmitMetricsWithDirectionAttributeFeatureGateID)
	scraper.emitMetricsWithoutDirectionAttribute = featuregate.GetRegistry().IsEnabled(internal.EmitMetricsWithoutDirectionAttributeFeatureGateID)

	var err error

	if len(cfg.Include.Devices) > 0 {
		scraper.includeFS, err = filterset.CreateFilterSet(cfg.Include.Devices, &cfg.Include.Config)
		if err != nil {
			return nil, fmt.Errorf("error creating device include filters: %w", err)
		}
	}

	if len(cfg.Exclude.Devices) > 0 {
		scraper.excludeFS, err = filterset.CreateFilterSet(cfg.Exclude.Devices, &cfg.Exclude.Config)
		if err != nil {
			return nil, fmt.Errorf("error creating device exclude filters: %w", err)
		}
	}

	return scraper, nil
}

func (s *scraper) start(context.Context, component.Host) error {
	bootTime, err := s.bootTime()
	if err != nil {
		return err
	}

	s.startTime = pcommon.Timestamp(bootTime * 1e9)
	s.mb = metadata.NewMetricsBuilder(s.config.Metrics, s.settings.BuildInfo, metadata.WithStartTime(s.startTime))
	return nil
}

func (s *scraper) scrape(_ context.Context) (pmetric.Metrics, error) {
	now := pcommon.NewTimestampFromTime(time.Now())
	ioCounters, err := s.ioCounters()
	if err != nil {
		return pmetric.NewMetrics(), scrapererror.NewPartialScrapeError(err, metricsLen)
	}

	// filter devices by name
	ioCounters = s.filterByDevice(ioCounters)

	if len(ioCounters) > 0 {
		s.recordDiskIOMetric(now, ioCounters)
		s.recordDiskOperationsMetric(now, ioCounters)
		s.recordDiskIOTimeMetric(now, ioCounters)
		s.recordDiskOperationTimeMetric(now, ioCounters)
		s.recordDiskPendingOperationsMetric(now, ioCounters)
		s.recordSystemSpecificDataPoints(now, ioCounters)
	}

	return s.mb.Emit(), nil
}

func (s *scraper) recordDiskIOMetric(now pcommon.Timestamp, ioCounters map[string]disk.IOCountersStat) {
	for device, ioCounter := range ioCounters {
		if s.emitMetricsWithDirectionAttribute {
			s.mb.RecordSystemDiskIoDataPoint(now, int64(ioCounter.ReadBytes), device, metadata.AttributeDirectionRead)
			s.mb.RecordSystemDiskIoDataPoint(now, int64(ioCounter.WriteBytes), device, metadata.AttributeDirectionWrite)
		}
		if s.emitMetricsWithoutDirectionAttribute {
			s.mb.RecordSystemDiskIoReadDataPoint(now, int64(ioCounter.ReadBytes), device)
			s.mb.RecordSystemDiskIoWriteDataPoint(now, int64(ioCounter.WriteBytes), device)
		}
	}
}

func (s *scraper) recordDiskOperationsMetric(now pcommon.Timestamp, ioCounters map[string]disk.IOCountersStat) {
	for device, ioCounter := range ioCounters {
		if s.emitMetricsWithDirectionAttribute {
			s.mb.RecordSystemDiskOperationsDataPoint(now, int64(ioCounter.ReadCount), device, metadata.AttributeDirectionRead)
			s.mb.RecordSystemDiskOperationsDataPoint(now, int64(ioCounter.WriteCount), device, metadata.AttributeDirectionWrite)
		}
		if s.emitMetricsWithoutDirectionAttribute {
			s.mb.RecordSystemDiskOperationsReadDataPoint(now, int64(ioCounter.ReadCount), device)
			s.mb.RecordSystemDiskOperationsWriteDataPoint(now, int64(ioCounter.WriteCount), device)
		}
	}
}

func (s *scraper) recordDiskIOTimeMetric(now pcommon.Timestamp, ioCounters map[string]disk.IOCountersStat) {
	for device, ioCounter := range ioCounters {
		s.mb.RecordSystemDiskIoTimeDataPoint(now, float64(ioCounter.IoTime)/1e3, device)
	}
}

func (s *scraper) recordDiskOperationTimeMetric(now pcommon.Timestamp, ioCounters map[string]disk.IOCountersStat) {
	for device, ioCounter := range ioCounters {
		if s.emitMetricsWithDirectionAttribute {
			s.mb.RecordSystemDiskOperationTimeDataPoint(now, float64(ioCounter.ReadTime)/1e3, device, metadata.AttributeDirectionRead)
			s.mb.RecordSystemDiskOperationTimeDataPoint(now, float64(ioCounter.WriteTime)/1e3, device, metadata.AttributeDirectionWrite)
		}
		if s.emitMetricsWithoutDirectionAttribute {
			s.mb.RecordSystemDiskOperationTimeReadDataPoint(now, float64(ioCounter.ReadTime)/1e3, device)
			s.mb.RecordSystemDiskOperationTimeWriteDataPoint(now, float64(ioCounter.WriteTime)/1e3, device)
		}
	}
}

func (s *scraper) recordDiskPendingOperationsMetric(now pcommon.Timestamp, ioCounters map[string]disk.IOCountersStat) {
	for device, ioCounter := range ioCounters {
		s.mb.RecordSystemDiskPendingOperationsDataPoint(now, int64(ioCounter.IopsInProgress), device)
	}
}

func (s *scraper) filterByDevice(ioCounters map[string]disk.IOCountersStat) map[string]disk.IOCountersStat {
	if s.includeFS == nil && s.excludeFS == nil {
		return ioCounters
	}

	for device := range ioCounters {
		if !s.includeDevice(device) {
			delete(ioCounters, device)
		}
	}
	return ioCounters
}

func (s *scraper) includeDevice(deviceName string) bool {
	return (s.includeFS == nil || s.includeFS.Matches(deviceName)) &&
		(s.excludeFS == nil || !s.excludeFS.Matches(deviceName))
}
