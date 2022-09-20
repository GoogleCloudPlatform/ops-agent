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

//go:build linux
// +build linux

package memoryscraper // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal/scraper/memoryscraper"

import (
	"github.com/shirou/gopsutil/v3/mem"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal/scraper/memoryscraper/internal/metadata"
)

func (s *scraper) recordMemoryUsageMetric(now pcommon.Timestamp, memInfo *mem.VirtualMemoryStat) {
	s.mb.RecordSystemMemoryUsageDataPoint(now, int64(memInfo.Used), metadata.AttributeStateUsed)
	s.mb.RecordSystemMemoryUsageDataPoint(now, int64(memInfo.Free), metadata.AttributeStateFree)
	s.mb.RecordSystemMemoryUsageDataPoint(now, int64(memInfo.Buffers), metadata.AttributeStateBuffered)
	s.mb.RecordSystemMemoryUsageDataPoint(now, int64(memInfo.Cached), metadata.AttributeStateCached)
	s.mb.RecordSystemMemoryUsageDataPoint(now, int64(memInfo.Sreclaimable), metadata.AttributeStateSlabReclaimable)
	s.mb.RecordSystemMemoryUsageDataPoint(now, int64(memInfo.Sunreclaim), metadata.AttributeStateSlabUnreclaimable)
}

func (s *scraper) recordMemoryUtilizationMetric(now pcommon.Timestamp, memInfo *mem.VirtualMemoryStat) {
	s.mb.RecordSystemMemoryUtilizationDataPoint(now, float64(memInfo.Used)/float64(memInfo.Total), metadata.AttributeStateUsed)
	s.mb.RecordSystemMemoryUtilizationDataPoint(now, float64(memInfo.Free)/float64(memInfo.Total), metadata.AttributeStateFree)
	s.mb.RecordSystemMemoryUtilizationDataPoint(now, float64(memInfo.Buffers)/float64(memInfo.Total), metadata.AttributeStateBuffered)
	s.mb.RecordSystemMemoryUtilizationDataPoint(now, float64(memInfo.Cached)/float64(memInfo.Total), metadata.AttributeStateCached)
	s.mb.RecordSystemMemoryUtilizationDataPoint(now, float64(memInfo.Sreclaimable)/float64(memInfo.Total), metadata.AttributeStateSlabReclaimable)
	s.mb.RecordSystemMemoryUtilizationDataPoint(now, float64(memInfo.Sunreclaim)/float64(memInfo.Total), metadata.AttributeStateSlabUnreclaimable)
}
