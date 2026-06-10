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

func generateUtilizationMetricsInput() pmetric.Metrics {
	input := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)

	mb1 := b.addMetric("system.cpu.time", pmetric.MetricTypeSum, true)
	mb1.addDoubleDataPoint(101, map[string]string{"label1": "value1", "state": "user"})
	mb1.addDoubleDataPoint(202, map[string]string{"label1": "value2", "state": "user"})
	mb1.addDoubleDataPoint(303, map[string]string{"label1": "value1", "state": "idle"})
	mb1.addDoubleDataPoint(404, map[string]string{"label1": "value2", "state": "idle"})

	mb2 := b.addMetric("system.memory.usage", pmetric.MetricTypeSum, false)
	mb2.addIntDataPoint(1, map[string]string{"state": "used"})
	mb2.addIntDataPoint(2, map[string]string{"state": "free"})

	mb3 := b.addMetric("system.filesystem.usage", pmetric.MetricTypeSum, true)
	mb3.addDoubleDataPoint(1.5, map[string]string{"label1": "value1", "label2": "value1", "state": "used"})
	mb3.addDoubleDataPoint(2.3, map[string]string{"label1": "value2", "label2": "value1", "state": "used"})
	mb3.addDoubleDataPoint(7.8, map[string]string{"label1": "value1", "label2": "value2", "state": "used"})
	mb3.addDoubleDataPoint(9.7, map[string]string{"label1": "value2", "label2": "value2", "state": "used"})
	mb3.addDoubleDataPoint(3.3, map[string]string{"label1": "value1", "label2": "value1", "state": "free"})
	mb3.addDoubleDataPoint(6.4, map[string]string{"label1": "value2", "label2": "value1", "state": "free"})
	mb3.addDoubleDataPoint(5.9, map[string]string{"label1": "value1", "label2": "value2", "state": "free"})
	mb3.addDoubleDataPoint(2.1, map[string]string{"label1": "value2", "label2": "value2", "state": "free"})
	mb3.addDoubleDataPoint(2.6, map[string]string{"label1": "value1", "label2": "value1", "state": "reserved"})
	mb3.addDoubleDataPoint(6.0, map[string]string{"label1": "value2", "label2": "value1", "state": "reserved"})
	mb3.addDoubleDataPoint(9.9, map[string]string{"label1": "value1", "label2": "value2", "state": "reserved"})
	mb3.addDoubleDataPoint(1.5, map[string]string{"label1": "value2", "label2": "value2", "state": "reserved"})

	rmb.Build().CopyTo(input.ResourceMetrics())
	return input
}

func generateUtilizationPrevCPUTimeValuesInput() map[string]float64 {
	return map[string]float64{
		"label1=value1;state=user": 100,
		"label1=value2;state=user": 200,
		"label1=value1;state=idle": 300,
		"label1=value2;state=idle": 400,
	}
}

