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

package networkscraper // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal/scraper/networkscraper"

import (
	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/coreinternal/processor/filterset"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal/scraper/networkscraper/internal/metadata"
)

// Config relating to Network Metric Scraper.
type Config struct {
	Metrics metadata.MetricsSettings `mapstructure:"metrics"`
	// Include specifies a filter on the network interfaces that should be included from the generated metrics.
	Include MatchConfig `mapstructure:"include"`
	// Exclude specifies a filter on the network interfaces that should be excluded from the generated metrics.
	Exclude MatchConfig `mapstructure:"exclude"`
}

type MatchConfig struct {
	filterset.Config `mapstructure:",squash"`

	Interfaces []string `mapstructure:"interfaces"`
}
