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
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	conventions "go.opentelemetry.io/collector/semconv/v1.9.0"
)

// The following code performs a translation from metrics that store process
// information as resources to metrics that store process information as labels.
//
// Starting format:
//
//    ResourceMetrics         ResourceMetrics               ResourceMetrics
// +-------------------+ +-----------------------+     +-----------------------+
// |  Resource: Empty  | |  Resource: Process 1  |     |  Resource: Process X  |
// +-------+---+-------+ +---------+---+---------+ ... +---------+---+---------+
// |Metric1|...|MetricN| |MetricN+1|...|MetricN+M|     |MetricN+1|...|MetricN+M|
// +-------+---+-------+ +---------+---+---------+     +---------+---+---------+
//
// Converted format:
//
//                             ResourceMetrics
// +---------------------------------------------------------------------+
// |                           Resource: Empty                           |
// +-------+---+-------+----------------------+---+----------------------+
// |Metric1|...|MetricN|MetricN+1{P1, ..., PX}|...|MetricN+M{P1, ..., PX}|
// +-------+---+-------+----------------------+---+----------------------+
//
// Assumptions:
// * There is at most one resource metrics slice without process resource info (will raise error if not)
// * There is no other resource info supplied that needs to be retained (info may be silently lost if it exists)

func combineProcessMetrics(rms pmetric.ResourceMetricsSlice) error {
	// create collection of combined process metrics, disregarding any ResourceMetrics
	// with no process resource attributes as "otherMetrics"
	processMetrics, otherMetrics, err := createProcessMetrics(rms)
	if err != nil {
		return err
	}

	// if non-process specific metrics were supplied, initialize the result
	// with those metrics
	resultMetrics := otherMetrics

	// append all of the process metrics
	metrics := resultMetrics.At(0).ScopeMetrics().At(0).Metrics()
	for _, metric := range processMetrics {
		metric.Metric.CopyTo(metrics.AppendEmpty())
	}

	resultMetrics.CopyTo(rms)
	return nil
}

func createProcessMetrics(rms pmetric.ResourceMetricsSlice) (processMetrics convertedMetrics, otherMetrics pmetric.ResourceMetricsSlice, err error) {
	processMetrics = convertedMetrics{}
	otherMetrics = pmetric.NewResourceMetricsSlice()

	for i := 0; i < rms.Len(); i++ {
		rm := rms.At(i)
		resource := rm.Resource()

		// if these ResourceMetrics do not contain process resource attributes,
		// these must be the "other" non-process metrics
		if !includesProcessAttributes(resource) {
			rm.CopyTo(otherMetrics.AppendEmpty())
			continue
		}

		// combine all metrics into the process metrics map by appending
		// the data points
		ilms := rm.ScopeMetrics()
		for j := 0; j < ilms.Len(); j++ {
			metrics := ilms.At(j).Metrics()
			for k := 0; k < metrics.Len(); k++ {
				err = processMetrics.append(metrics.At(k), resource)
				if err != nil {
					return
				}
			}
		}
	}

	return
}

const processAttributePrefix = "process."

// includesProcessAttributes returns true if the resource includes
// any attributes with a "process." prefix
func includesProcessAttributes(resource pcommon.Resource) bool {
	includesProcessAttributes := false
	resource.Attributes().Range(func(k string, _ pcommon.Value) bool {
		if strings.HasPrefix(k, processAttributePrefix) {
			includesProcessAttributes = true
			return false
		}
		return true
	})
	return includesProcessAttributes
}

// convertedMetrics stores a map of metric names to converted metrics
// where convertedMetrics have process information stored as labels.
type convertedMetrics map[string]*convertedMetric

// append appends the data points associated with the provided metric to the
// associated converted metric (creating this metric if it doesn't exist yet),
// and appends the provided resource attributes as labels against these
// data points.
func (cms convertedMetrics) append(metric pmetric.Metric, resource pcommon.Resource) error {
	cm := cms.getOrCreate(metric)
	return cm.append(metric, resource)
}

// getOrCreate returns the converted metric associated with a given metric
// name (creating this metric if it doesn't exist yet).
func (cms convertedMetrics) getOrCreate(metric pmetric.Metric) *convertedMetric {
	// if we have an existing converted metric, return this
	metricName := metric.Name()
	if cm, ok := cms[metricName]; ok {
		return cm
	}

	// if there is no existing converted metric, create one using the
	// descriptor info from the provided metric
	cm := &convertedMetric{newMetric(metric)}
	cms[metricName] = cm
	return cm
}

// convertedMetric is a pmetric.Metric with process information stored as labels.
type convertedMetric struct {
	pmetric.Metric
}

// append appends the data points associated with the provided metric to the
// converted metric and appends the provided resource attributes as labels
// against these data points.
func (cm convertedMetric) append(metric pmetric.Metric, resource pcommon.Resource) error {
	var err error

	switch t := metric.Type(); t {
	case pmetric.MetricTypeSum:
		err = appendNumberDataSlice(metric.Sum().DataPoints(), cm.Sum().DataPoints(), resource)
	case pmetric.MetricTypeGauge:
		err = appendNumberDataSlice(metric.Gauge().DataPoints(), cm.Gauge().DataPoints(), resource)
	}

	return err
}

func appendNumberDataSlice(ndps, converted pmetric.NumberDataPointSlice, resource pcommon.Resource) error {
	for i := 0; i < ndps.Len(); i++ {
		err := appendAttributesToLabels(ndps.At(i).Attributes(), resource.Attributes())
		if err != nil {
			return err
		}
	}
	ndps.MoveAndAppendTo(converted)
	return nil
}

// appendAttributesToLabels appends the provided attributes to the provided labels map.
// This requires converting the attributes to string format.
func appendAttributesToLabels(labels pcommon.Map, attributes pcommon.Map) error {
	var err error
	attributes.Range(func(k string, v pcommon.Value) bool {
		// break if error has occurred in previous iteration
		if err != nil {
			return false
		}

		key := toCloudMonitoringLabel(k)
		// ignore attributes that do not map to a cloud ops label
		if key == "" {
			return true
		}

		var value string
		value, err = stringValue(v)
		// break if error
		if err != nil {
			return false
		}

		labels.PutStr(key, value)
		return true
	})
	return err
}

func toCloudMonitoringLabel(resourceAttributeKey string) string {
	// see https://cloud.google.com/monitoring/api/metrics_agent#agent-processes
	switch resourceAttributeKey {
	case conventions.AttributeProcessPID:
		return "pid"
	case conventions.AttributeProcessExecutableName:
		return "command"
	case conventions.AttributeProcessCommandLine:
		return "command_line"
	case conventions.AttributeProcessOwner:
		return "owner"
	default:
		return ""
	}
}

func stringValue(attributeValue pcommon.Value) (string, error) {
	var stringValue string
	switch t := attributeValue.Type(); t {
	case pcommon.ValueTypeBool:
		stringValue = strconv.FormatBool(attributeValue.Bool())
	case pcommon.ValueTypeInt:
		stringValue = strconv.FormatInt(attributeValue.Int(), 10)
	case pcommon.ValueTypeDouble:
		stringValue = strconv.FormatFloat(attributeValue.Double(), 'f', -1, 64)
	case pcommon.ValueTypeStr:
		stringValue = attributeValue.Str()
	default:
		return "", fmt.Errorf("unexpected attribute type: %v", t)
	}

	// cloud operations has a maximum label value length of 1024
	if len(stringValue) > 1024 {
		return stringValue[:1024], nil
	}

	return stringValue, nil
}
