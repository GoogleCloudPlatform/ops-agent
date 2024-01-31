// Copyright 2022 Google LLC
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
	"strings"
	"time"

	mexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel/attribute"
	metricapi "go.opentelemetry.io/otel/metric"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/aggregation"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	metricsReceivers, err := uc.MetricsReceivers()
	if err != nil {
		return eR, err
	}
	loggingReceivers, err := uc.AllLoggingReceivers(ctx)
	if err != nil {
		return eR, err
	}

	// Logging Pipelines
	for _, p := range uc.Logging.Service.Pipelines {
		err := countReceivers(eR.LogsReceiverCountsByType, p, loggingReceivers)
		if err != nil {
			return eR, err
		}
	}

	// Metrics Pipelines
	for _, p := range uc.Metrics.Service.Pipelines {
		err := countReceivers(eR.MetricsReceiverCountsByType, p, metricsReceivers)
		if err != nil {
			return eR, err
		}
	}

	return eR, nil
}

func countReceivers[C confgenerator.Component](receiverCounts map[string]int, p *confgenerator.Pipeline, receivers map[string]C) error {
	for _, rID := range p.ReceiverIDs {
		if r, ok := receivers[rID]; ok {
			receiverCounts[r.Type()] += 1
		} else {
			return fmt.Errorf("receiver id %s not found in unified config", rID)
		}
	}
	return nil
}

func InstrumentEnabledReceiversMetric(ctx context.Context, uc *confgenerator.UnifiedConfig, meter metricapi.Meter) error {
	eR, err := CountEnabledReceivers(ctx, uc)
	if err != nil {
		return err
	}

	_, err = meter.Int64ObservableGauge(
		"agent/ops_agent/enabled_receivers",
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

func InstrumentFeatureTrackingMetric(uc *confgenerator.UnifiedConfig, meter metricapi.Meter) error {
	features, err := confgenerator.ExtractFeatures(uc)
	if err != nil {
		return err
	}
	_, err = meter.Int64ObservableGauge(
		"agent/internal/ops/feature_tracking",
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
					Name: "agent/internal/ops/feature_tracking",
					Kind: metricsdk.InstrumentKindObservableGauge,
				},
				metricsdk.Stream{
					Name:        "agent/internal/ops/feature_tracking",
					Aggregation: aggregation.Default{},
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
					Name: "agent/ops_agent/enabled_receivers",
					Kind: metricsdk.InstrumentKindObservableGauge,
				},
				metricsdk.Stream{
					Name:        "agent/ops_agent/enabled_receivers",
					Aggregation: aggregation.Default{},
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
	err = InstrumentFeatureTrackingMetric(userUc, featureTrackingProvider.Meter("ops_agent/feature_tracking"))
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
