// Copyright 2021 Google LLC
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

package agentmetricsprocessor

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func cleanCPUNumber(rms pmetric.ResourceMetricsSlice) error {
	for i := 0; i < rms.Len(); i++ {
		ilms := rms.At(i).ScopeMetrics()
		for j := 0; j < ilms.Len(); j++ {
			metrics := ilms.At(j).Metrics()
			for k := 0; k < metrics.Len(); k++ {
				metric := metrics.At(k)

				if err := forEachPoint(metric, cleanCPUNumberDataPoint); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

type labelsMapper interface {
	Attributes() pcommon.Map
}

func forEachPoint(metric pmetric.Metric, fn func(labelsMapper) error) error {
	switch t := metric.Type(); t {
	case pmetric.MetricTypeSum:
		dp := metric.Sum().DataPoints()
		for i := 0; i < dp.Len(); i++ {
			if err := fn(dp.At(i)); err != nil {
				return err
			}
		}
	case pmetric.MetricTypeGauge:
		dp := metric.Gauge().DataPoints()
		for i := 0; i < dp.Len(); i++ {
			if err := fn(dp.At(i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func cleanCPUNumberDataPoint(lm labelsMapper) error {
	sm := lm.Attributes()
	if value, ok := sm.Get("cpu"); ok {
		value.SetStr(strings.TrimPrefix(value.Str(), "cpu"))
	}
	return nil
}
