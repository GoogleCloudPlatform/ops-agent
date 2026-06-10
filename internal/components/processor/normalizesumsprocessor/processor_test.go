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

package normalizesumsprocessor

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

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
	name     string
	inputs   []pmetric.Metrics
	expected []pmetric.Metrics
}

func TestNormalizeSumsProcessor(t *testing.T) {
	testStart := time.Now().Unix()
	tests := []testCase{
		{
			name:     "no-transform-case",
			inputs:   generateNoTransformMetrics(testStart),
			expected: generateNoTransformMetrics(testStart),
		},
		{
			name:     "removed-metric-case",
			inputs:   generateRemoveInput(testStart),
			expected: generateRemoveOutput(testStart),
		},
		{
			name:     "transform-all-happy-case",
			inputs:   generateLabelledInput(testStart),
			expected: generateLabelledOutput(testStart),
		},
		{
			name:     "transform-all-label-separated-case",
			inputs:   generateSeparatedLabelledInput(testStart),
			expected: generateSeparatedLabelledOutput(testStart),
		},
		{
			name:     "more-complex-case",
			inputs:   generateComplexInput(testStart),
			expected: generateComplexOutput(testStart),
		},
		{
			name:     "multiple-resource-case",
			inputs:   generateMultipleResourceInput(testStart),
			expected: generateMultipleResourceOutput(testStart),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nsp := newNormalizeSumsProcessor(zap.NewExample())

			tmn := &consumertest.MetricsSink{}
			rmp, err := processorhelper.NewMetrics(
				context.Background(),
				processortest.NewNopSettings(componentType),
				&Config{},
				tmn,
				nsp.ProcessMetrics,
				processorhelper.WithCapabilities(processorCapabilities))
			require.NoError(t, err)

			require.True(t, rmp.Capabilities().MutatesData)

			require.NoError(t, rmp.Start(context.Background(), componenttest.NewNopHost()))
			defer func() { require.NoError(t, rmp.Shutdown(context.Background())) }()

			for _, input := range tt.inputs {
				err = rmp.ConsumeMetrics(context.Background(), input)
				require.NoError(t, err)
			}

			requireEqual(t, tt.expected, tmn.AllMetrics())
		})
	}
}

func generateNoTransformMetrics(startTime int64) []pmetric.Metrics {
	input := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)

	mb1 := b.addMetric("m1", pmetric.MetricTypeSum, true)
	mb1.addIntDataPoint(1, map[string]string{}, startTime+1000, startTime)
	mb1.addIntDataPoint(5, map[string]string{}, startTime+2000, startTime)
	mb1.addIntDataPoint(2, map[string]string{}, startTime+3000, startTime+2000)

	mb2 := b.addMetric("m2", pmetric.MetricTypeSum, true)
	mb2.addDoubleDataPoint(3, map[string]string{}, startTime+6000, startTime)
	mb2.addDoubleDataPoint(4, map[string]string{}, startTime+7000, startTime)

	mb3 := b.addMetric("m3", pmetric.MetricTypeGauge, false)
	mb3.addIntDataPoint(5, map[string]string{}, startTime, 0)
	mb3.addIntDataPoint(4, map[string]string{}, startTime+1000, 0)

	mb4 := b.addMetric("m4", pmetric.MetricTypeGauge, false)
	mb4.addDoubleDataPoint(50000.2, map[string]string{}, startTime, 0)
	mb4.addDoubleDataPoint(11, map[string]string{}, startTime+1000, 0)

	rmb.Build().CopyTo(input.ResourceMetrics())
	return []pmetric.Metrics{input}
}

