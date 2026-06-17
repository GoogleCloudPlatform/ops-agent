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

func commonAverageDiskInput(b metricsBuilder) {
	b.timestamp = twoSeconds

	mb1 := b.addMetric("system.disk.operation_time", pmetric.MetricTypeSum, true)
	mb1.addDoubleDataPoint(200, map[string]string{"device": "hda", "direction": "read"})
	mb1.addDoubleDataPoint(400, map[string]string{"device": "hda", "direction": "write"})
	mb1.addDoubleDataPoint(100, map[string]string{"device": "hdb", "direction": "read"})
	mb1.addDoubleDataPoint(100, map[string]string{"device": "hdb", "direction": "write"})

	mb2 := b.addMetric("system.disk.operations", pmetric.MetricTypeSum, true)
	mb2.addIntDataPoint(5, map[string]string{"device": "hda", "direction": "read"})
	mb2.addIntDataPoint(4, map[string]string{"device": "hda", "direction": "write"})
	mb2.addIntDataPoint(2, map[string]string{"device": "hdb", "direction": "read"})
	mb2.addIntDataPoint(20, map[string]string{"device": "hdb", "direction": "write"})
}

func generateAverageDiskInput() pmetric.Metrics {
	input := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)

	commonAverageDiskInput(b)

	rmb.Build().CopyTo(input.ResourceMetrics())
	return input
}

func od(ops int64, time, cum float64) opData {
	opsDp := pmetric.NewNumberDataPoint()
	opsDp.SetIntValue(ops)
	timeDp := pmetric.NewNumberDataPoint()
	timeDp.SetDoubleValue(time)
	return opData{
		opsDp,
		timeDp,
		cum,
	}
}

func generateAverageDiskPrevOpInput() map[opKey]opData {
	return map[opKey]opData{
		{"hda", "read"}:  od(0, 100, 15),
		{"hda", "write"}: od(3, 300, 20),
		{"hdb", "read"}:  od(2, 100, 30),
		{"hdb", "write"}: od(10, 50, 5),
	}
}

func generateAverageDiskExpected() pmetric.Metrics {
	expected := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)

	commonAverageDiskInput(b)

	// No average_operation_time expected for the first point

	rmb.Build().CopyTo(expected.ResourceMetrics())
	return expected
}

const twoSeconds = 2000000000

func generateAverageDiskPrevExpected() pmetric.Metrics {
	expected := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)

	// One second elapsed
	b.timestamp = twoSeconds

	commonAverageDiskInput(b)

	mb3 := b.addMetric("system.disk.average_operation_time", pmetric.MetricTypeSum, true)
	mb3.addDoubleDataPoint(15+2*(100/5), map[string]string{"device": "hda", "direction": "read"})
	mb3.addDoubleDataPoint(20+2*(100/1), map[string]string{"device": "hda", "direction": "write"})
	mb3.addDoubleDataPoint(30, map[string]string{"device": "hdb", "direction": "read"})
	mb3.addDoubleDataPoint(5+2*(50/10), map[string]string{"device": "hdb", "direction": "write"})

	rmb.Build().CopyTo(expected.ResourceMetrics())
	return expected
}
