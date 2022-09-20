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

package diskscraper // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal/scraper/diskscraper"

import (
	"context"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v3/host"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/receiver/scrapererror"
	"go.opentelemetry.io/collector/service/featuregate"

	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/coreinternal/processor/filterset"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal/perfcounters"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal/scraper/diskscraper/internal/metadata"
)

const (
	metricsLen = 5

	logicalDisk = "LogicalDisk"

	readsPerSec  = "Disk Reads/sec"
	writesPerSec = "Disk Writes/sec"

	readBytesPerSec  = "Disk Read Bytes/sec"
	writeBytesPerSec = "Disk Write Bytes/sec"

	idleTime = "% Idle Time"

	avgDiskSecsPerRead  = "Avg. Disk sec/Read"
	avgDiskSecsPerWrite = "Avg. Disk sec/Write"

	queueLength = "Current Disk Queue Length"
)

// scraper for Disk Metrics
type scraper struct {
	settings  component.ReceiverCreateSettings
	config    *Config
	startTime pcommon.Timestamp
	mb        *metadata.MetricsBuilder
	includeFS filterset.FilterSet
	excludeFS filterset.FilterSet

	perfCounterScraper perfcounters.PerfCounterScraper

	// for mocking
	bootTime                             func() (uint64, error)
	emitMetricsWithDirectionAttribute    bool
	emitMetricsWithoutDirectionAttribute bool
}

// newDiskScraper creates a Disk Scraper
func newDiskScraper(_ context.Context, settings component.ReceiverCreateSettings, cfg *Config) (*scraper, error) {
	scraper := &scraper{settings: settings, config: cfg, perfCounterScraper: &perfcounters.PerfLibScraper{}, bootTime: host.BootTime}
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

	return s.perfCounterScraper.Initialize(logicalDisk)
}

func (s *scraper) scrape(ctx context.Context) (pmetric.Metrics, error) {
	now := pcommon.NewTimestampFromTime(time.Now())

	counters, err := s.perfCounterScraper.Scrape()
	if err != nil {
		return pmetric.NewMetrics(), scrapererror.NewPartialScrapeError(err, metricsLen)
	}

	logicalDiskObject, err := counters.GetObject(logicalDisk)
	if err != nil {
		return pmetric.NewMetrics(), scrapererror.NewPartialScrapeError(err, metricsLen)
	}

	// filter devices by name
	logicalDiskObject.Filter(s.includeFS, s.excludeFS, false)

	logicalDiskCounterValues, err := logicalDiskObject.GetValues(readsPerSec, writesPerSec, readBytesPerSec, writeBytesPerSec, idleTime, avgDiskSecsPerRead, avgDiskSecsPerWrite, queueLength)
	if err != nil {
		return pmetric.NewMetrics(), scrapererror.NewPartialScrapeError(err, metricsLen)
	}

	if len(logicalDiskCounterValues) > 0 {
		s.recordDiskIOMetric(now, logicalDiskCounterValues)
		s.recordDiskOperationsMetric(now, logicalDiskCounterValues)
		s.recordDiskIOTimeMetric(now, logicalDiskCounterValues)
		s.recordDiskOperationTimeMetric(now, logicalDiskCounterValues)
		s.recordDiskPendingOperationsMetric(now, logicalDiskCounterValues)
	}

	return s.mb.Emit(), nil
}

func (s *scraper) recordDiskIOMetric(now pcommon.Timestamp, logicalDiskCounterValues []*perfcounters.CounterValues) {
	for _, logicalDiskCounter := range logicalDiskCounterValues {
		if s.emitMetricsWithDirectionAttribute {
			s.mb.RecordSystemDiskIoDataPoint(now, logicalDiskCounter.Values[readBytesPerSec], logicalDiskCounter.InstanceName, metadata.AttributeDirectionRead)
			s.mb.RecordSystemDiskIoDataPoint(now, logicalDiskCounter.Values[writeBytesPerSec], logicalDiskCounter.InstanceName, metadata.AttributeDirectionWrite)
		}
		if s.emitMetricsWithoutDirectionAttribute {
			s.mb.RecordSystemDiskIoReadDataPoint(now, logicalDiskCounter.Values[readBytesPerSec], logicalDiskCounter.InstanceName)
			s.mb.RecordSystemDiskIoWriteDataPoint(now, logicalDiskCounter.Values[writeBytesPerSec], logicalDiskCounter.InstanceName)
		}
	}
}

func (s *scraper) recordDiskOperationsMetric(now pcommon.Timestamp, logicalDiskCounterValues []*perfcounters.CounterValues) {
	for _, logicalDiskCounter := range logicalDiskCounterValues {
		if s.emitMetricsWithDirectionAttribute {
			s.mb.RecordSystemDiskOperationsDataPoint(now, logicalDiskCounter.Values[readsPerSec], logicalDiskCounter.InstanceName, metadata.AttributeDirectionRead)
			s.mb.RecordSystemDiskOperationsDataPoint(now, logicalDiskCounter.Values[writesPerSec], logicalDiskCounter.InstanceName, metadata.AttributeDirectionWrite)
		}
		if s.emitMetricsWithoutDirectionAttribute {
			s.mb.RecordSystemDiskOperationsReadDataPoint(now, logicalDiskCounter.Values[readsPerSec], logicalDiskCounter.InstanceName)
			s.mb.RecordSystemDiskOperationsWriteDataPoint(now, logicalDiskCounter.Values[writesPerSec], logicalDiskCounter.InstanceName)
		}
	}
}

func (s *scraper) recordDiskIOTimeMetric(now pcommon.Timestamp, logicalDiskCounterValues []*perfcounters.CounterValues) {
	for _, logicalDiskCounter := range logicalDiskCounterValues {
		// disk active time = system boot time - disk idle time
		s.mb.RecordSystemDiskIoTimeDataPoint(now, float64(now-s.startTime)/1e9-float64(logicalDiskCounter.Values[idleTime])/1e7, logicalDiskCounter.InstanceName)
	}
}

func (s *scraper) recordDiskOperationTimeMetric(now pcommon.Timestamp, logicalDiskCounterValues []*perfcounters.CounterValues) {
	for _, logicalDiskCounter := range logicalDiskCounterValues {
		if s.emitMetricsWithDirectionAttribute {
			s.mb.RecordSystemDiskOperationTimeDataPoint(now, float64(logicalDiskCounter.Values[avgDiskSecsPerRead])/1e7, logicalDiskCounter.InstanceName, metadata.AttributeDirectionRead)
			s.mb.RecordSystemDiskOperationTimeDataPoint(now, float64(logicalDiskCounter.Values[avgDiskSecsPerWrite])/1e7, logicalDiskCounter.InstanceName, metadata.AttributeDirectionWrite)
		}
		if s.emitMetricsWithoutDirectionAttribute {
			s.mb.RecordSystemDiskOperationTimeReadDataPoint(now, float64(logicalDiskCounter.Values[avgDiskSecsPerRead])/1e7, logicalDiskCounter.InstanceName)
			s.mb.RecordSystemDiskOperationTimeWriteDataPoint(now, float64(logicalDiskCounter.Values[avgDiskSecsPerWrite])/1e7, logicalDiskCounter.InstanceName)
		}
	}
}

func (s *scraper) recordDiskPendingOperationsMetric(now pcommon.Timestamp, logicalDiskCounterValues []*perfcounters.CounterValues) {
	for _, logicalDiskCounter := range logicalDiskCounterValues {
		s.mb.RecordSystemDiskPendingOperationsDataPoint(now, logicalDiskCounter.Values[queueLength], logicalDiskCounter.InstanceName)
	}
}
