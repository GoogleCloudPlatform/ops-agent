// Copyright 2025 Google LLC
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

package self_metrics

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func agentMetricsPrefixFormatter(d metricdata.Metrics) string {
	return fmt.Sprintf("agent.googleapis.com/%s", d.Name)
}

type EnabledReceivers struct {
	MetricsReceiverCountsByType map[string]int
	LogsReceiverCountsByType    map[string]int
}

func CountEnabledReceivers(ctx context.Context, uc *confgenerator.UnifiedConfig) (EnabledReceivers, error) {
	eR := EnabledReceivers{
		MetricsReceiverCountsByType: make(map[string]int),
		LogsReceiverCountsByType:    make(map[string]int),
	}
	pipelines, err := uc.Pipelines(ctx)
	if err != nil {
		return eR, err
	}
	for _, p := range pipelines {
		pipelineType, receiverType := p.Types()
		if pipelineType == "metrics" {
			eR.MetricsReceiverCountsByType[receiverType] += 1
		} else if pipelineType == "logs" {
			eR.LogsReceiverCountsByType[receiverType] += 1
		}
	}

	return eR, nil
}

func InstrumentEnabledReceiversMetric(ctx context.Context, uc *confgenerator.UnifiedConfig) error {
	eR, err := CountEnabledReceivers(ctx, uc)
	if err != nil {
		return err
	}

	md := pmetric.NewMetrics()
	metric := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName("agent.googleapis.com/agent/ops_agent/enabled_receivers")
	dataPoints := metric.SetEmptyGauge().DataPoints()

	for rType, count := range eR.MetricsReceiverCountsByType {
		point := dataPoints.AppendEmpty()
		point.SetIntValue(int64(count))
		attributes := point.Attributes()
		attributes.PutStr("telemetry_type", "metrics")
		attributes.PutStr("receiver_type", rType)
	}

	for rType, count := range eR.LogsReceiverCountsByType {
		point := dataPoints.AppendEmpty()
		point.SetIntValue(int64(count))
		attributes := point.Attributes()
		attributes.PutStr("telemetry_type", "logs")
		attributes.PutStr("receiver_type", rType)
	}

	jsonMarshaler := &pmetric.JSONMarshaler{}
	json, err := jsonMarshaler.MarshalMetrics(md)
	if err != nil {
		return err
	}

	err = os.WriteFile("/tmp/enabledReceiversOTLP.json", json, 0644)
	if err != nil {
		return err
	}

	return nil
}

func InstrumentFeatureTrackingMetric(ctx context.Context, uc *confgenerator.UnifiedConfig) error {
	features, err := confgenerator.ExtractFeatures(ctx, uc)
	if err != nil {
		return err
	}

	md := pmetric.NewMetrics()
	metric := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName("agent.googleapis.com/agent/internal/ops/feature_tracking")
	dataPoints := metric.SetEmptyGauge().DataPoints()

	for _, f := range features {
		point := dataPoints.AppendEmpty()
		point.SetIntValue(int64(1))
		attributes := point.Attributes()
		attributes.PutStr("module", f.Module)
		attributes.PutStr("feature", fmt.Sprintf("%s:%s", f.Kind, f.Type))
		attributes.PutStr("key", strings.Join(f.Key, "."))
		attributes.PutStr("value", f.Value)
	}

	jsonMarshaler := &pmetric.JSONMarshaler{}
	json, err := jsonMarshaler.MarshalMetrics(md)
	if err != nil {
		return err
	}

	err = os.WriteFile("/tmp/featureTrackingOTLP.json", json, 0644)
	if err != nil {
		return err
	}

	return nil
}

func CollectOpsAgentSelfMetrics(ctx context.Context, userUc, mergedUc *confgenerator.UnifiedConfig) (err error) {
	err = InstrumentFeatureTrackingMetric(ctx, userUc)
	if err != nil {
		return fmt.Errorf("failed to instrument feature tracking: %w", err)
	}

	err = InstrumentEnabledReceiversMetric(ctx, mergedUc)
	if err != nil {
		return fmt.Errorf("failed to instrument enabled receivers: %w", err)
	}
	return nil
}
