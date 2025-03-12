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
	"log"
	"path/filepath"
	"strings"
	"time"

	mexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	"github.com/GoogleCloudPlatform/ops-agent/cmd/google_cloud_ops_agent_diagnostics/utils"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel/attribute"
	metricapi "go.opentelemetry.io/otel/metric"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	agentMetricNamespace       string = "agent.googleapis.com"
	enabledReceiversMetricName string = "agent/ops_agent/enabled_receivers"
	featureTrackingMetricName  string = "agent/internal/ops/feature_tracking"
)

func getFullAgentMetricName(metricName string) string {
	return fmt.Sprintf("%s/%s", agentMetricNamespace, metricName)
}

func agentMetricsPrefixFormatter(d metricdata.Metrics) string {
	return getFullAgentMetricName(d.Name)
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

func InstrumentEnabledReceiversMetric(ctx context.Context, uc *confgenerator.UnifiedConfig, meter metricapi.Meter) error {
	eR, err := CountEnabledReceivers(ctx, uc)
	if err != nil {
		return err
	}

	_, err = meter.Int64ObservableGauge(
		enabledReceiversMetricName,
		metricapi.WithInt64Callback(
			func(ctx context.Context, observer metricapi.Int64Observer) error {
				for rType, count := range eR.MetricsReceiverCountsByType {
					labels := []attribute.KeyValue{
						attribute.String("telemetry_type", "metrics"),
						attribute.String("receiver_type", rType),
					}
					observer.Observe(int64(count), metricapi.WithAttributes(labels...))
				}

				for rType, count := range eR.LogsReceiverCountsByType {
					labels := []attribute.KeyValue{
						attribute.String("telemetry_type", "logs"),
						attribute.String("receiver_type", rType),
					}
					observer.Observe(int64(count), metricapi.WithAttributes(labels...))
				}
				return nil
			}),
	)

	if err != nil {
		return err
	}
	return nil
}

func InstrumentFeatureTrackingMetric(ctx context.Context, userUc, mergedUc *confgenerator.UnifiedConfig, meter metricapi.Meter) error {
	features, err := confgenerator.ExtractFeatures(ctx, userUc, mergedUc)
	if err != nil {
		return err
	}
	_, err = meter.Int64ObservableGauge(
		featureTrackingMetricName,
		metricapi.WithInt64Callback(
			func(ctx context.Context, observer metricapi.Int64Observer) error {
				for _, f := range features {
					labels := []attribute.KeyValue{
						attribute.String("module", f.Module),
						attribute.String("feature", fmt.Sprintf("%s:%s", f.Kind, f.Type)),
						attribute.String("key", strings.Join(f.Key, ".")),
						attribute.String("value", f.Value),
					}
					observer.Observe(int64(1), metricapi.WithAttributes(labels...))
				}
				return nil
			}),
	)

	if err != nil {
		return err
	}
	return nil
}

func CreateFeatureTrackingMeterProvider(exporter metricsdk.Exporter, res *resource.Resource) *metricsdk.MeterProvider {
	provider := metricsdk.NewMeterProvider(
		metricsdk.WithReader(
			metricsdk.NewPeriodicReader(
				exporter,
				metricsdk.WithInterval(2*time.Hour),
			),
		),
		metricsdk.WithView(
			metricsdk.NewView(
				metricsdk.Instrument{
					Name: featureTrackingMetricName,
					Kind: metricsdk.InstrumentKindObservableGauge,
				},
				metricsdk.Stream{
					Name:        featureTrackingMetricName,
					Aggregation: metricsdk.AggregationDefault{},
				},
			)),
		metricsdk.WithResource(res),
	)
	return provider
}

func CreateEnabledReceiversMeterProvider(exporter metricsdk.Exporter, res *resource.Resource) *metricsdk.MeterProvider {
	provider := metricsdk.NewMeterProvider(
		metricsdk.WithReader(
			metricsdk.NewPeriodicReader(
				exporter,
			),
		),
		metricsdk.WithView(
			metricsdk.NewView(
				metricsdk.Instrument{
					Name: enabledReceiversMetricName,
					Kind: metricsdk.InstrumentKindObservableGauge,
				},
				metricsdk.Stream{
					Name:        enabledReceiversMetricName,
					Aggregation: metricsdk.AggregationDefault{},
				},
			)),
		metricsdk.WithResource(res),
	)
	return provider
}

func CollectOpsAgentSelfMetrics(ctx context.Context, userUc, mergedUc *confgenerator.UnifiedConfig) (err error) {

	// Resource for GCP and SDK detectors
	res, err := resource.New(ctx,
		resource.WithDetectors(gcp.NewDetector()),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	// Create exporter pipeline
	exporter, err := mexporter.New(
		mexporter.WithMetricDescriptorTypeFormatter(agentMetricsPrefixFormatter),
		mexporter.WithDisableCreateMetricDescriptors(),
	)
	if err != nil {
		return fmt.Errorf("failed to create exporter: %w", err)
	}

	featureTrackingProvider := CreateFeatureTrackingMeterProvider(exporter, res)
	err = InstrumentFeatureTrackingMetric(ctx, userUc, mergedUc, featureTrackingProvider.Meter("ops_agent/feature_tracking"))
	if err != nil {
		return fmt.Errorf("failed to instrument feature tracking: %w", err)
	}

	enabledReceiversProvider := CreateEnabledReceiversMeterProvider(exporter, res)
	err = InstrumentEnabledReceiversMetric(ctx, mergedUc, enabledReceiversProvider.Meter("ops_agent/self_metrics"))
	if err != nil {
		return fmt.Errorf("failed to instrument enabled receivers: %w", err)
	}

	defer func() {
		if serr := featureTrackingProvider.Shutdown(ctx); serr != nil {
			myStatus, ok := status.FromError(serr)
			if !ok && myStatus.Code() == codes.Unknown {
				log.Print(serr)
			} else if err == nil {
				err = fmt.Errorf("failed to shutdown meter provider: %w", serr)
			}
		}
		if serr := enabledReceiversProvider.Shutdown(ctx); serr != nil {
			myStatus, ok := status.FromError(serr)
			if !ok && myStatus.Code() == codes.Unknown {
				log.Print(serr)
			} else if err == nil {
				err = fmt.Errorf("failed to shutdown meter provider: %w", serr)
			}
		}
	}()

	timer := time.NewTimer(10 * time.Second)

	for {
		select {
		case <-timer.C:
			err := featureTrackingProvider.ForceFlush(ctx)
			if err != nil {
				log.Print(err)
			}
			err = enabledReceiversProvider.ForceFlush(ctx)
			if err != nil {
				log.Print(err)
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func CollectEnabledReceiversMetricToOLTPJSON(ctx context.Context, uc *confgenerator.UnifiedConfig) ([]byte, error) {
	eR, err := CountEnabledReceivers(ctx, uc)
	if err != nil {
		return nil, err
	}

	metrics := pmetric.NewMetrics()
	gaugeMetric := metrics.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	gaugeMetric.SetName(getFullAgentMetricName(enabledReceiversMetricName))
	dataPoints := gaugeMetric.SetEmptyGauge().DataPoints()

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
	json, err := jsonMarshaler.MarshalMetrics(metrics)
	if err != nil {
		return nil, err
	}

	return json, nil
}

func CollectFeatureTrackingMetricToOTLPJSON(ctx context.Context, userUc, mergedUc *confgenerator.UnifiedConfig) ([]byte, error) {
	features, err := confgenerator.ExtractFeatures(ctx, userUc, mergedUc)
	if err != nil {
		return nil, err
	}

	metrics := pmetric.NewMetrics()
	gaugeMetric := metrics.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
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

	jsonMarshaler := &pmetric.JSONMarshaler{}
	json, err := jsonMarshaler.MarshalMetrics(metrics)
	if err != nil {
		return nil, err
	}

	return json, nil
}

func GenerateOpsAgentSelfMetricsOTLPJSON(ctx context.Context, config, service, outDir string) (err error) {
	userUc, mergedUc, err := utils.GetUserAndMergedConfigs(ctx, config)
	if err != nil {
		return err
	}

	featureTrackingOTLPJSON, err := CollectFeatureTrackingMetricToOTLPJSON(ctx, userUc, mergedUc)
	if err != nil {
		return fmt.Errorf("failed to generate feature tracking metric otlp json: %w", err)
	}
	if err = confgenerator.WriteConfigFile(featureTrackingOTLPJSON, filepath.Join(outDir, "featureTrackingOTLP.json")); err != nil {
		return fmt.Errorf("failed to write feature tracking metric otlp json file: %w", err)
	}

	enabledReceiverOTLPJSON, err := CollectEnabledReceiversMetricToOLTPJSON(ctx, mergedUc)
	if err != nil {
		return fmt.Errorf("failed to generate enabled receivers metric otlp json: %w", err)
	}
	if err = confgenerator.WriteConfigFile(enabledReceiverOTLPJSON, filepath.Join(outDir, "enabledReceiversOTLP.json")); err != nil {
		return fmt.Errorf("failed to write enabled receivers metric otlp json file: %w", err)
	}
	return nil
}
