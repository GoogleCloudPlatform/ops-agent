// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal"

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func AssertContainsAttribute(t *testing.T, attr pcommon.Map, key string) {
	_, ok := attr.Get(key)
	assert.True(t, ok)
}

func AssertDescriptorEqual(t *testing.T, expected pmetric.Metric, actual pmetric.Metric) {
	assert.Equal(t, expected.Name(), actual.Name())
	assert.Equal(t, expected.Description(), actual.Description())
	assert.Equal(t, expected.Unit(), actual.Unit())
	assert.Equal(t, expected.DataType(), actual.DataType())
}

func AssertSumMetricHasAttributeValue(t *testing.T, metric pmetric.Metric, index int, labelName string, expectedVal pcommon.Value) {
	val, ok := metric.Sum().DataPoints().At(index).Attributes().Get(labelName)
	assert.Truef(t, ok, "Missing attribute %q in metric %q", labelName, metric.Name())
	assert.Equal(t, expectedVal, val)
}

func AssertSumMetricHasAttribute(t *testing.T, metric pmetric.Metric, index int, labelName string) {
	_, ok := metric.Sum().DataPoints().At(index).Attributes().Get(labelName)
	assert.Truef(t, ok, "Missing attribute %q in metric %q", labelName, metric.Name())
}

func AssertSumMetricStartTimeEquals(t *testing.T, metric pmetric.Metric, startTime pcommon.Timestamp) {
	ddps := metric.Sum().DataPoints()
	for i := 0; i < ddps.Len(); i++ {
		require.Equal(t, startTime, ddps.At(i).StartTimestamp())
	}
}

func AssertGaugeMetricHasAttributeValue(t *testing.T, metric pmetric.Metric, index int, labelName string, expectedVal pcommon.Value) {
	val, ok := metric.Gauge().DataPoints().At(index).Attributes().Get(labelName)
	assert.Truef(t, ok, "Missing attribute %q in metric %q", labelName, metric.Name())
	assert.Equal(t, expectedVal, val)
}

func AssertGaugeMetricHasAttribute(t *testing.T, metric pmetric.Metric, index int, labelName string) {
	_, ok := metric.Gauge().DataPoints().At(index).Attributes().Get(labelName)
	assert.Truef(t, ok, "Missing attribute %q in metric %q", labelName, metric.Name())
}

func AssertGaugeMetricStartTimeEquals(t *testing.T, metric pmetric.Metric, startTime pcommon.Timestamp) {
	ddps := metric.Gauge().DataPoints()
	for i := 0; i < ddps.Len(); i++ {
		require.Equal(t, startTime, ddps.At(i).StartTimestamp())
	}
}
func AssertSameTimeStampForAllMetrics(t *testing.T, metrics pmetric.MetricSlice) {
	AssertSameTimeStampForMetrics(t, metrics, 0, metrics.Len())
}

func AssertSameTimeStampForMetrics(t *testing.T, metrics pmetric.MetricSlice, startIdx, endIdx int) {
	var ts pcommon.Timestamp
	for i := startIdx; i < endIdx; i++ {
		metric := metrics.At(i)
		if metric.DataType() == pmetric.MetricDataTypeSum {
			ddps := metric.Sum().DataPoints()
			for j := 0; j < ddps.Len(); j++ {
				if ts == 0 {
					ts = ddps.At(j).Timestamp()
				}
				require.Equalf(t, ts, ddps.At(j).Timestamp(), "metrics contained different end timestamp values")
			}
		}
	}
}
