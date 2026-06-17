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
	"fmt"

	"go.opentelemetry.io/collector/pdata/pmetric"
)

// The following code splits metrics with read/write direction labels into
// two separate metrics.
//
// Starting format:
//
// +-----------------------------------------------------+
// |                     metric                          |
// +---------+---+---------+------------+---+------------+
// |dp1{read}|...|dpN{read}|dpN+1{write}|...|dpN+N{write}|
// +---------+---+---------+------------+---+------------+
//
// Converted format:
//
// +-----------+ +---------------+
// |read_metric| | write_metric  |
// +---+---+---+ +-----+---+-----+
// |dp1|...|dpN| |dpN+1|...|dpN+N|
// +---+---+---+ +-----+---+-----+

const (
	hostDiskBytes    = "system.disk.io"
	processDiskBytes = "process.disk.io"
)

var metricsToSplit = map[string]bool{
	hostDiskBytes:    true,
	processDiskBytes: true,
}

func splitReadWriteBytesMetrics(rms pmetric.ResourceMetricsSlice) error {
	for i := 0; i < rms.Len(); i++ {
		ilms := rms.At(i).ScopeMetrics()
		for j := 0; j < ilms.Len(); j++ {
			metrics := ilms.At(j).Metrics()
			for k := 0; k < metrics.Len(); k++ {
				metric := metrics.At(k)

				// ignore all metrics except "disk.io" metrics
				metricName := metric.Name()
				if _, ok := metricsToSplit[metricName]; !ok {
					continue
				}

				// split into read and write metrics
				read, write, err := splitReadWriteBytesMetric(metric)
				if err != nil {
					return err
				}

				// append the new metrics to the collection, overwriting the old one
				read.CopyTo(metric)
				write.CopyTo(metrics.AppendEmpty())
			}
		}
	}

	return nil
}

const (
	directionLabel = "direction"
	readDirection  = "read"
	writeDirection = "write"
)

func splitReadWriteBytesMetric(metric pmetric.Metric) (read pmetric.Metric, write pmetric.Metric, err error) {
	// create new read & write metrics with descriptor & name including "read_" & "write_" prefix respectively
	read = newMetricWithName(metric, metricPostfixRegex.ReplaceAllString(metric.Name(), "read_$1"))
	write = newMetricWithName(metric, metricPostfixRegex.ReplaceAllString(metric.Name(), "write_$1"))

	// append data points to the read or write metric as appropriate
	switch t := metric.Type(); t {
	case pmetric.MetricTypeSum:
		err = appendNumberDataPoints(metric.Name(), metric.Sum().DataPoints(), read.Sum().DataPoints(), write.Sum().DataPoints())
	case pmetric.MetricTypeGauge:
		err = appendNumberDataPoints(metric.Name(), metric.Gauge().DataPoints(), read.Gauge().DataPoints(), write.Gauge().DataPoints())
	default:
		return read, write, fmt.Errorf("unsupported metric data type: %v", t)
	}

	return read, write, err
}

func appendNumberDataPoints(metricName string, ndps, read, write pmetric.NumberDataPointSlice) error {
	for i := 0; i < ndps.Len(); i++ {
		ndp := ndps.At(i)
		labels := ndp.Attributes()

		dir, ok := labels.Get(directionLabel)
		if !ok {
			return fmt.Errorf("metric %v did not contain %v label as expected", metricName, directionLabel)
		}

		var ndpNew pmetric.NumberDataPoint
		switch dir.Str() {
		case readDirection:
			ndpNew = read.AppendEmpty()
		case writeDirection:
			ndpNew = write.AppendEmpty()
		default:
			return fmt.Errorf("metric %v label %v contained unexpected value %v", metricName, directionLabel, dir.AsString())
		}
		ndp.CopyTo(ndpNew)
		ndpNew.Attributes().Remove(directionLabel)
	}

	return nil
}
