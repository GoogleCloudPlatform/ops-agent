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
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor/processorhelper"
	"go.opentelemetry.io/collector/processor/processortest"
	"go.uber.org/zap"
)

type testCase struct {
	name                      string
	input                     pmetric.Metrics
	expected                  pmetric.Metrics
	prevCPUTimeValuesInput    map[string]float64
	prevCPUTimeValuesExpected map[string]float64
	prevOpInput               map[opKey]opData
}

func TestAgentMetricsProcessor(t *testing.T) {
	tests := []testCase{
		{
			name:     "non-monotonic-sums-case",
			input:    generateNonMonotonicSumsInput(),
			expected: generateNonMonotonicSumsExpected(),
		},
		{
			name:     "remove-version-case",
			input:    generateVersionInput(),
			expected: generateVersionExpected(),
		},
		{
			name:     "remove--just-version-case",
			input:    generateMultiAttrVersionInput(),
			expected: generateMultiAttrVersionExpected(),
		},
		{
			name:     "process-resources-case",
			input:    generateProcessResourceMetricsInput(),
			expected: generateProcessResourceMetricsExpected(),
		},
		{
			name:     "read-write-split-case",
			input:    generateReadWriteMetricsInput(),
			expected: generateReadWriteMetricsExpected(),
		},
		{
			name:                      "utilization-case",
			input:                     generateUtilizationMetricsInput(),
			expected:                  generateUtilizationMetricsExpected(),
			prevCPUTimeValuesInput:    generateUtilizationPrevCPUTimeValuesInput(),
			prevCPUTimeValuesExpected: generateUtilizationPrevCPUTimeValuesExpected(),
		},
		{
			name:     "cpu-number-case",
			input:    generateCPUMetricsInput(),
			expected: generateCPUMetricsExpected(),
		},
		{
			name:     "average-disk",
			input:    generateAverageDiskInput(),
			expected: generateAverageDiskExpected(),
		},
		{
			name:        "average-disk-prev",
			input:       generateAverageDiskInput(),
			expected:    generateAverageDiskPrevExpected(),
			prevOpInput: generateAverageDiskPrevOpInput(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			amp := newAgentMetricsProcessor(zap.NewExample(), &Config{
				BlankLabelMetrics: []string{"system.cpu.time"},
			})

			tmn := &consumertest.MetricsSink{}
			rmp, err := processorhelper.NewMetrics(
				context.Background(),
				processortest.NewNopSettings(componentType),
				&Config{},
				tmn,
				amp.ProcessMetrics,
				processorhelper.WithCapabilities(processorCapabilities))
			require.NoError(t, err)
			assert.True(t, rmp.Capabilities().MutatesData)

			amp.prevCPUTimeValues = tt.prevCPUTimeValuesInput
			if tt.prevOpInput != nil {
				amp.prevOp = tt.prevOpInput
			}
			require.NoError(t, rmp.Start(context.Background(), componenttest.NewNopHost()))
			defer func() { assert.NoError(t, rmp.Shutdown(context.Background())) }()

			err = rmp.ConsumeMetrics(context.Background(), tt.input)
			require.NoError(t, err)

			marshaler := &pmetric.JSONMarshaler{}
			outJSON, err := marshaler.MarshalMetrics(tmn.AllMetrics()[0])
			require.NoError(t, err)
			t.Logf("actual metrics: %s", outJSON)

			assertEqual(t, tt.expected, tmn.AllMetrics()[0])
			if tt.prevCPUTimeValuesExpected != nil {
				assert.Equal(t, tt.prevCPUTimeValuesExpected, amp.prevCPUTimeValues)
			}
		})
	}
}

// builders to generate test metrics

type resourceMetricsBuilder struct {
	rms pmetric.ResourceMetricsSlice
}

func newResourceMetricsBuilder() resourceMetricsBuilder {
	return resourceMetricsBuilder{rms: pmetric.NewResourceMetricsSlice()}
}