func generateMultipleResourceInput(startTime int64) []pmetric.Metrics {
	input := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(map[string]pcommon.Value{
		"label1": pcommon.NewValueStr("value1"),
	})

	mb1 := b.addMetric("m1", pmetric.MetricTypeSum, true)
	mb1.addIntDataPoint(1, map[string]string{}, startTime, 0)
	mb1.addIntDataPoint(2, map[string]string{}, startTime+1000, 0)

	b2 := rmb.addResourceMetrics(map[string]pcommon.Value{
		"label1": pcommon.NewValueStr("value2"),
	})

	mb2 := b2.addMetric("m1", pmetric.MetricTypeSum, true)
	mb2.addIntDataPoint(5, map[string]string{}, startTime+2000, 0)
	mb2.addIntDataPoint(10, map[string]string{}, startTime+3000, 0)

	rmb.Build().CopyTo(input.ResourceMetrics())
	return []pmetric.Metrics{input}
}

func generateMultipleResourceOutput(startTime int64) []pmetric.Metrics {
	output := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(map[string]pcommon.Value{
		"label1": pcommon.NewValueStr("value1"),
	})

	mb1 := b.addMetric("m1", pmetric.MetricTypeSum, true)
	// mb1.addIntDataPoint(1, map[string]string{}, startTime, 0)
	mb1.addIntDataPoint(1, map[string]string{}, startTime+1000, startTime)

	b2 := rmb.addResourceMetrics(map[string]pcommon.Value{
		"label1": pcommon.NewValueStr("value2"),
	})

	mb2 := b2.addMetric("m1", pmetric.MetricTypeSum, true)
	// mb2.addIntDataPoint(5, map[string]string{}, startTime+2000, 0)
	mb2.addIntDataPoint(5, map[string]string{}, startTime+3000, startTime+2000)

	rmb.Build().CopyTo(output.ResourceMetrics())
	return []pmetric.Metrics{output}
}

func generateLabelledInput(startTime int64) []pmetric.Metrics {
	input := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)

	mb1 := b.addMetric("m1", pmetric.MetricTypeSum, true)
	mb1.addIntDataPoint(0, map[string]string{"label": "val1"}, startTime, 0)
	mb1.addIntDataPoint(3, map[string]string{"label": "val2"}, startTime, 0)
	mb1.addIntDataPoint(12, map[string]string{"label": "val1"}, startTime+1000, 0)
	mb1.addIntDataPoint(5, map[string]string{"label": "val2"}, startTime+1000, 0)
	mb1.addIntDataPoint(15, map[string]string{"label": "val1"}, startTime+2000, 0)
	mb1.addIntDataPoint(1, map[string]string{"label": "val2"}, startTime+2000, 0)
	mb1.addIntDataPoint(22, map[string]string{"label": "val1"}, startTime+3000, 0)
	mb1.addIntDataPoint(11, map[string]string{"label": "val2"}, startTime+3000, 0)

	rmb.Build().CopyTo(input.ResourceMetrics())
	return []pmetric.Metrics{input}
}

func generateLabelledOutput(startTime int64) []pmetric.Metrics {
	output := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)

	mb1 := b.addMetric("m1", pmetric.MetricTypeSum, true)
	// mb1.addIntDataPoint(1, map[string]string{"label": "val1"}, startTime, 0)
	// mb1.addIntDataPoint(1, map[string]string{"label": "val2"}, startTime, 0)
	mb1.addIntDataPoint(12, map[string]string{"label": "val1"}, startTime+1000, startTime)
	mb1.addIntDataPoint(2, map[string]string{"label": "val2"}, startTime+1000, startTime)
	mb1.addIntDataPoint(15, map[string]string{"label": "val1"}, startTime+2000, startTime)
	// mb1.addIntDataPoint(1, map[string]string{"label": "val2"}, startTime+2000, 1)
	mb1.addIntDataPoint(22, map[string]string{"label": "val1"}, startTime+3000, startTime)
	mb1.addIntDataPoint(10, map[string]string{"label": "val2"}, startTime+3000, startTime+2000)

	rmb.Build().CopyTo(output.ResourceMetrics())
	return []pmetric.Metrics{output}
}

