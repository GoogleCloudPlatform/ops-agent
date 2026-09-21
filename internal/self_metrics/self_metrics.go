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
	"path/filepath"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

const (
	agentMetricNamespace       string = "agent.googleapis.com"
	enabledReceiversMetricName string = "agent/ops_agent/enabled_receivers"
	featureTrackingMetricName  string = "agent/internal/ops/feature_tracking"
)

func getFullAgentMetricName(metricName string) string {
	return fmt.Sprintf("%s/%s", agentMetricNamespace, metricName)
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

func metricToJson(metrics pmetric.Metrics) ([]byte, error) {
	jsonMarshaler := &pmetric.JSONMarshaler{}
	jsonResult, err := jsonMarshaler.MarshalMetrics(metrics)
	if err != nil {
		return nil, err
	}
	return jsonResult, nil
}

func CollectEnabledReceiversMetricToOLTPJSON(ctx context.Context, uc *confgenerator.UnifiedConfig) ([]byte, error) {
	eR, err := CountEnabledReceivers(ctx, uc)
	if err != nil {
		return nil, err
	}

	metrics := pmetric.NewMetrics()
	resource := metrics.ResourceMetrics().AppendEmpty()

	// Temporarily add resource attributes. This will be properly populated
	// later in the pipeline by gce resource detector.
	resource.Resource().Attributes().PutStr("k", "v")

	gaugeMetric := resource.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	gaugeMetric.SetName(getFullAgentMetricName(enabledReceiversMetricName))
	dataPoints := gaugeMetric.SetEmptyGauge().DataPoints()

	// Sort map keys to always generate the same json output.
	for _, k := range confgenerator.GetSortedKeys(eR.MetricsReceiverCountsByType) {
		rType := k
		count := eR.MetricsReceiverCountsByType[k]
		point := dataPoints.AppendEmpty()
		point.SetIntValue(int64(count))
		attributes := point.Attributes()
		attributes.PutStr("telemetry_type", "metrics")
		attributes.PutStr("receiver_type", rType)
	}

	for _, k := range confgenerator.GetSortedKeys(eR.LogsReceiverCountsByType) {
		rType := k
		count := eR.LogsReceiverCountsByType[k]
		point := dataPoints.AppendEmpty()
		point.SetIntValue(int64(count))
		attributes := point.Attributes()
		attributes.PutStr("telemetry_type", "logs")
		attributes.PutStr("receiver_type", rType)
	}

	return metricToJson(metrics)
}

func CollectFeatureTrackingMetricToOTLPJSON(ctx context.Context, userUc, mergedUc *confgenerator.UnifiedConfig) ([]byte, error) {
	features, err := confgenerator.ExtractFeatures(ctx, userUc, mergedUc)
	if err != nil {
		return nil, err
	}

	metrics := pmetric.NewMetrics()
	resource := metrics.ResourceMetrics().AppendEmpty()
	resource.Resource().Attributes().PutStr("k", "v") // Resources can't be empty

	gaugeMetric := resource.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	gaugeMetric.SetName(getFullAgentMetricName(featureTrackingMetricName))
	dataPoints := gaugeMetric.SetEmptyGauge().DataPoints()

	for _, f := range features {
		point := dataPoints.AppendEmpty()
		point.SetIntValue(int64(1))
		attributes := point.Attributes()
		attributes.PutStr("module", f.Module)
		attributes.PutStr("feature", fmt.Sprintf("%s:%s", f.Kind, f.Type))
		attributes.PutStr("key", strings.Join(f.Key, "."))
		attributes.PutStr("value", f.Value)
	}

	return metricToJson(metrics)
}

// config and merged config respectively
func getUserAndMergedConfigs(ctx context.Context, userConfPath string) (*confgenerator.UnifiedConfig, *confgenerator.UnifiedConfig, error) {
	userUc, err := confgenerator.ReadUnifiedConfigFromFile(ctx, userConfPath)
	if err != nil {
		return nil, nil, err
	}
	if userUc == nil {
		userUc = &confgenerator.UnifiedConfig{}
	}

	mergedUc, err := confgenerator.MergeConfFiles(ctx, userConfPath)
	if err != nil {
		return nil, nil, err
	}

	return userUc, mergedUc, nil
}

func GenerateOpsAgentSelfMetricsOTLPJSON(ctx context.Context, config, outDir string) (err error) {
	userUc, mergedUc, err := getUserAndMergedConfigs(ctx, config)
	if err != nil {
		return err
	}

	featureTrackingOTLPJSON, err := CollectFeatureTrackingMetricToOTLPJSON(ctx, userUc, mergedUc)
	if err != nil {
		return fmt.Errorf("failed to generate feature tracking metric otlp json: %w", err)
	}
	if err = confgenerator.WriteConfigFile(featureTrackingOTLPJSON, filepath.Join(outDir, "feature_tracking_otlp.json")); err != nil {
		return fmt.Errorf("failed to write feature tracking metric otlp json file: %w", err)
	}

	enabledReceiverOTLPJSON, err := CollectEnabledReceiversMetricToOLTPJSON(ctx, mergedUc)
	if err != nil {
		return fmt.Errorf("failed to generate enabled receivers metric otlp json: %w", err)
	}
	if err = confgenerator.WriteConfigFile(enabledReceiverOTLPJSON, filepath.Join(outDir, "enabled_receivers_otlp.json")); err != nil {
		return fmt.Errorf("failed to write enabled receivers metric otlp json file: %w", err)
	}
	return nil
}