func (rmsb resourceMetricsBuilder) addResourceMetrics(resourceAttributes map[string]pcommon.Value) metricsBuilder {
	rm := rmsb.rms.AppendEmpty()

	for k, v := range resourceAttributes {
		switch v.Type() {
		case pcommon.ValueTypeStr:
			rm.Resource().Attributes().PutStr(k, v.Str())
		case pcommon.ValueTypeInt:
			rm.Resource().Attributes().PutInt(k, v.Int())
		case pcommon.ValueTypeBool:
			rm.Resource().Attributes().PutBool(k, v.Bool())
		case pcommon.ValueTypeDouble:
			rm.Resource().Attributes().PutDouble(k, v.Double())
		}
	}

	ilm := rm.ScopeMetrics().AppendEmpty()

	return metricsBuilder{metrics: ilm.Metrics()}
}

func (rmsb resourceMetricsBuilder) Build() pmetric.ResourceMetricsSlice {
	return rmsb.rms
}

type metricsBuilder struct {
	metrics   pmetric.MetricSlice
	timestamp pcommon.Timestamp
}

func (msb metricsBuilder) addMetric(name string, t pmetric.MetricType, isMonotonic bool) metricBuilder {
	metric := msb.metrics.AppendEmpty()
	metric.SetName(name)

	switch t {
	case pmetric.MetricTypeSum:
		metric.SetEmptySum()
		sum := metric.Sum()
		sum.SetIsMonotonic(isMonotonic)
		sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	case pmetric.MetricTypeGauge:
		metric.SetEmptyGauge()
		metric.Gauge()
	}

	return metricBuilder{metric: metric, timestamp: msb.timestamp}
}

type metricBuilder struct {
	metric    pmetric.Metric
	timestamp pcommon.Timestamp
}

func (mb metricBuilder) addIntDataPoint(value int64, labels map[string]string) metricBuilder {
	var idp pmetric.NumberDataPoint
	switch mb.metric.Type() {
	case pmetric.MetricTypeSum:
		idp = mb.metric.Sum().DataPoints().AppendEmpty()
	case pmetric.MetricTypeGauge:
		idp = mb.metric.Gauge().DataPoints().AppendEmpty()
	}
	for k, v := range labels {
		idp.Attributes().PutStr(k, v)
	}
	idp.SetIntValue(value)
	idp.SetTimestamp(mb.timestamp)

	return mb
}

func (mb metricBuilder) addDoubleDataPoint(value float64, labels map[string]string) metricBuilder {
	var ddp pmetric.NumberDataPoint
	switch mb.metric.Type() {
	case pmetric.MetricTypeSum:
		ddp = mb.metric.Sum().DataPoints().AppendEmpty()
	case pmetric.MetricTypeGauge:
		ddp = mb.metric.Gauge().DataPoints().AppendEmpty()
	}
	for k, v := range labels {
		ddp.Attributes().PutStr(k, v)
	}
	ddp.SetDoubleValue(value)
	ddp.SetTimestamp(mb.timestamp)

	return mb
}