func generateSeparatedLabelledInput(startTime int64) []pmetric.Metrics {
	input := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)

	mb1 := b.addMetric("m1", pmetric.MetricTypeSum, true)
	mb1.addIntDataPoint(0, map[string]string{"label": "val1"}, startTime, 0)
	mb1.addIntDataPoint(12, map[string]string{"label": "val1"}, startTime+1000, 0)
	mb1.addIntDataPoint(15, map[string]string{"label": "val1"}, startTime+2000, 0)
	mb1.addIntDataPoint(22, map[string]string{"label": "val1"}, startTime+3000, 0)

	mb2 := b.addMetric("m1", pmetric.MetricTypeSum, true)
	mb2.addIntDataPoint(3, map[string]string{"label": "val2"}, startTime, 0)
	mb2.addIntDataPoint(5, map[string]string{"label": "val2"}, startTime+1000, 0)
	mb2.addIntDataPoint(1, map[string]string{"label": "val2"}, startTime+2000, 0)
	mb2.addIntDataPoint(11, map[string]string{"label": "val2"}, startTime+3000, 0)

	rmb.Build().CopyTo(input.ResourceMetrics())
	return []pmetric.Metrics{input}
}

func generateSeparatedLabelledOutput(startTime int64) []pmetric.Metrics {
	output := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)

	mb1 := b.addMetric("m1", pmetric.MetricTypeSum, true)
	// mb1.addIntDataPoint(1, map[string]string{"label": "val1"}, startTime, 0)
	mb1.addIntDataPoint(12, map[string]string{"label": "val1"}, startTime+1000, startTime)
	mb1.addIntDataPoint(15, map[string]string{"label": "val1"}, startTime+2000, startTime)
	mb1.addIntDataPoint(22, map[string]string{"label": "val1"}, startTime+3000, startTime)

	mb2 := b.addMetric("m1", pmetric.MetricTypeSum, true)
	// mb2.addIntDataPoint(1, map[string]string{"label": "val2"}, startTime, 0)
	mb2.addIntDataPoint(2, map[string]string{"label": "val2"}, startTime+1000, startTime)
	// mb2.addIntDataPoint(1, map[string]string{"label": "val2"}, startTime+2000, 1)
	mb2.addIntDataPoint(10, map[string]string{"label": "val2"}, startTime+3000, startTime+2000)

	rmb.Build().CopyTo(output.ResourceMetrics())
	return []pmetric.Metrics{output}
}

func generateRemoveInput(startTime int64) []pmetric.Metrics {
	input := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)

	mb1 := b.addMetric("m1", pmetric.MetricTypeSum, true)
	mb1.addIntDataPoint(1, map[string]string{}, startTime, 0)

	mb2 := b.addMetric("m2", pmetric.MetricTypeSum, true)
	mb2.addDoubleDataPoint(3, map[string]string{}, startTime, 0)
	mb2.addDoubleDataPoint(4, map[string]string{}, startTime+1000, 0)

	mb3 := b.addMetric("m3", pmetric.MetricTypeGauge, false)
	mb3.addDoubleDataPoint(5, map[string]string{}, startTime, 0)
	mb3.addDoubleDataPoint(6, map[string]string{}, startTime+1000, 0)

	rmb.Build().CopyTo(input.ResourceMetrics())
	return []pmetric.Metrics{input}
}

func generateRemoveOutput(startTime int64) []pmetric.Metrics {
	output := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)

	mb2 := b.addMetric("m2", pmetric.MetricTypeSum, true)
	// mb2.addDoubleDataPoint(3, map[string]string{}, startTime, 0)
	mb2.addDoubleDataPoint(1, map[string]string{}, startTime+1000, startTime)

	mb3 := b.addMetric("m3", pmetric.MetricTypeGauge, false)
	mb3.addDoubleDataPoint(5, map[string]string{}, startTime, 0)
	mb3.addDoubleDataPoint(6, map[string]string{}, startTime+1000, 0)

	rmb.Build().CopyTo(output.ResourceMetrics())
	return []pmetric.Metrics{output}
}