func generateUtilizationMetricsExpected() pmetric.Metrics {
	expected := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)

	mb1 := b.addMetric("system.cpu.time", pmetric.MetricTypeSum, true)
	mb1.addDoubleDataPoint(101, map[string]string{"label1": "value1", "state": "user", "blank": ""})
	mb1.addDoubleDataPoint(202, map[string]string{"label1": "value2", "state": "user", "blank": ""})
	mb1.addDoubleDataPoint(303, map[string]string{"label1": "value1", "state": "idle", "blank": ""})
	mb1.addDoubleDataPoint(404, map[string]string{"label1": "value2", "state": "idle", "blank": ""})

	mb2 := b.addMetric("system.memory.usage", pmetric.MetricTypeGauge, false)
	mb2.addIntDataPoint(1, map[string]string{"state": "used"})
	mb2.addIntDataPoint(2, map[string]string{"state": "free"})

	mb3 := b.addMetric("system.filesystem.usage", pmetric.MetricTypeSum, true)
	mb3.addDoubleDataPoint(1.5, map[string]string{"label1": "value1", "label2": "value1", "state": "used"})
	mb3.addDoubleDataPoint(2.3, map[string]string{"label1": "value2", "label2": "value1", "state": "used"})
	mb3.addDoubleDataPoint(7.8, map[string]string{"label1": "value1", "label2": "value2", "state": "used"})
	mb3.addDoubleDataPoint(9.7, map[string]string{"label1": "value2", "label2": "value2", "state": "used"})
	mb3.addDoubleDataPoint(3.3, map[string]string{"label1": "value1", "label2": "value1", "state": "free"})
	mb3.addDoubleDataPoint(6.4, map[string]string{"label1": "value2", "label2": "value1", "state": "free"})
	mb3.addDoubleDataPoint(5.9, map[string]string{"label1": "value1", "label2": "value2", "state": "free"})
	mb3.addDoubleDataPoint(2.1, map[string]string{"label1": "value2", "label2": "value2", "state": "free"})
	mb3.addDoubleDataPoint(2.6, map[string]string{"label1": "value1", "label2": "value1", "state": "reserved"})
	mb3.addDoubleDataPoint(6.0, map[string]string{"label1": "value2", "label2": "value1", "state": "reserved"})
	mb3.addDoubleDataPoint(9.9, map[string]string{"label1": "value1", "label2": "value2", "state": "reserved"})
	mb3.addDoubleDataPoint(1.5, map[string]string{"label1": "value2", "label2": "value2", "state": "reserved"})

	mb4 := b.addMetric("system.cpu.utilization", pmetric.MetricTypeGauge, false)
	mb4.addDoubleDataPoint(1.0/(1.0+3.0)*100, map[string]string{"label1": "value1", "state": "user"})
	mb4.addDoubleDataPoint(2.0/(2.0+4.0)*100, map[string]string{"label1": "value2", "state": "user"})
	mb4.addDoubleDataPoint(3.0/(1.0+3.0)*100, map[string]string{"label1": "value1", "state": "idle"})
	mb4.addDoubleDataPoint(4.0/(2.0+4.0)*100, map[string]string{"label1": "value2", "state": "idle"})

	mb5 := b.addMetric("system.memory.utilization", pmetric.MetricTypeGauge, false)
	mb5.addDoubleDataPoint(1.0/(1.0+2.0)*100, map[string]string{"state": "used"})
	mb5.addDoubleDataPoint(2.0/(1.0+2.0)*100, map[string]string{"state": "free"})

	mb6 := b.addMetric("system.filesystem.utilization", pmetric.MetricTypeGauge, false)
	mb6.addDoubleDataPoint(1.5/(1.5+3.3+2.6)*100, map[string]string{"label1": "value1", "label2": "value1", "state": "used"})
	mb6.addDoubleDataPoint(2.3/(2.3+6.4+6.0)*100, map[string]string{"label1": "value2", "label2": "value1", "state": "used"})
	mb6.addDoubleDataPoint(7.8/(7.8+5.9+9.9)*100, map[string]string{"label1": "value1", "label2": "value2", "state": "used"})
	mb6.addDoubleDataPoint(9.7/(9.7+2.1+1.5)*100, map[string]string{"label1": "value2", "label2": "value2", "state": "used"})
	mb6.addDoubleDataPoint(3.3/(1.5+3.3+2.6)*100, map[string]string{"label1": "value1", "label2": "value1", "state": "free"})
	mb6.addDoubleDataPoint(6.4/(2.3+6.4+6.0)*100, map[string]string{"label1": "value2", "label2": "value1", "state": "free"})
	mb6.addDoubleDataPoint(5.9/(7.8+5.9+9.9)*100, map[string]string{"label1": "value1", "label2": "value2", "state": "free"})
	mb6.addDoubleDataPoint(2.1/(9.7+2.1+1.5)*100, map[string]string{"label1": "value2", "label2": "value2", "state": "free"})
	mb6.addDoubleDataPoint(2.6/(1.5+3.3+2.6)*100, map[string]string{"label1": "value1", "label2": "value1", "state": "reserved"})
	mb6.addDoubleDataPoint(6.0/(2.3+6.4+6.0)*100, map[string]string{"label1": "value2", "label2": "value1", "state": "reserved"})
	mb6.addDoubleDataPoint(9.9/(7.8+5.9+9.9)*100, map[string]string{"label1": "value1", "label2": "value2", "state": "reserved"})
	mb6.addDoubleDataPoint(1.5/(9.7+2.1+1.5)*100, map[string]string{"label1": "value2", "label2": "value2", "state": "reserved"})

	rmb.Build().CopyTo(expected.ResourceMetrics())
	return expected
}

func generateUtilizationPrevCPUTimeValuesExpected() map[string]float64 {
	return map[string]float64{
		"label1=value1;state=user": 101,
		"label1=value2;state=user": 202,
		"label1=value1;state=idle": 303,
		"label1=value2;state=idle": 404,
	}
}