// assertEqual is required because we don't provide any guarantees on the order of transformed metrics
func assertEqual(t *testing.T, expected, actual pmetric.Metrics) {
	rmsAct := actual.ResourceMetrics()
	rmsExp := expected.ResourceMetrics()
	require.Equal(t, rmsExp.Len(), rmsAct.Len())
	for i := 0; i < rmsAct.Len(); i++ {
		rmAct := rmsAct.At(i)
		rmExp := rmsExp.At(i)

		// assert equality of resource attributes
		assert.Equal(t, rmExp.Resource().Attributes().AsRaw(), rmAct.Resource().Attributes().AsRaw())

		// assert equality of IL metrics
		ilmsAct := rmAct.ScopeMetrics()
		ilmsExp := rmExp.ScopeMetrics()
		require.Equal(t, ilmsExp.Len(), ilmsAct.Len())
		for j := 0; j < ilmsAct.Len(); j++ {
			ilmAct := ilmsAct.At(j)
			ilmExp := ilmsExp.At(j)

			// assert equality of metrics
			metricsAct := ilmAct.Metrics()
			metricsExp := ilmExp.Metrics()
			require.Equal(t, metricsExp.Len(), metricsAct.Len(), "Number of metrics")

			// build a map of expected metrics
			metricsExpMap := make(map[string]pmetric.Metric, metricsExp.Len())
			for k := 0; k < metricsExp.Len(); k++ {
				metricsExpMap[metricsExp.At(k).Name()] = metricsExp.At(k)
			}

			for k := 0; k < metricsAct.Len(); k++ {
				metricAct := metricsAct.At(k)
				metricExp, ok := metricsExpMap[metricAct.Name()]
				if !ok {
					require.Fail(t, fmt.Sprintf("unexpected metric %v", metricAct.Name()))
				}

				// assert equality of descriptors
				assert.Equal(t, metricExp.Name(), metricAct.Name())
				assert.Equalf(t, metricExp.Description(), metricAct.Description(), "Metric %s", metricAct.Name())
				assert.Equalf(t, metricExp.Unit(), metricAct.Unit(), "Metric %s", metricAct.Name())
				assert.Equalf(t, metricExp.Type(), metricAct.Type(), "Metric %s", metricAct.Name())

				// assert equality of aggregation info & data points
				switch ty := metricAct.Type(); ty {
				case pmetric.MetricTypeSum:
					assert.Equal(t, metricAct.Sum().AggregationTemporality(), metricExp.Sum().AggregationTemporality(), "Metric %s", metricAct.Name())
					assert.Equal(t, metricAct.Sum().IsMonotonic(), metricExp.Sum().IsMonotonic(), "Metric %s", metricAct.Name())
					assertEqualNumberDataPointSlice(t, metricAct.Name(), metricAct.Sum().DataPoints(), metricExp.Sum().DataPoints())
				case pmetric.MetricTypeGauge:
					assertEqualNumberDataPointSlice(t, metricAct.Name(), metricAct.Gauge().DataPoints(), metricExp.Gauge().DataPoints())
				default:
					assert.Fail(t, "unexpected metric type", t)
				}
			}
		}
	}
}

const epsilon = 0.0000000001

func assertEqualNumberDataPointSlice(t *testing.T, metricName string, ndpsAct, ndpsExp pmetric.NumberDataPointSlice) {
	require.Equalf(t, ndpsExp.Len(), ndpsAct.Len(), "Metric %s", metricName)

	// build a map of expected data points
	ndpsExpMap := make(map[string]pmetric.NumberDataPoint, ndpsExp.Len())
	for k := 0; k < ndpsExp.Len(); k++ {
		ndpsExpMap[labelsAsKey(ndpsExp.At(k).Attributes())] = ndpsExp.At(k)
	}

	for l := 0; l < ndpsAct.Len(); l++ {
		ndpAct := ndpsAct.At(l)

		key := labelsAsKey(ndpAct.Attributes())

		ndpExp, ok := ndpsExpMap[key]
		if !ok {
			require.Failf(t, fmt.Sprintf("no data point for %s", labelsAsKey(ndpAct.Attributes())), "Metric %s", metricName)
		}

		assert.Equalf(t, ndpExp.Attributes().AsRaw(), ndpAct.Attributes().AsRaw(), "Metric %s attributes %s", metricName, key)
		assert.Equalf(t, ndpExp.StartTimestamp(), ndpAct.StartTimestamp(), "Metric %s attributes %s", metricName, key)
		assert.Equalf(t, ndpExp.Timestamp(), ndpAct.Timestamp(), "Metric %s attributes %s", metricName, key)
		assert.Equalf(t, ndpExp.ValueType(), ndpAct.ValueType(), "Metric %s attributes %s", metricName, key)
		switch ndpExp.ValueType() {
		case pmetric.NumberDataPointValueTypeInt:
			assert.Equalf(t, ndpExp.IntValue(), ndpAct.IntValue(), "Metric %s attributes %s", metricName, key)
		case pmetric.NumberDataPointValueTypeDouble:
			assert.InEpsilonf(t, ndpExp.DoubleValue(), ndpAct.DoubleValue(), epsilon, "Metric %s attributes %s", metricName, key)
		}
	}
}
