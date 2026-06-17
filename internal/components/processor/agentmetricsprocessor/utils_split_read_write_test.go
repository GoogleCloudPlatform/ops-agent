// Copyright 2020 Google LLC
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
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func generateReadWriteMetricsInput() pmetric.Metrics {
	input := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)

	mb1 := b.addMetric("system.disk.io", pmetric.MetricTypeSum, true)
	mb1.addIntDataPoint(1, map[string]string{"label1": "value1", "direction": "read"})
	mb1.addIntDataPoint(2, map[string]string{"label1": "value2", "direction": "write"})

	mb2 := b.addMetric("process.disk.io", pmetric.MetricTypeSum, false)
	mb2.addDoubleDataPoint(3, map[string]string{"label1": "value1", "direction": "read"})
	mb2.addDoubleDataPoint(4, map[string]string{"label1": "value2", "direction": "write"})

	rmb.Build().CopyTo(input.ResourceMetrics())
	return input
}

func generateReadWriteMetricsExpected() pmetric.Metrics {
	expected := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)
	b.addMetric("system.disk.read_io", pmetric.MetricTypeSum, true).addIntDataPoint(1, map[string]string{"label1": "value1"})
	b.addMetric("process.disk.read_io", pmetric.MetricTypeGauge, false).addDoubleDataPoint(3, map[string]string{"label1": "value1"})
	b.addMetric("system.disk.write_io", pmetric.MetricTypeSum, true).addIntDataPoint(2, map[string]string{"label1": "value2"})
	b.addMetric("process.disk.write_io", pmetric.MetricTypeGauge, false).addDoubleDataPoint(4, map[string]string{"label1": "value2"})

	rmb.Build().CopyTo(expected.ResourceMetrics())
	return expected
}
