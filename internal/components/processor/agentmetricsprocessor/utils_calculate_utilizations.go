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
	"sort"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

// The following code calculates a new utilization metric from
// a usage metric across one label (dimension) using the formula:
//
// value{l1=v1,...} = value{l1=v1,...} / sum(value{l1=vx,...}) over x=1..N

const (
	cpuTime         = "system.cpu.time"
	memoryUsage     = "system.memory.usage"
	fileSystemUsage = "system.filesystem.usage"
	swapUsage       = "system.paging.usage"
)

var metricsToComputeUtilizationFor = map[string]bool{
	cpuTime:         true,
	memoryUsage:     true,
	fileSystemUsage: true,
	swapUsage:       true,
}

const stateLabel = "state"

func (mtp *agentMetricsProcessor) appendUtilizationMetrics(rms pmetric.ResourceMetricsSlice) error {
	for i := 0; i < rms.Len(); i++ {
		ilms := rms.At(i).ScopeMetrics()
		for j := 0; j < ilms.Len(); j++ {
			metrics := ilms.At(j).Metrics()
			for k := 0; k < metrics.Len(); k++ {
				metric := metrics.At(k)

				// ignore all metrics except the ones we want to compute utilizations for
				metricName := metric.Name()
				if !metricsToComputeUtilizationFor[metricName] {
					continue
				}

				// calculate new utilization metric and append it
				utilizationMetric, err := mtp.calculateUtilizationMetric(metric)
				if err != nil {
					return err
				}

				utilizationMetric.CopyTo(metrics.AppendEmpty())
			}
		}
	}

	return nil
}

func (mtp *agentMetricsProcessor) calculateUtilizationMetric(usageMetric pmetric.Metric) (pmetric.Metric, error) {
	utilizationMetric := pmetric.NewMetric()
	usageMetric.CopyTo(utilizationMetric)

	utilizationMetric.SetName(metricPostfixRegex.ReplaceAllString(usageMetric.Name(), "utilization"))
	utilizationMetric.SetEmptyGauge()
	utilizationMetric.Gauge()

	metric := usageMetric

	// for "cpu.time", we need to convert cumulative values to delta values before
	// computing utilization of the deltas
	isCPUTime := usageMetric.Name() == cpuTime
	if isCPUTime {
		delta := pmetric.NewMetric()
		usageMetric.CopyTo(delta)
		mtp.convertPrevCPUTimeToDelta(delta)
		metric = delta
	}

	switch t := metric.Type(); t {
	case pmetric.MetricTypeSum, pmetric.MetricTypeGauge:
		if err := calculateUtilizationFromNumberDataPoints(metric, utilizationMetric); err != nil {
			return pmetric.NewMetric(), err
		}
	default:
		return pmetric.NewMetric(), fmt.Errorf("unsupported metric data type: %v", t)
	}

	// persist the values of "cpu.time" so we can compute deltas on the next cycle
	if isCPUTime {
		mtp.setPrevCPUTimes(usageMetric)
	}

	return utilizationMetric, nil
}

// convertPrevCPUTimeToDelta converts the cpu.time values to delta values using the
// values persisted in the previous snapshot
func (mtp *agentMetricsProcessor) convertPrevCPUTimeToDelta(cpuTimeMetric pmetric.Metric) {
	mtp.mutex.Lock()
	defer mtp.mutex.Unlock()

	ndps := cpuTimeMetric.Sum().DataPoints()
	out := pmetric.NewNumberDataPointSlice()
	for i := 0; i < ndps.Len(); i++ {
		ndp := ndps.At(i)

		// if we have no previous value for this cpu/state combination,
		// remove the data point as we cannot calculate a utilization
		prevValue, ok := mtp.prevCPUTimeValues[labelsAsKey(ndp.Attributes())]
		if !ok {
			continue
		}

		// delta value = current cumulative value - previous cumulative value
		ndp2 := out.AppendEmpty()
		ndp.CopyTo(ndp2)
		ndp2.SetDoubleValue(ndp.DoubleValue() - prevValue)
	}
	// overwrite previous slice
	out.CopyTo(ndps)
}

// setPrevCPUTimes persists the cpu.time cumulative values as a map so they can
// be used to calculate deltas in the next snapshot
func (mtp *agentMetricsProcessor) setPrevCPUTimes(cpuTimeMetric pmetric.Metric) {
	mtp.mutex.Lock()
	defer mtp.mutex.Unlock()

	mtp.prevCPUTimeValues = doubleDataPointsToMap(cpuTimeMetric)
}

