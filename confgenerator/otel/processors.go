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
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel/ottl"
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

// MetricsOTTLFilter returns a Component that filters metrics using OTTL.
// OTTL can only be used as an exclude filter, any metrics or datapoints that match
// one of the provided queries are dropped.
// Example query: 'name == "jvm.memory.heap.used" and resource.attributes["elasticsearch.node.name"] == nil'
func MetricsOTTLFilter(metricQueries []string, datapointQueries []string) Component {
	metricsConfig := map[string]interface{}{}

	if len(metricQueries) > 0 {
		metricsConfig["metric"] = metricQueries
	}
	if len(datapointQueries) > 0 {
		metricsConfig["datapoint"] = metricQueries
	}

	return Component{
		Type: "filter",
		Config: map[string]interface{}{
			"metrics": metricsConfig,
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

// CastToSum returns a Component that performs a cast of each metric to a sum.
func CastToSum(metrics ...string) Component {
	return Component{
		Type: "casttosum",
		Config: map[string]interface{}{
			"metrics": metrics,
		},
	}
}

// CumulativeToDelta returns a Component that converts each cumulative metric to delta.
func CumulativeToDelta(metrics ...string) Component {
	return Component{
		Type: "cumulativetodelta",
		Config: map[string]interface{}{
			"include": map[string]interface{}{
				"metrics":    metrics,
				"match_type": "strict",
			},
		},
	}
}

// DeltaToRate returns a Component that converts each delta metric to a gauge rate.
func DeltaToRate(metrics ...string) Component {
	return Component{
		Type: "deltatorate",
		Config: map[string]interface{}{
			"metrics": metrics,
		},
	}
}

// AddPrefix returns a config snippet that adds a domain prefix to all metrics.
func AddPrefix(prefix string, operations ...map[string]interface{}) map[string]interface{} {
	return RegexpRename(
		`^(.*)$`,
		path.Join(prefix, `${1}`),
		operations...,
	)
}

// ChangePrefix returns a config snippet that updates a prefix on all metrics.
func ChangePrefix(oldPrefix, newPrefix string) map[string]interface{} {
	return RegexpRename(
		fmt.Sprintf(`^%s(.*)$`, oldPrefix),
		fmt.Sprintf("%s%s", newPrefix, `${1}`),
	)
}

// RegexpRename returns a config snippet that renames metrics matching the given regexp.
// The `rename` argument supports capture groups as `${1}`, `${2}`, and so on.
func RegexpRename(regexp string, rename string, operations ...map[string]interface{}) map[string]interface{} {
	// $ needs to be escaped because reasons.
	// https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/metricstransformprocessor#rename-multiple-metrics-using-substitution
	out := map[string]interface{}{
		"include":    strings.ReplaceAll(regexp, "$", "$$"),
		"match_type": "regexp",
		"action":     "update",
		"new_name":   strings.ReplaceAll(rename, "$", "$$"),
	}

	if len(operations) > 0 {
		out["operations"] = operations
	}

	return out
}

// Transform returns a transform processor object that executes statements on statementType data.
func Transform(statementType, context string, statements ottl.Statements) Component {
	return Component{
		Type: "transform",
		Config: map[string]any{
			"error_mode": "ignore",
			fmt.Sprintf("%s_statements", statementType): []map[string]any{
				{
					"context":    context,
					"statements": statements,
				},
			},
		},
	}
}

// Filter returns a filter processor object that drops dataType.context data matching any of the expressions.
func Filter(dataType, context string, expressions []ottl.Value) Component {
	var strings []string
	for _, e := range expressions {
		strings = append(strings, e.String())
	}
	return Component{
		Type: "filter",
		Config: map[string]any{
			"error_mode": "ignore",
			dataType: map[string]any{
				context: strings,
			},
		},
	}
}

// TransformationMetrics returns a transform processor object that contains all the queries passed into it.
func TransformationMetrics(queries ...TransformQuery) Component {
	metricQueryStrings := []string{}
	datapointQueryStrings := []string{}
	for _, q := range queries {
		switch q.Context {
		case Metric:
			metricQueryStrings = append(metricQueryStrings, string(q.Statement))
		default:
			datapointQueryStrings = append(datapointQueryStrings, string(q.Statement))
		}
	}

	// Only add metricStatement type if any queuries to go with it
	metricStatements := []map[string]any{}
	if len(metricQueryStrings) != 0 {
		metricMetricStatement := map[string]any{
			"context":    "metric",
			"statements": metricQueryStrings,
		}
		metricStatements = append(metricStatements, metricMetricStatement)
	}
	if len(datapointQueryStrings) != 0 {
		datapointMetricStatement := map[string]any{
			"context":    "datapoint",
			"statements": datapointQueryStrings,
		}
		metricStatements = append(metricStatements, datapointMetricStatement)
	}
	return Component{
		Type: "transform",
		Config: map[string]any{
			"metric_statements": metricStatements,
		},
	}
}

// TransformQueryContext is a type wrapper for the context of a query expression within the transoform processor
type TransformQueryContext string

const (
	Metric    TransformQueryContext = "metric"
	Datapoint TransformQueryContext = "datapoint"
)

// TransformQuery is a type wrapper for query expressions supported by the transform
// processor found here: https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/transformprocessor
type TransformQuery struct {
	Context   TransformQueryContext
	Statement string
}

// FlattenResourceAttribute returns an expression that brings down a resource attribute to a
// metric attribute.
func FlattenResourceAttribute(resourceAttribute, metricAttribute string) TransformQuery {
	return TransformQuery{
		Context:   Datapoint,
		Statement: fmt.Sprintf(`set(attributes["%s"], resource.attributes["%s"])`, metricAttribute, resourceAttribute),
	}
}

// GroupByAttribute returns an expression that makes a metric attribute into a resource attribute.
func GroupByAttribute(attribute string) TransformQuery {
	return TransformQuery{
		Context:   Datapoint,
		Statement: fmt.Sprintf(`set(resource.attributes["%s"], attributes["%s"])`, attribute, attribute),
	}
}

// DeleteMetricAttribute returns an expression that removes the metric attribute specified.
func DeleteMetricAttribute(metricAttribute string) TransformQuery {
	return TransformQuery{
		Context:   Datapoint,
		Statement: fmt.Sprintf(`delete_key(attributes, "%s")`, metricAttribute),
	}
}

// ConvertGaugeToSum returns an expression where a gauge metric can be converted into a sum
func ConvertGaugeToSum(metricName string) TransformQuery {
	return TransformQuery{
		Context:   Metric,
		Statement: fmt.Sprintf(`convert_gauge_to_sum("cumulative", true) where name == "%s"`, metricName),
	}
}

// ConvertFloatToInt returns an expression where a float-valued metric can be converted to an int
func ConvertFloatToInt(metricName string) TransformQuery {
	return TransformQuery{
		Context:   Datapoint,
		Statement: fmt.Sprintf(`set(value_int, Int(value_double)) where metric.name == "%s"`, metricName),
	}
}

// SetDescription returns a metrics transform expression where the metrics description will be set to what is provided
func SetDescription(metricName, metricDescription string) TransformQuery {
	return TransformQuery{
		Context:   Datapoint,
		Statement: fmt.Sprintf(`set(metric.description, "%s") where metric.name == "%s"`, metricDescription, metricName),
	}
}

// SetUnit returns a metrics transform expression where the metric unit is set to provided value
func SetUnit(metricName, unit string) TransformQuery {
	return TransformQuery{
		Context:   Datapoint,
		Statement: fmt.Sprintf(`set(metric.unit, "%s") where metric.name == "%s"`, unit, metricName),
	}
}

// SetName returns a metrics transform expression where the metric name is set to provided value
func SetName(oldName, newName string) TransformQuery {
	return TransformQuery{
		Context:   Datapoint,
		Statement: fmt.Sprintf(`set(metric.name, "%s") where metric.name == "%s"`, newName, oldName),
	}
}

func SetAttribute(metricName, attributeKey, attributeValue string) TransformQuery {
	return TransformQuery{
		Context:   Datapoint,
		Statement: fmt.Sprintf(`set(attributes["%s"], "%s") where metric.name == "%s"`, attributeKey, attributeValue, metricName),
	}
}

// SummarySumValToSum creates a new Sum metric out of a summary metric's sum value. The new metric has a name of "<Old Name>_sum".
func SummarySumValToSum(metricName, aggregation string, isMonotonic bool) TransformQuery {
	return TransformQuery{
		Context:   Datapoint,
		Statement: fmt.Sprintf(`convert_summary_sum_val_to_sum("%s",  %t) where metric.name == "%s"`, aggregation, isMonotonic, metricName),
	}
}

// SummaryCountValToSum creates a new Sum metric out of a summary metric's count value. The new metric has a name of "<Old Name>_count".
func SummaryCountValToSum(metricName, aggregation string, isMonotonic bool) TransformQuery {
	return TransformQuery{
		Context:   Datapoint,
		Statement: fmt.Sprintf(`convert_summary_count_val_to_sum("%s",  %t) where metric.name == "%s"`, aggregation, isMonotonic, metricName),
	}
}

// RetainResource retains the resource attributes provided, and drops all other attributes.
func RetainResource(resourceAttributeKeys ...string) TransformQuery {
	return TransformQuery{
		Context:   Datapoint,
		Statement: fmt.Sprintf(`keep_keys(resource.attributes, [%s])`, strings.Join(resourceAttributeKeys[:], ",")),
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

// UpdateMetric returns a config snippet applies transformations to the given metric name
func UpdateMetric(metric string, operations ...map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{
		"include": metric,
		"action":  "update",
	}
	if len(operations) > 0 {
		out["operations"] = operations
	}
	return out
}

// UpdateMetricRegexp returns a config snippet that applies transformations to metrics matching
// the input regex
func UpdateMetricRegexp(metricRegex string, operations ...map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{
		"include":    metricRegex,
		"match_type": "regexp",
		"action":     "update",
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

// CondenseResourceMetrics merges multiple resource metrics on
// a slice of metrics to a single resource metrics payload, if they have the same
// resource.
func CondenseResourceMetrics() Component {
	return Component{
		Type:   "groupbyattrs",
		Config: map[string]any{},
	}
}

// ModifyInstrumentationScope sets the instrumentation scope name and version
// fields which will later be exported to Cloud Monitoring metric labels.
// The name will always be prefixed with "agent.googleapis.com/".
func ModifyInstrumentationScope(name string, version string) Component {
	return Component{
		Type: "modifyscope",
		Config: map[string]interface{}{
			"override_scope_name":    "agent.googleapis.com/" + name,
			"override_scope_version": version,
		},
	}
}

// GroupByGMPAttrs moves the "namespace", "cluster", and "location"
// metric attributes to resource attributes. The
// googlemanagedprometheus exporter will use these resource attributes
// to populate metric labels.
func GroupByGMPAttrs() Component {
	return Component{
		Type: "groupbyattrs",
		Config: map[string]interface{}{
			"keys": []string{"namespace", "cluster", "location"},
		},
	}
}

// GroupByGMPAttrs_OTTL moves the "namespace", "cluster", and "location"
// metric attributes to resource attributes similar to GroupByGMPAttrs.
// The difference here is it uses OTTL instead of the groupbyattrs processor
// since that processor discards certain metadata from the metrics that are
// important to preserve prometheus untyped metrics.
//
// See more detail in https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/33419.
func GroupByGMPAttrs_OTTL() Component {
	return TransformationMetrics(
		GroupByAttribute("location"),
		GroupByAttribute("cluster"),
		GroupByAttribute("namespace"),
		DeleteMetricAttribute("location"),
		DeleteMetricAttribute("cluster"),
		DeleteMetricAttribute("namespace"),
	)
}

// GCPResourceDetector returns a resourcedetection processor configured for only GCP.
func GCPResourceDetector(override bool) Component {
	config := map[string]interface{}{
		"detectors": []string{"gcp"},
	}
	if override != true {
		// override defaults to true; omit it in that case to prevent config diffs.
		config["override"] = override
	}
	return Component{
		Type:   "resourcedetection",
		Config: config,
	}
}

// ResourceTransform returns a Component that applies changes on resource attributes.
func ResourceTransform(attributes map[string]string, override bool) Component {
	a := []map[string]interface{}{}
	keys := make([]string, 0, len(attributes))
	for k := range attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	action := "insert"
	if override {
		action = "upsert"
	}
	for _, v := range keys {
		a = append(a, map[string]interface{}{
			"key":    v,
			"value":  attributes[v],
			"action": action,
		})
	}
	config := map[string]interface{}{
		"attributes": a,
	}
	return Component{
		Type:   "resource",
		Config: config,
	}
}
