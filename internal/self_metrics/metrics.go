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
	"fmt"
	"log"
	"time"
	"context"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"go.opentelemetry.io/otel/sdk/metric/sdkapi"
	mexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel/sdk/metric/controller/basic"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/instrument"
)

var (
	agentMetricsPrefixFormat = "agent.googleapis.com/%s"
	formatter = func(d *sdkapi.Descriptor) string {
		return fmt.Sprintf(agentMetricsPrefixFormat, d.Name())
	}
)

type enabledReceivers struct {
	metric map[string]int
	log    map[string]int
}

func GetEnabledReceivers(uc *confgenerator.UnifiedConfig) (enabledReceivers, error) {
	eR := enabledReceivers{
		metric: make(map[string]int),
		log:    make(map[string]int),
	}

	// Logging Pipelines
	for _, p := range uc.Logging.Service.Pipelines {
		for _, rID := range p.ReceiverIDs {
			rType := uc.Logging.Receivers[rID].Type()
			eR.log[rType] += 1
		}
	}

	// Metrics Pipelines
	for _, p := range uc.Metrics.Service.Pipelines {
		for _, rID := range p.ReceiverIDs {
			rType := uc.Metrics.Receivers[rID].Type()
			eR.metric[rType] += 1
		}
	}

	return eR, nil
}

func CollectOpsAgentSelfMetrics(uc *confgenerator.UnifiedConfig, death chan bool) error {
	eR, err := GetEnabledReceivers(uc)
	if err != nil {
		return err
	}
	log.Println("Found Enabled Receivers : ", eR)

	opts := []mexporter.Option{
		mexporter.WithMetricDescriptorTypeFormatter(formatter),
		mexporter.WithInterval(time.Minute),
	}

	ctx := context.Background()

	res, err := resource.New(ctx,
        // Use the GCP resource detector to detect information about the GCP platform
        resource.WithDetectors(gcp.NewDetector()),
        // Keep the default detectors
        resource.WithTelemetrySDK(),
    )

    resOpt := basic.WithResource(res)

	pusher, err := mexporter.InstallNewPipeline(opts, resOpt)
	if err != nil {
		return fmt.Errorf("Failed to establish pipeline: %v", err)
	}
	defer pusher.Stop(ctx)

	// Start meter
	meter := pusher.Meter("cloudmonitoring/example")

	gaugeObserver, err := meter.AsyncInt64().Gauge("agent/ops_agent/enabled_receivers")
	if err != nil {
		return fmt.Errorf("failed to initialize instrument: %v", err)
	}
	_ = meter.RegisterCallback([]instrument.Asynchronous{gaugeObserver}, func(ctx context.Context) {
		
		for key, value := range eR.metric {
			labels := []attribute.KeyValue{
				attribute.String("telemetry_type", "metrics"),
				attribute.String("receiver_type", key),
			}
			gaugeObserver.Observe(ctx, int64(value), labels...)
		}

		for key, value := range eR.log {
			labels := []attribute.KeyValue{
				attribute.String("telemetry_type", "logs"),
				attribute.String("receiver_type", key),
			}
			gaugeObserver.Observe(ctx, int64(value), labels...)
		}
	})

	for {
		select {
		case <-death:
			break
		}
	}

	return nil
}