type numberPoints struct {
	pts []pmetric.NumberDataPoint
	sum float64
}

func calculateUtilizationFromNumberDataPoints(metric, utilizationMetric pmetric.Metric) error {
	var ndps pmetric.NumberDataPointSlice
	switch t := metric.Type(); t {
	case pmetric.MetricTypeSum:
		ndps = metric.Sum().DataPoints()
	case pmetric.MetricTypeGauge:
		ndps = metric.Gauge().DataPoints()
	}

	pointCount := ndps.Len()
	groupedPoints := make(map[string]*numberPoints, pointCount) // overallocate to ensure no resizes are required
	for i := 0; i < pointCount; i++ {
		ndp := ndps.At(i)

		key, err := otherLabelsAsKey(ndp.Attributes(), stateLabel)
		if err != nil {
			return fmt.Errorf("metric %v: %w", metric.Name(), err)
		}

		points, ok := groupedPoints[key]
		if !ok {
			points = &numberPoints{}
			groupedPoints[key] = points
		}

		switch ndp.ValueType() {
		case pmetric.NumberDataPointValueTypeInt:
			points.sum += float64(ndp.IntValue())
		case pmetric.NumberDataPointValueTypeDouble:
			points.sum += ndp.DoubleValue()
		}
		points.pts = append(points.pts, ndp)
	}

	ndps = pmetric.NewNumberDataPointSlice()
	ndps.EnsureCapacity(pointCount)
	for _, points := range groupedPoints {
		for _, point := range points.pts {
			ndp := ndps.AppendEmpty()

			// copy dp, setting the value based on utilization calculation
			point.Attributes().CopyTo(ndp.Attributes())
			ndp.SetStartTimestamp(point.StartTimestamp())
			ndp.SetTimestamp(point.Timestamp())
			var num float64
			switch point.ValueType() {
			case pmetric.NumberDataPointValueTypeInt:
				num = float64(point.IntValue())
			case pmetric.NumberDataPointValueTypeDouble:
				num = point.DoubleValue()
			}
			ndp.SetDoubleValue(num / points.sum * 100)
		}
	}
	ndps.CopyTo(utilizationMetric.Gauge().DataPoints())

	return nil
}

// doubleDataPointsToMap converts the double data points in the provided metric
// to a map of labels to values
func doubleDataPointsToMap(metric pmetric.Metric) map[string]float64 {
	ddps := metric.Sum().DataPoints()
	labelToValuesMap := make(map[string]float64, ddps.Len())
	for i := 0; i < ddps.Len(); i++ {
		ddp := ddps.At(i)
		key, _ := otherLabelsAsKey(ddp.Attributes())
		labelToValuesMap[key] = ddp.DoubleValue()
	}
	return labelToValuesMap
}

// labelsAsKey returns a key representing the labels in the provided labelset.
func labelsAsKey(labels pcommon.Map) string {
	otherLabelsLen := labels.Len()

	idx, otherLabels := 0, make([]string, otherLabelsLen)
	labels.Range(func(k string, v pcommon.Value) bool {
		otherLabels[idx] = k + "=" + v.AsString()
		idx++
		return true
	})

	// sort the slice so that we consider labelsets
	// the same regardless of order
	sort.StringSlice(otherLabels).Sort()
	return strings.Join(otherLabels, ";")
}

// otherLabelsAsKey returns a key representing the other labels in the provided
// labelset excluding the specified label keys. An error is returned if any of the
// specified labels to exclude do not exist in the labelset.
func otherLabelsAsKey(labels pcommon.Map, excluding ...string) (string, error) {
	otherLabelsLen := labels.Len() - len(excluding)

	otherLabels := make([]string, 0, otherLabelsLen)
	labels.Range(func(k string, v pcommon.Value) bool {
		// ignore any keys specified in excluding
		for _, e := range excluding {
			if k == e {
				return true
			}
		}

		otherLabels = append(otherLabels, fmt.Sprintf("%s=%s", k, v.AsString()))

		return true
	})

	if len(otherLabels) > otherLabelsLen {
		return "", fmt.Errorf("label set did not include all expected labels: %v", excluding)
	}

	// sort the slice so that we consider labelsets
	// the same regardless of order
	sort.StringSlice(otherLabels).Sort()
	return strings.Join(otherLabels, ";"), nil
}
