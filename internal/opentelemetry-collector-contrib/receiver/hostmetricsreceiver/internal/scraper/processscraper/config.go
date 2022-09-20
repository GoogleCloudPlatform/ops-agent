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

package processscraper // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal/scraper/processscraper"

import (
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/coreinternal/processor/filterset"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal/scraper/processscraper/internal/metadata"
)

// Config relating to Process Metric Scraper.
type Config struct {
	// Metrics allows to customize scraped metrics representation.
	Metrics metadata.MetricsSettings `mapstructure:"metrics"`
	// Include specifies a filter on the process names that should be included from the generated metrics.
	// Exclude specifies a filter on the process names that should be excluded from the generated metrics.
	// If neither `include` or `exclude` are set, process metrics will be generated for all processes.
	Include MatchConfig `mapstructure:"include"`
	Exclude MatchConfig `mapstructure:"exclude"`

	// MuteProcessNameError is a flag that will mute the error encountered when trying to read a process the
	// collector does not have permission for.
	// See https://github.com/open-telemetry/opentelemetry-collector/issues/3004 for more information.
	MuteProcessNameError bool `mapstructure:"mute_process_name_error,omitempty"`

	// ScrapeProcessDelay is used to indicate the minimum amount of time a process must be running
	// before metrics are scraped for it.  The default value is 0 seconds (0s)
	ScrapeProcessDelay time.Duration `mapstructure:"scrape_process_delay"`
}

type MatchConfig struct {
	filterset.Config `mapstructure:",squash"`

	Names []string `mapstructure:"names"`
}
