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
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/instrument"
	"go.opentelemetry.io/otel/sdk/metric/controller/basic"
	"go.opentelemetry.io/otel/sdk/metric/sdkapi"
	"go.opentelemetry.io/otel/sdk/resource"
)

var (
	agentMetricsPrefixFormat = "agent.googleapis.com/%s"
	formatter                = func(d *sdkapi.Descriptor) string {
		return fmt.Sprintf(agentMetricsPrefixFormat, d.Name())
	}
)

type enabledReceivers struct {
	metricsReceiverCountsByType map[string]int
	logsReceiverCountsByType    map[string]int
}

func CountEnabledReceivers(uc *confgenerator.UnifiedConfig) (enabledReceivers, error) {
	eR := enabledReceivers{
		metricsReceiverCountsByType: make(map[string]int),
		logsReceiverCountsByType:    make(map[string]int),
	}

	// Logging Pipelines
	for _, p := range uc.Logging.Service.Pipelines {
		for _, rID := range p.ReceiverIDs {
			rType := uc.Logging.Receivers[rID].Type()
			eR.logsReceiverCountsByType[rType] += 1
		}
	}

	// Metrics Pipelines
	for _, p := range uc.Metrics.Service.Pipelines {
		for _, rID := range p.ReceiverIDs {
			rType := uc.Metrics.Receivers[rID].Type()
			eR.metricsReceiverCountsByType[rType] += 1
		}
	}

	return eR, nil
}

func InstrumentEnabledReceiversMetric(uc *confgenerator.UnifiedConfig, meter metric.Meter) error {
	eR, err := CountEnabledReceivers(uc)
	if err != nil {
		return err
	}

	// Collect GAUGE metric
	gaugeObserver, err := meter.AsyncInt64().Gauge("agent/ops_agent/enabled_receivers")
	if err != nil {
		return fmt.Errorf("failed to initialize instrument: %v", err)
	}

	return meter.RegisterCallback([]instrument.Asynchronous{gaugeObserver}, func(ctx context.Context) {

		for rType, count := range eR.metricsReceiverCountsByType {
			labels := []attribute.KeyValue{
				attribute.String("telemetry_type", "metrics"),
				attribute.String("receiver_type", rType),
			}
			gaugeObserver.Observe(ctx, int64(count), labels...)
		}

		for rType, count := range eR.logsReceiverCountsByType {
			labels := []attribute.KeyValue{
				attribute.String("telemetry_type", "logs"),
				attribute.String("receiver_type", rType),
			}
			gaugeObserver.Observe(ctx, int64(count), labels...)
		}
	})
}

func CollectOpsAgentSelfMetrics(uc *confgenerator.UnifiedConfig, death chan bool) error {
	// Detect GCP credentials
	ctx := context.Background()
	res, err := resource.New(ctx,
		resource.WithDetectors(gcp.NewDetector()),
		resource.WithTelemetrySDK(),
	)

	resOpt := basic.WithResource(res)

	// GCP exporter options
	opts := []mexporter.Option{
		mexporter.WithMetricDescriptorTypeFormatter(formatter),
		mexporter.WithInterval(time.Minute),
	}

	// Create exporter pipeline
	pusher, err := mexporter.InstallNewPipeline(opts, resOpt)
	if err != nil {
		return fmt.Errorf("Failed to establish pipeline: %v", err)
	}
	defer pusher.Stop(ctx)

	// Start meter
	meter := pusher.Meter("ops_agent/self_metrics")

	err = InstrumentEnabledReceiversMetric(uc, meter)
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

	return nil
}
