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

	MetricsPattern []string `yaml:"metrics_pattern,flow" validate:"dive,endswith=/*,startswith=agent.googleapis.com/"`
}

func (p MetricsProcessorExcludeMetrics) Type() string {
	return "exclude_metrics"
}

func (p MetricsProcessorExcludeMetrics) Processors() []otel.Component {
	var metricNames []string
	for _, glob := range p.MetricsPattern {
		// $ needs to be escaped because reasons.
		// https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/metricstransformprocessor#rename-multiple-metrics-using-substitution
		var literals []string
		for _, g := range strings.Split(glob, "*") {
			literals = append(literals, strings.Replace(regexp.QuoteMeta(g), "$", "$$", -1))
		}
		metricNames = append(metricNames, fmt.Sprintf(`^%s$$`, strings.Join(literals, `.*`)))
	}
	return []otel.Component{otel.MetricsFilter(
		"exclude",
		"regexp",
		metricNames...,
	)}
}

func init() {
	confgenerator.MetricsProcessorTypes.RegisterType(func() confgenerator.Component { return &MetricsProcessorExcludeMetrics{} })
}
