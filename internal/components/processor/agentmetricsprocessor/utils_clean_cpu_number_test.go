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
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func generateCPUMetricsInput() pmetric.Metrics {
	input := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)

	mb1 := b.addMetric("system.cpu.time", pmetric.MetricTypeSum, true)
	mb1.addDoubleDataPoint(1, map[string]string{"cpu": "cpu0", "state": "idle"})
	mb1.addDoubleDataPoint(2, map[string]string{"cpu": "cpu0", "state": "system"})

	rmb.Build().CopyTo(input.ResourceMetrics())
	return input
}

func generateCPUMetricsExpected() pmetric.Metrics {
	expected := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)
	mb1 := b.addMetric("system.cpu.time", pmetric.MetricTypeSum, true)
	mb1.addDoubleDataPoint(1, map[string]string{"cpu": "0", "state": "idle", "blank": ""})
	mb1.addDoubleDataPoint(2, map[string]string{"cpu": "0", "state": "system", "blank": ""})

	b.addMetric("system.cpu.utilization", pmetric.MetricTypeGauge, false)

	rmb.Build().CopyTo(expected.ResourceMetrics())
	return expected
}
