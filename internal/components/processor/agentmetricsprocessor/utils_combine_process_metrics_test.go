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
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func generateProcessResourceMetricsInput() pmetric.Metrics {
	input := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b1 := rmb.addResourceMetrics(nil)
	b1.addMetric("m1", pmetric.MetricTypeSum, true).addIntDataPoint(1, map[string]string{"label1": "value1"})
	b1.addMetric("m2", pmetric.MetricTypeSum, false).addDoubleDataPoint(2, map[string]string{"label1": "value1"})

	b2 := rmb.addResourceMetrics(map[string]pcommon.Value{
		"process.pid":             pcommon.NewValueInt(1),
		"process.executable.name": pcommon.NewValueStr("process1"),
		"process.executable.path": pcommon.NewValueStr("/path/to/process1"),
		"process.command":         pcommon.NewValueStr("to/process1"),
		"process.command_line":    pcommon.NewValueStr("to/process1 -arg arg"),
		"process.owner":           pcommon.NewValueStr("username1"),
	})
	b2.addMetric("m3", pmetric.MetricTypeSum, true).addIntDataPoint(3, map[string]string{"label1": "value1"})
	b2.addMetric("m4", pmetric.MetricTypeGauge, false).addDoubleDataPoint(4, map[string]string{"label1": "value1"})

	b3 := rmb.addResourceMetrics(map[string]pcommon.Value{
		"process.pid":             pcommon.NewValueInt(2),
		"process.executable.name": pcommon.NewValueStr("process2"),
		"process.executable.path": pcommon.NewValueStr("/path/to/process2"),
		"process.command":         pcommon.NewValueStr("to/process2"),
		"process.command_line":    pcommon.NewValueStr("to/process2 -arg arg"),
		"process.owner":           pcommon.NewValueStr("username2"),
	})
	b3.addMetric("m3", pmetric.MetricTypeSum, true).addIntDataPoint(5, map[string]string{"label1": "value2"})
	b3.addMetric("m4", pmetric.MetricTypeGauge, false).addDoubleDataPoint(6, map[string]string{"label1": "value2"})

	rmb.Build().CopyTo(input.ResourceMetrics())
	return input
}

func generateProcessResourceMetricsExpected() pmetric.Metrics {
	expected := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)

	b.addMetric("m1", pmetric.MetricTypeSum, true).addIntDataPoint(1, map[string]string{"label1": "value1"})
	b.addMetric("m2", pmetric.MetricTypeGauge, false).addDoubleDataPoint(2, map[string]string{"label1": "value1"})

	mb3 := b.addMetric("m3", pmetric.MetricTypeSum, true)
	mb3.addIntDataPoint(3, map[string]string{
		"label1":       "value1",
		"pid":          "1",
		"command":      "process1",
		"command_line": "to/process1 -arg arg",
		"owner":        "username1",
	})
	mb3.addIntDataPoint(5, map[string]string{
		"label1":       "value2",
		"pid":          "2",
		"command":      "process2",
		"command_line": "to/process2 -arg arg",
		"owner":        "username2",
	})

	mb4 := b.addMetric("m4", pmetric.MetricTypeGauge, false)
	mb4.addDoubleDataPoint(4, map[string]string{
		"label1":       "value1",
		"pid":          "1",
		"command":      "process1",
		"command_line": "to/process1 -arg arg",
		"owner":        "username1",
	})
	mb4.addDoubleDataPoint(6, map[string]string{
		"label1":       "value2",
		"pid":          "2",
		"command":      "process2",
		"command_line": "to/process2 -arg arg",
		"owner":        "username2",
	})

	rmb.Build().CopyTo(expected.ResourceMetrics())
	return expected
}
