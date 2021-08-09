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

package confgenerator

import (
	"fmt"
	"strings"
)

type MetricsProcessorExcludeMetrics struct {
	ConfigComponent `yaml:",inline"`

	MetricsPattern []string `yaml:"metrics_pattern,flow" validate:"required"`
}

func (r MetricsProcessorExcludeMetrics) Type() string {
	return "exclude_metrics"
}

func (p *MetricsProcessorExcludeMetrics) ValidateParameters(subagent string, kind string, id string) error {
	if err := validateParameters(*p, subagent, kind, id, p.Type()); err != nil {
		return err
	}
	var errors []string
	for _, prefix := range p.MetricsPattern {
		if !strings.HasSuffix(prefix, "/*") {
			errors = append(errors, fmt.Sprintf(`%q must end with "/*"`, prefix))
		}
		// TODO: Relax the prefix check when we support metrics with other prefixes.
		if !strings.HasPrefix(prefix, "agent.googleapis.com/") {
			errors = append(errors, fmt.Sprintf(`%q must start with "agent.googleapis.com/"`, prefix))
		}
	}
	if len(errors) > 0 {
		err := fmt.Errorf(strings.Join(errors, " | "))
		return fmt.Errorf(`%s has invalid value %q: %s`, parameterErrorPrefix(subagent, kind, id, p.Type(), "metrics_pattern"), p.MetricsPattern, err)
	}
	return nil
}

func init() {
	metricsProcessorTypes.registerType(func() component { return &MetricsProcessorExcludeMetrics{} })
}
