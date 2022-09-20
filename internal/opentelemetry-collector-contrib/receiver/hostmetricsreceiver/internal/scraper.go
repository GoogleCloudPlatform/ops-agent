// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal"

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/receiver/scraperhelper"
	"go.opentelemetry.io/collector/service/featuregate"
)

const (
	EmitMetricsWithDirectionAttributeFeatureGateID    = "receiver.hostmetricsreceiver.emitMetricsWithDirectionAttribute"
	EmitMetricsWithoutDirectionAttributeFeatureGateID = "receiver.hostmetricsreceiver.emitMetricsWithoutDirectionAttribute"
)

var (
	emitMetricsWithDirectionAttributeFeatureGate = featuregate.Gate{
		ID:      EmitMetricsWithDirectionAttributeFeatureGateID,
		Enabled: true,
		Description: "Some process host metrics reported are transitioning from being reported with a direction " +
			"attribute to being reported with the direction included in the metric name to adhere to the " +
			"OpenTelemetry specification. This feature gate controls emitting the old metrics with the direction " +
			"attribute. For more details, see: " +
			"https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/receiver/hostmetricsreceiver/README.md#feature-gate-configurations",
	}

	emitMetricsWithoutDirectionAttributeFeatureGate = featuregate.Gate{
		ID:      EmitMetricsWithoutDirectionAttributeFeatureGateID,
		Enabled: false,
		Description: "Some process host metrics reported are transitioning from being reported with a direction " +
			"attribute to being reported with the direction included in the metric name to adhere to the " +
			"OpenTelemetry specification. This feature gate controls emitting the new metrics without the direction " +
			"attribute. For more details, see: " +
			"https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/receiver/hostmetricsreceiver/README.md#feature-gate-configurations",
	}
)

func init() {
	featuregate.GetRegistry().MustRegister(emitMetricsWithDirectionAttributeFeatureGate)
	featuregate.GetRegistry().MustRegister(emitMetricsWithoutDirectionAttributeFeatureGate)
}

// ScraperFactory can create a MetricScraper.
type ScraperFactory interface {
	// CreateDefaultConfig creates the default configuration for the Scraper.
	CreateDefaultConfig() Config

	// CreateMetricsScraper creates a scraper based on this config.
	// If the config is not valid, error will be returned instead.
	CreateMetricsScraper(ctx context.Context, settings component.ReceiverCreateSettings, cfg Config) (scraperhelper.Scraper, error)
}

// Config is the configuration of a scraper.
type Config interface {
}
