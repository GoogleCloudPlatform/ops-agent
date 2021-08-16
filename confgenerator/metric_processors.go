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
	"regexp"
	"sort"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsProcessorExcludeMetrics struct {
	ConfigComponent `yaml:",inline"`

	MetricsPattern []string `yaml:"metrics_pattern,flow" validate:"dive,endswith=/*,startswith=agent.googleapis.com/"`
}

func (p MetricsProcessorExcludeMetrics) Type() string {
	return "exclude_metrics"
}

func (p MetricsProcessorExcludeMetrics) Processors() []otel.Component {
	var metricNames []string
	for _, glob := range p.MetricsPattern {
		// TODO: Remove TrimPrefix when we support metrics with other prefixes.
		glob = strings.TrimPrefix(glob, "agent.googleapis.com/")
		var literals []string
		for _, g := range strings.Split(glob, "*") {
			literals = append(literals, regexp.QuoteMeta(g))
		}
		metricNames = append(metricNames, fmt.Sprintf(`^%s$`, strings.Join(literals, `.*`)))
	}
	return []otel.Component{metricsFilter(
		"exclude",
		"regexp",
		metricNames...,
	)}
}

// metricsFilter returns an otel.Component that filters metrics.
// polarity should be "include" or "exclude.
// matchType should be "strict" or "regexp"
func metricsFilter(polarity, matchType string, metricNames ...string) otel.Component {
	return otel.Component{
		Type: "filter",
		Config: map[string]interface{}{
			"metrics": map[string]interface{}{
				polarity: map[string]interface{}{
					"match_type":   matchType,
					"metric_names": metricNames,
				},
			},
		},
	}
}

func init() {
	metricsProcessorTypes.registerType(func() component { return &MetricsProcessorExcludeMetrics{} })
}

// Helper methods to easily build up metricstransformprocessor configs.

// metricsTransform returns an otel.Component that performs tha transformations specified as arguments.
func metricsTransform(metrics ...map[string]interface{}) otel.Component {
	return otel.Component{
		Type: "metricstransform",
		Config: map[string]interface{}{
			"transforms": metrics,
		},
	}
}

// renameMetric returns a config snippet that renames old to new, applying zero or more transformations.
func renameMetric(old, new string, operations ...map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"metric_name": old,
		"action":      "update",
		"new_name":    new,
		"operations":  operations,
	}
}

// duplicateMetric returns a config snippet that copies old to new, applying zero or more transformations.
func duplicateMetric(old, new string, operations ...map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"metric_name": old,
		"action":      "insert",
		"new_name":    new,
		"operations":  operations,
	}
}

// combineMetrics returns a config snippet that renames metrics matching the regex old to new, applying zero or more transformations.
func combineMetrics(old, new string, operations ...map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"include":       old,
		"match_type":    "regexp",
		"action":        "combine",
		"new_name":      new,
		"submatch_case": "lower",
		"operations":    operations,
	}
}

// toggleScalarDataType transforms int -> double and double -> int.
var toggleScalarDataType = map[string]interface{}{"action": "toggle_scalar_data_type"}

// addLabel adds a label with a fixed value.
func addLabel(key, value string) map[string]interface{} {
	return map[string]interface{}{
		"action":    "add_label",
		"label":     key,
		"new_label": value,
	}
}

// renameLabel renames old to new
func renameLabel(old, new string) map[string]interface{} {
	return map[string]interface{}{
		"action":    "update_label",
		"label":     old,
		"new_label": new,
	}
}

// renameLabelValues renames label values
func renameLabelValues(label string, transforms map[string]string) map[string]interface{} {
	var actions []map[string]string
	for old, new := range transforms {
		actions = append(actions, map[string]string{
			"value":     old,
			"new_value": new,
		})
	}
	sort.Slice(actions, func(i, j int) bool {
		return actions[i]["value"] < actions[j]["value"]
	})
	return map[string]interface{}{
		"action":        "update_label",
		"label":         label,
		"value_actions": actions,
	}
}

// deleteLabelValue removes streams with the given label value
func deleteLabelValue(label, value string) map[string]interface{} {
	return map[string]interface{}{
		"action":      "delete_label_value",
		"label":       label,
		"label_value": value,
	}
}

// scaleValue multiplies the value by factor
func scaleValue(factor float64) map[string]interface{} {
	return map[string]interface{}{
		"action":             "experimental_scale_value",
		"experimental_scale": factor,
	}
}

// aggregateLabels removes all labels except those in the passed list, aggregating values using aggregationType.
func aggregateLabels(aggregationType string, labels ...string) map[string]interface{} {
	return map[string]interface{}{
		"action":           "aggregate_labels",
		"label_set":        labels,
		"aggregation_type": aggregationType,
	}
}

// aggregateLabelValues combines the given values into a single value
func aggregateLabelValues(aggregationType string, label string, new string, old ...string) map[string]interface{} {
	return map[string]interface{}{
		"action":            "aggregate_label_values",
		"aggregation_type":  aggregationType,
		"label":             label,
		"new_value":         new,
		"aggregated_values": old,
	}
}