func generateComplexInput(startTime int64) []pmetric.Metrics {
	list := []pmetric.Metrics{}
	input := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)

	mb1 := b.addMetric("m1", pmetric.MetricTypeSum, true)
	mb1.addIntDataPoint(1, map[string]string{}, startTime, 0)
	mb1.addIntDataPoint(2, map[string]string{}, startTime+1000, 0)
	mb1.addIntDataPoint(2, map[string]string{}, startTime+2000, 0)
	mb1.addIntDataPoint(5, map[string]string{}, startTime+3000, 0)
	mb1.addIntDataPoint(2, map[string]string{}, startTime+4000, 0)
	mb1.addIntDataPoint(4, map[string]string{}, startTime+5000, 0)

	mb2 := b.addMetric("m2", pmetric.MetricTypeSum, true)
	mb2.addDoubleDataPoint(3, map[string]string{}, startTime, 0)
	mb2.addDoubleDataPoint(4, map[string]string{}, startTime+1000, 0)
	mb2.addDoubleDataPoint(5, map[string]string{}, startTime+2000, 0)
	mb2.addDoubleDataPoint(2, map[string]string{}, startTime, 0)
	mb2.addDoubleDataPoint(8, map[string]string{}, startTime+3000, 0)
	mb2.addDoubleDataPoint(2, map[string]string{}, startTime+10000, 0)
	mb2.addDoubleDataPoint(6, map[string]string{}, startTime+120000, 0)

	mb3 := b.addMetric("m3", pmetric.MetricTypeGauge, false)
	mb3.addDoubleDataPoint(5, map[string]string{}, startTime, 0)
	mb3.addDoubleDataPoint(6, map[string]string{}, startTime+1000, 0)

	mb4 := b.addMetric("m4", pmetric.MetricTypeSum, false)
	mb4.addDoubleDataPoint(12, map[string]string{}, startTime, 0)
	mb4.addDoubleDataPoint(13, map[string]string{}, startTime+2000, 0)

	rmb.Build().CopyTo(input.ResourceMetrics())
	list = append(list, input)

	input = pmetric.NewMetrics()
	rmb = newResourceMetricsBuilder()
	b = rmb.addResourceMetrics(nil)

	mb1 = b.addMetric("m1", pmetric.MetricTypeSum, true)
	mb1.addIntDataPoint(7, map[string]string{}, startTime+6000, 0)
	mb1.addIntDataPoint(9, map[string]string{}, startTime+7000, 0)

	rmb.Build().CopyTo(input.ResourceMetrics())
	list = append(list, input)

	return list
}

func generateComplexOutput(startTime int64) []pmetric.Metrics {
	list := []pmetric.Metrics{}
	output := pmetric.NewMetrics()

	rmb := newResourceMetricsBuilder()
	b := rmb.addResourceMetrics(nil)

	mb1 := b.addMetric("m1", pmetric.MetricTypeSum, true)
	// mb1.addIntDataPoint(1, map[string]string{}, startTime, 0)
	mb1.addIntDataPoint(1, map[string]string{}, startTime+1000, startTime)
	mb1.addIntDataPoint(1, map[string]string{}, startTime+2000, startTime)
	mb1.addIntDataPoint(4, map[string]string{}, startTime+3000, startTime)
	// mb1.addIntDataPoint(2, map[string]string{}, startTime+4000, 0)
	mb1.addIntDataPoint(2, map[string]string{}, startTime+5000, startTime+4000)

	mb2 := b.addMetric("m2", pmetric.MetricTypeSum, true)
	// mb2.addDoubleDataPoint(3, map[string]string{}, startTime, 0)
	mb2.addDoubleDataPoint(1, map[string]string{}, startTime+1000, startTime)
	mb2.addDoubleDataPoint(2, map[string]string{}, startTime+2000, startTime)
	// mb2.addDoubleDataPoint(2, map[string]string{}, startTime, 0)
	mb2.addDoubleDataPoint(5, map[string]string{}, startTime+3000, startTime)
	// mb2.addDoubleDataPoint(2, map[string]string{}, startTime+10000, 0)
	mb2.addDoubleDataPoint(4, map[string]string{}, startTime+120000, startTime+10000)

	mb3 := b.addMetric("m3", pmetric.MetricTypeGauge, false)
	mb3.addDoubleDataPoint(5, map[string]string{}, startTime, 0)
	mb3.addDoubleDataPoint(6, map[string]string{}, startTime+1000, 0)

	mb4 := b.addMetric("m4", pmetric.MetricTypeSum, false)
	mb4.addDoubleDataPoint(12, map[string]string{}, startTime, 0)
	mb4.addDoubleDataPoint(13, map[string]string{}, startTime+2000, 0)

	rmb.Build().CopyTo(output.ResourceMetrics())
	list = append(list, output)

	output = pmetric.NewMetrics()

	rmb = newResourceMetricsBuilder()
	b = rmb.addResourceMetrics(nil)

	mb1 = b.addMetric("m1", pmetric.MetricTypeSum, true)
	mb1.addIntDataPoint(5, map[string]string{}, startTime+6000, startTime+4000)
	mb1.addIntDataPoint(7, map[string]string{}, startTime+7000, startTime+4000)

	rmb.Build().CopyTo(output.ResourceMetrics())
	list = append(list, output)

	return list
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
	metrics pmetric.MetricSlice
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

	return metricBuilder{metric: metric}
}

