// Copyright 2021 Google LLC
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

package apps

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsProcessorExcludeMetrics struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	MetricsPattern []string `yaml:"metrics_pattern,flow"`
}

func (p MetricsProcessorExcludeMetrics) Type() string {
	return "exclude_metrics"
}

func (p MetricsProcessorExcludeMetrics) Processors() []otel.Component {
	var metricNames []string
	for _, glob := range p.MetricsPattern {
		// $ needs to be escaped because reasons.
		// https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/metricstransformprocessor#rename-multiple-metrics-using-substitution
		metricNames = append(metricNames, strings.Replace(globToRegex(glob), "$", "$$", -1))
	}
	return []otel.Component{otel.MetricsFilter(
		"exclude",
		"regexp",
		metricNames...,
	)}
}

// globToRegex converts metrics glob patterns to regex patterns
func globToRegex(glob string) string {
	var literals []string
	for _, g := range strings.Split(glob, "*") {
		literals = append(literals, regexp.QuoteMeta(g))
	}
	return fmt.Sprintf(`^%s$`, strings.Join(literals, `.*`))

}

// AllMetricsExcluded checks if its MetricsPattern list can match all of the
// input metrics which would indicate all of the metrics will be excluded
func (p MetricsProcessorExcludeMetrics) AllMetricsExcluded(metrics ...string) bool {
OUTER:
	for _, metric := range metrics {
		for _, excludePattern := range p.MetricsPattern {
			if r, _ := regexp.MatchString(globToRegex(excludePattern), metric); r {
				continue OUTER
			}
		}
		return false
	}
	return true
}

func init() {
	confgenerator.MetricsProcessorTypes.RegisterType(func() confgenerator.MetricsProcessor { return &MetricsProcessorExcludeMetrics{} })
}
