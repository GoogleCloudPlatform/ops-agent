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
	"time"

	mexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel/attribute"
	metricapi "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/instrument"
	"go.opentelemetry.io/otel/metric/instrument/asyncint64"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
)

func agentMetricsPrefixFormatter(d metricdata.Metrics) string {
	return fmt.Sprintf("agent.googleapis.com/%s", d.Name)
}

type EnabledReceivers struct {
	MetricsReceiverCountsByType map[string]int
	LogsReceiverCountsByType    map[string]int
}

func CountEnabledReceivers(uc *confgenerator.UnifiedConfig) (EnabledReceivers, error) {
	eR := EnabledReceivers{
		MetricsReceiverCountsByType: make(map[string]int),
		LogsReceiverCountsByType:    make(map[string]int),
	}

	// Logging Pipelines
	for _, p := range uc.Logging.Service.Pipelines {
		err := countReceivers(eR.LogsReceiverCountsByType, p, uc.Logging.Receivers)
		if err != nil {
			return eR, err
		}
	}

	// Metrics Pipelines
	for _, p := range uc.Metrics.Service.Pipelines {
		err := countReceivers(eR.MetricsReceiverCountsByType, p, uc.Metrics.Receivers)
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

func AddGaugeObserver(meter metricapi.Meter, name string, function func(gaugeObserver asyncint64.Gauge, ctx context.Context)) error {
	gaugeObserver, err := meter.AsyncInt64().Gauge(name)
	if err != nil {
		return fmt.Errorf("failed to initialize instrument: %w", err)
	}
	err = meter.RegisterCallback([]instrument.Asynchronous{gaugeObserver}, func(ctx context.Context) {
		function(gaugeObserver, ctx)
	})
	if err != nil {
		return fmt.Errorf("failed to register callback: %w", err)
	}
	return nil
}

func InstrumentEnabledReceiversMetric(uc *confgenerator.UnifiedConfig, meter metricapi.Meter) error {
	eR, err := CountEnabledReceivers(uc)
	if err != nil {
		return err
	}

	err = AddGaugeObserver(meter, "agent/ops_agent/enabled_receivers", func(gaugeObserver asyncint64.Gauge, ctx context.Context) {
		for rType, count := range eR.MetricsReceiverCountsByType {
			labels := []attribute.KeyValue{
				attribute.String("telemetry_type", "metrics"),
				attribute.String("receiver_type", rType),
			}
			gaugeObserver.Observe(ctx, int64(count), labels...)
		}

		for rType, count := range eR.LogsReceiverCountsByType {
			labels := []attribute.KeyValue{
				attribute.String("telemetry_type", "logs"),
				attribute.String("receiver_type", rType),
			}
			gaugeObserver.Observe(ctx, int64(count), labels...)
		}
	})

	if err != nil {
		return fmt.Errorf("failed to register callback: %w", err)
	}
	return nil
}

func InstrumentFeatureTrackingMetric(uc *confgenerator.UnifiedConfig, meter metricapi.Meter) error {
	// Call Feature Extraction
	eR, err := CountEnabledReceivers(uc)
	if err != nil {
		return err
	}

	err = AddGaugeObserver(meter, "agent/internal/ops/feature_tracking", func(gaugeObserver asyncint64.Gauge, ctx context.Context) {
		for rType, _ := range eR.MetricsReceiverCountsByType {
			labels := []attribute.KeyValue{
				attribute.String("module", "metrics"),
				attribute.String("feature", rType),
				attribute.String("kind", rType),
				attribute.String("value", rType),
			}
			gaugeObserver.Observe(ctx, int64(1), labels...)
		}
	})

	if err != nil {
		return fmt.Errorf("failed to register callback: %w", err)
	}
	return nil
}

func CollectOpsAgentSelfMetrics(uc *confgenerator.UnifiedConfig, death chan bool) error {
	// Resource for GCP and SDK detectors
	ctx := context.Background()
	res, err := resource.New(ctx,
		resource.WithDetectors(gcp.NewDetector()),
		resource.WithTelemetrySDK(),
	)

	// Create exporter pipeline
	exporter, err := mexporter.New(
		mexporter.WithMetricDescriptorTypeFormatter(agentMetricsPrefixFormatter),
	)
	if err != nil {
		return fmt.Errorf("failed to create exporter: %w", err)
	}

	enableReceiverProvider, err := getEnabledReceiverProvider(uc, exporter, res)
	if err != nil {
		return err
	}

	featureTrackingProvider, err := getFeatureTrackingProvider(uc, exporter, res)
	if err != nil {
		return err
	}

waitForDeathSignal:
	for {
		select {
		case <-death:
			break waitForDeathSignal
		}
	}

	if err = enableReceiverProvider.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown meter provider: %w", err)
	}
	if err = featureTrackingProvider.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown meter provider: %w", err)
	}
	return nil
}

func getEnabledReceiverProvider(uc *confgenerator.UnifiedConfig, exporter metricsdk.Exporter, res *resource.Resource) (*metricsdk.MeterProvider, error) {
	// Create provider which periodically exports to the GCP exporter
	provider := metricsdk.NewMeterProvider(
		metricsdk.WithReader(metricsdk.NewPeriodicReader(exporter)),
		metricsdk.WithResource(res),
	)

	meter := provider.Meter("ops_agent/self_metrics")
	err := InstrumentEnabledReceiversMetric(uc, meter)
	return provider, err
}

func getFeatureTrackingProvider(uc *confgenerator.UnifiedConfig, exporter metricsdk.Exporter, res *resource.Resource) (*metricsdk.MeterProvider, error) {
	// Create provider which periodically exports to the GCP exporter
	provider := metricsdk.NewMeterProvider(
		metricsdk.WithReader(metricsdk.NewPeriodicReader(exporter, metricsdk.WithInterval(2*time.Hour))),
		metricsdk.WithResource(res),
	)

	meter := provider.Meter("ops_agent/feature_tracking")
	err := InstrumentFeatureTrackingMetric(uc, meter)
	return provider, err
}