type metricBuilder struct {
	metric pmetric.Metric
}

func (mb metricBuilder) addDoubleDataPoint(value float64, labels map[string]string, timestamp int64, startTimestamp int64) {
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
	ddp.SetTimestamp(pcommon.NewTimestampFromTime(time.Unix(timestamp, 0)))
	if startTimestamp > 0 {
		ddp.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Unix(startTimestamp, 0)))
	}
}

func (mb metricBuilder) addIntDataPoint(value int64, labels map[string]string, timestamp int64, startTimestamp int64) {
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
	idp.SetTimestamp(pcommon.NewTimestampFromTime(time.Unix(timestamp, 0)))
	if startTimestamp > 0 {
		idp.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Unix(startTimestamp, 0)))
	}
}

// requireEqual is required because Attribute & Label Maps are not sorted by default
// and we don't provide any guarantees on the order of transformed metrics
func requireEqual(t *testing.T, expected, actual []pmetric.Metrics) {
	require.Equal(t, len(expected), len(actual))

	marshaler := &pmetric.JSONMarshaler{}

	for q := 0; q < len(actual); q++ {
		outJSON, err := marshaler.MarshalMetrics(actual[q])
		require.NoError(t, err)
		t.Logf("actual metrics %d: %s", q, outJSON)

		rmsAct := actual[q].ResourceMetrics()
		rmsExp := expected[q].ResourceMetrics()
		require.Equal(t, rmsExp.Len(), rmsAct.Len())
		for i := 0; i < rmsAct.Len(); i++ {
			rmAct := rmsAct.At(i)
			rmExp := rmsExp.At(i)

			// require equality of resource attributes
			require.Equal(t, rmExp.Resource().Attributes().AsRaw(), rmAct.Resource().Attributes().AsRaw())

			// require equality of IL metrics
			ilmsAct := rmAct.ScopeMetrics()
			ilmsExp := rmExp.ScopeMetrics()
			require.Equal(t, ilmsExp.Len(), ilmsAct.Len())
			for j := 0; j < ilmsAct.Len(); j++ {
				ilmAct := ilmsAct.At(j)
				ilmExp := ilmsExp.At(j)

				// require equality of metrics
				metricsAct := ilmAct.Metrics()
				metricsExp := ilmExp.Metrics()
				require.Equal(t, metricsExp.Len(), metricsAct.Len())

				// build a map of expected metrics
				metricsExpMap := make(map[string]pmetric.Metric, metricsExp.Len())
				for k := 0; k < metricsExp.Len(); k++ {
					metricsExpMap[metricsExp.At(k).Name()] = metricsExp.At(k)
				}

				for k := 0; k < metricsAct.Len(); k++ {
					metricAct := metricsAct.At(k)
					metricExp := metricsExp.At(k)

					// require equality of descriptors
					require.Equal(t, metricExp.Name(), metricAct.Name())
					require.Equalf(t, metricExp.Description(), metricAct.Description(), "Metric %s", metricAct.Name())
					require.Equalf(t, metricExp.Unit(), metricAct.Unit(), "Metric %s", metricAct.Name())
					require.Equalf(t, metricExp.Type(), metricAct.Type(), "Metric %s", metricAct.Name())

					// require equality of aggregation info & data points
					switch ty := metricAct.Type(); ty {
					case pmetric.MetricTypeSum:
						require.Equal(t, metricAct.Sum().AggregationTemporality(), metricExp.Sum().AggregationTemporality(), "Metric %s", metricAct.Name())
						require.Equal(t, metricAct.Sum().IsMonotonic(), metricExp.Sum().IsMonotonic(), "Metric %s", metricAct.Name())
						requireEqualNumberDataPointSlice(t, metricAct.Name(), metricAct.Sum().DataPoints(), metricExp.Sum().DataPoints())
					case pmetric.MetricTypeGauge:
						requireEqualNumberDataPointSlice(t, metricAct.Name(), metricAct.Gauge().DataPoints(), metricExp.Gauge().DataPoints())
					default:
						require.Fail(t, "unexpected metric type", t)
					}
				}
			}
		}
	}
}

