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

import "github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"

// MetricsReceiverAgent provides the agent.googleapis.com/agent/ metrics.
// It is never referenced in the config file, and instead is forcibly added in confgenerator.go.
// Therefore, it does not need to implement any interfaces.
type MetricsReceiverAgent struct {
	Version string
}

func (r MetricsReceiverAgent) Pipeline() otel.Pipeline {
	return otel.Pipeline{
		Receiver: otel.Component{
			Type: "prometheus",
			Config: map[string]interface{}{
				"config": map[string]interface{}{
					"scrape_configs": []map[string]interface{}{{
						"job_name":        "otel-collector",
						"scrape_interval": "1m",
						"static_configs": []map[string]interface{}{{
							// TODO(b/196990135): Customization for the port number
							"targets": []string{"0.0.0.0:8888"},
						}},
					}},
				},
			},
		},
		Processors: []otel.Component{
			otel.MetricsFilter(
				"include",
				"strict",
				"otelcol_process_uptime",
				"otelcol_process_memory_rss",
				"otelcol_grpc_io_client_completed_rpcs",
				"otelcol_googlecloudmonitoring_point_count",
			),
			otel.MetricsTransform(
				otel.RenameMetric("otelcol_process_uptime", "agent/uptime",
					// change data type from double -> int64
					otel.ToggleScalarDataType,
					otel.AddLabel("version", r.Version),
				),
				otel.RenameMetric("otelcol_process_memory_rss", "agent/memory_usage"),
				otel.RenameMetric("otelcol_grpc_io_client_completed_rpcs", "agent/api_request_count",
					// change data type from double -> int64
					otel.ToggleScalarDataType,
					// TODO: below is proposed new configuration for the metrics transform processor
					// ignore any non "google.monitoring" RPCs (note there won't be any other RPCs for now)
					// - action: select_label_values
					//   label: grpc_client_method
					//   value_regexp: ^google\.monitoring
					otel.RenameLabel("grpc_client_status", "state"),
					// delete grpc_client_method dimension, retaining only state
					otel.AggregateLabels("sum", "state"),
				),
				otel.RenameMetric("otelcol_googlecloudmonitoring_point_count", "agent/monitoring/point_count",
					// change data type from double -> int64
					otel.ToggleScalarDataType,
				),
				otel.AddPrefix("agent.googleapis.com"),
			),
		},
	}
}

// intentionally not registered as a component because this is not created by users
