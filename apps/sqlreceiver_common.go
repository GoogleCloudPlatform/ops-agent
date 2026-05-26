// Copyright 2022 Google LLC
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

type sqlReceiverMetric struct {
	metric_name       string
	value_column      string
	unit              string
	description       string
	data_type         string
	monotonic         bool
	value_type        string
	attribute_columns []string
	static_attributes map[string]string
}

type sqlReceiverQuery struct {
	query   string
	metrics []sqlReceiverMetric
}

func sqlReceiverQueriesConfig(queries []sqlReceiverQuery) []map[string]interface{} {
	cfg := []map[string]interface{}{}
	for _, q := range queries {
		metrics := []map[string]interface{}{}
		for _, m := range q.metrics {
			metric := map[string]interface{}{
				"metric_name":       m.metric_name,
				"value_column":      m.value_column,
				"unit":              m.unit,
				"description":       m.description,
				"data_type":         m.data_type,
				"value_type":        m.value_type,
				"attribute_columns": m.attribute_columns,
				"static_attributes": m.static_attributes,
			}
			if m.data_type == "sum" {
				metric["monotonic"] = m.monotonic
			}

			metrics = append(metrics, metric)
		}

		query := map[string]interface{}{
			"sql":     q.query,
			"metrics": metrics,
		}

		cfg = append(cfg, query)
	}

	return cfg
}