func requireEqualNumberDataPointSlice(t *testing.T, metricName string, ndpsAct, ndpsExp pmetric.NumberDataPointSlice) {
	require.Equalf(t, ndpsExp.Len(), ndpsAct.Len(), "Metric %s", metricName)

	// build a map of expected data points
	ndpsExpMap := make(map[string]pmetric.NumberDataPoint, ndpsExp.Len())
	for k := 0; k < ndpsExp.Len(); k++ {
		ndpExp := ndpsExp.At(k)
		ndpsExpMap[dataPointKey(metricName, ndpExp.Attributes(), ndpExp.Timestamp(), ndpExp.StartTimestamp())] = ndpExp
	}

	for l := 0; l < ndpsAct.Len(); l++ {
		ndpAct := ndpsAct.At(l)
		dpKey := dataPointKey(metricName, ndpAct.Attributes(), ndpAct.Timestamp(), ndpAct.StartTimestamp())

		ndpExp, ok := ndpsExpMap[dpKey]
		if !ok {
			require.Failf(t, fmt.Sprintf("no data point for %s", dpKey), "Metric %s", metricName)
		}

		require.Equalf(t, ndpExp.Attributes().AsRaw(), ndpAct.Attributes().AsRaw(), "Metric %s", metricName)
		require.Equalf(t, ndpExp.StartTimestamp(), ndpAct.StartTimestamp(), "Metric %s", metricName)
		require.Equalf(t, ndpExp.Timestamp(), ndpAct.Timestamp(), "Metric %s", metricName)
		require.Equalf(t, ndpExp.ValueType(), ndpAct.ValueType(), "Metric %s", metricName)
		switch ndpExp.ValueType() {
		case pmetric.NumberDataPointValueTypeInt:
			require.Equalf(t, ndpExp.IntValue(), ndpAct.IntValue(), "Metric %s", metricName)
		case pmetric.NumberDataPointValueTypeDouble:
			require.Equalf(t, ndpExp.DoubleValue(), ndpAct.DoubleValue(), "Metric %s", metricName)
		}
	}
}

// dataPointKey returns a key representing the data point
func dataPointKey(metricName string, labelsMap pcommon.Map, timestamp pcommon.Timestamp, startTimestamp pcommon.Timestamp) string {
	idx, otherLabels := 0, make([]string, labelsMap.Len())
	labelsMap.Range(func(k string, v pcommon.Value) bool {
		otherLabels[idx] = k + "=" + v.AsString()
		idx++
		return true
	})
	// sort the slice so that we consider labelsets
	// the same regardless of order
	sort.StringSlice(otherLabels).Sort()
	return metricName + "/" + startTimestamp.String() + "-" + timestamp.String() + "/" + strings.Join(otherLabels, ";")
}
