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

import "go.opentelemetry.io/collector/pdata/pmetric"

// https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/metrics/semantic_conventions/system-metrics.md#systemdisk---disk-controller-metrics
const opName = "system.disk.operations"
const opTimeName = "system.disk.operation_time"

// system.disk.operations contains the cumulative number of operations per disk and direction
// system.disk.operation_time contains the cumulative busy time per disk and direction

func (mtp *agentMetricsProcessor) appendAverageDiskMetrics(rms pmetric.ResourceMetricsSlice) error {
	for i := 0; i < rms.Len(); i++ {
		ilms := rms.At(i).ScopeMetrics()
		for j := 0; j < ilms.Len(); j++ {
			// Collect the corresponding count and time so they can be divided.
			newOp := make(map[opKey]opData)
			var opTimeMetric pmetric.Metric
			metrics := ilms.At(j).Metrics()
			for k := 0; k < metrics.Len(); k++ {
				metric := metrics.At(k)

				// ignore all metrics except the ones we want to compute utilizations for
				switch metric.Name() {
				case opTimeName:
					opTimeMetric = metric
					fallthrough
				case opName:
					ndps := metric.Sum().DataPoints()
					for i := 0; i < ndps.Len(); i++ {
						ndp := ndps.At(i)

						lm := ndp.Attributes()
						device, _ := lm.Get("device")
						direction, _ := lm.Get("direction")
						key := opKey{device.AsString(), direction.AsString()}

						op, ok := newOp[key]
						if !ok {
							op = mtp.prevOp[key]
						}
						// Can't just save ndp because it is overwritten by OT.
						ndp2 := pmetric.NewNumberDataPoint()
						ndp.CopyTo(ndp2)
						switch metric.Name() {
						case opName:
							op.operations = ndp2
						case opTimeName:
							op.time = ndp2
						}
						newOp[key] = op
					}
				default:
					continue
				}
			}
			if len(newOp) == 0 {
				// No point making a new metric if there is no data.
				continue
			}
			// Generate a new metric from the operation count and time for each disk and direction.
			ndps := pmetric.NewNumberDataPointSlice()
			for key, new := range newOp {
				prev, prevOk := mtp.prevOp[key]
				t := new.time.DoubleValue()
				ops := new.operations.IntValue()
				if prevOk {
					t -= prev.time.DoubleValue()
					ops -= prev.operations.IntValue()
					ndp := ndps.AppendEmpty()
					new.time.CopyTo(ndp)
					if ops > 0 {
						interval := new.time.Timestamp() - prev.time.Timestamp()
						// Logic from https://github.com/Stackdriver/collectd/blob/2d176c650d9d6e4cd45d2add7977016c82dd8b55/src/disk.c#L321
						new.cumAvgTime += (t / float64(ops)) * float64(interval) / 1e9
					}
					ndp.SetDoubleValue(new.cumAvgTime)
				}
				mtp.prevOp[key] = new
			}
			if ndps.Len() > 0 {
				averageTimeMetric := metrics.AppendEmpty()
				opTimeMetric.CopyTo(averageTimeMetric)
				averageTimeMetric.SetName(metricPostfixRegex.ReplaceAllString(opTimeMetric.Name(), "average_operation_time"))
				ndps.CopyTo(averageTimeMetric.Sum().DataPoints())
			}
		}
	}

	return nil
}
