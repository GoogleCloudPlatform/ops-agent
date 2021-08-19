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

package otel

import (
	"path"
	"sort"
)

// Helper functions to easily build up processor configs.

// MetricsFilter returns a Component that filters metrics.
// polarity should be "include" or "exclude".
// matchType should be "strict" or "regexp".
func MetricsFilter(polarity, matchType string, metricNames ...string) Component {
	return Component{
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

// MetricsTransform returns a Component that performs the transformations specified as arguments.
func MetricsTransform(metrics ...map[string]interface{}) Component {
	return Component{
		Type: "metricstransform",
		Config: map[string]interface{}{
			"transforms": metrics,
		},
	}
}

// NormalizeSums returns a Component that performs counter normalization.
func NormalizeSums() Component {
	return Component{
		Type:   "normalizesums",
		Config: map[string]interface{}{},
	}
}

// AddPrefix returns a config snippet that adds a prefix to all metrics.
func AddPrefix(prefix string) map[string]interface{} {
	// $ needs to be escaped because reasons.
	// https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/metricstransformprocessor#rename-multiple-metrics-using-substitution
	return map[string]interface{}{
		"include":    `^(.*)$$`,
		"match_type": "regexp",
		"action":     "update",
		"new_name":   path.Join(prefix, `$${1}`),
	}
}

// RenameMetric returns a config snippet that renames old to new, applying zero or more transformations.
func RenameMetric(old, new string, operations ...map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{
		"include":  old,
		"action":   "update",
		"new_name": new,
	}
	if len(operations) > 0 {
		out["operations"] = operations
	}
	return out
}

// DuplicateMetric returns a config snippet that copies old to new, applying zero or more transformations.
func DuplicateMetric(old, new string, operations ...map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{
		"include":  old,
		"action":   "insert",
		"new_name": new,
	}
	if len(operations) > 0 {
		out["operations"] = operations
	}
	return out
}

// CombineMetrics returns a config snippet that renames metrics matching the regex old to new, applying zero or more transformations.
func CombineMetrics(old, new string, operations ...map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{
		"include":       old,
		"match_type":    "regexp",
		"action":        "combine",
		"new_name":      new,
		"submatch_case": "lower",
	}
	if len(operations) > 0 {
		out["operations"] = operations
	}
	return out
}

// ToggleScalarDataType transforms int -> double and double -> int.
var ToggleScalarDataType = map[string]interface{}{"action": "toggle_scalar_data_type"}

// AddLabel adds a label with a fixed value.
func AddLabel(key, value string) map[string]interface{} {
	return map[string]interface{}{
		"action":    "add_label",
		"new_label": key,
		"new_value": value,
	}
}

// RenameLabel renames old to new
func RenameLabel(old, new string) map[string]interface{} {
	return map[string]interface{}{
		"action":    "update_label",
		"label":     old,
		"new_label": new,
	}
}

// RenameLabelValues renames label values
func RenameLabelValues(label string, transforms map[string]string) map[string]interface{} {
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

// DeleteLabelValue removes streams with the given label value
func DeleteLabelValue(label, value string) map[string]interface{} {
	return map[string]interface{}{
		"action":      "delete_label_value",
		"label":       label,
		"label_value": value,
	}
}

// ScaleValue multiplies the value by factor
func ScaleValue(factor float64) map[string]interface{} {
	return map[string]interface{}{
		"action":             "experimental_scale_value",
		"experimental_scale": factor,
	}
}

// AggregateLabels removes all labels except those in the passed list, aggregating values using aggregationType.
func AggregateLabels(aggregationType string, labels ...string) map[string]interface{} {
	return map[string]interface{}{
		"action":           "aggregate_labels",
		"label_set":        labels,
		"aggregation_type": aggregationType,
	}
}

// AggregateLabelValues combines the given values into a single value
func AggregateLabelValues(aggregationType string, label string, new string, old ...string) map[string]interface{} {
	return map[string]interface{}{
		"action":            "aggregate_label_values",
		"aggregation_type":  aggregationType,
		"label":             label,
		"new_value":         new,
		"aggregated_values": old,
	}
}
