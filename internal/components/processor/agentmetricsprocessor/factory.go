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

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
)

// The value of "type" key in configuration.
var componentType component.Type = component.MustNewType("agentmetrics")

func NewFactory() processor.Factory {
	return processor.NewFactory(
		componentType,
		createDefaultConfig,
		processor.WithMetrics(createMetricsProcessor, component.StabilityLevelBeta))
}

func createDefaultConfig() component.Config {
	return &Config{}
}

var processorCapabilities = consumer.Capabilities{MutatesData: true}

func createMetricsProcessor(
	ctx context.Context,
	params processor.Settings,
	cfg component.Config,
	nextConsumer consumer.Metrics,
) (processor.Metrics, error) {
	// NewMetricsProcess takes an MProcessor, which is what agentMetricsProcessor implements, and returns a MetricsProcessor.
	mProcessor := newAgentMetricsProcessor(params.Logger, cfg.(*Config))
	return processorhelper.NewMetrics(
		ctx,
		params,
		cfg,
		nextConsumer,
		mProcessor.ProcessMetrics,
		processorhelper.WithCapabilities(processorCapabilities))
}
