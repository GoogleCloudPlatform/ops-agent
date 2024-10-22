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

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel/ottl"
)

// AgentSelfMetrics provides the agent.googleapis.com/agent/ metrics.
// It is never referenced in the config file, and instead is forcibly added in confgenerator.go.
// Therefore, it does not need to implement any interfaces.
type AgentSelfMetrics struct {
	Version string
	Port    int
}

func (r AgentSelfMetrics) MetricsSubmodulePipeline() otel.ReceiverPipeline {
	return otel.ReceiverPipeline{
		Receiver: otel.Component{
			Type: "prometheus",
			Config: map[string]interface{}{
				"config": map[string]interface{}{
					"scrape_configs": []map[string]interface{}{{
						"job_name":        "otel-collector",
						"scrape_interval": "1m",
						"static_configs": []map[string]interface{}{{
							// TODO(b/196990135): Customization for the port number
							"targets": []string{fmt.Sprintf("0.0.0.0:%d", r.Port)},
						}},
					}},
				},
			},
		},
		ExporterTypes: map[string]otel.ExporterType{
			"metrics": otel.System,
		},
		Processors: map[string][]otel.Component{"metrics": {
			otel.MetricsFilter(
				"include",
				"strict",
				"otelcol_process_uptime",
				"otelcol_process_memory_rss",
				"grpc_client_attempt_duration",
				"googlecloudmonitoring_point_count",
			),
			otel.Transform("metric", "metric",
				// create new count metric from histogram metric
				ottl.ExtractCountMetric(true, "grpc_client_attempt_duration"),
			),
			otel.MetricsFilter(
				"include",
				"strict",
				"otelcol_process_uptime",
				"otelcol_process_memory_rss",
				"grpc_client_attempt_duration_count",
				"googlecloudmonitoring_point_count",
			),
			otel.MetricsTransform(
				otel.RenameMetric("otelcol_process_uptime", "agent/uptime",
					// change data type from double -> int64
					otel.ToggleScalarDataType,
					otel.AddLabel("version", r.Version),
					// remove service.version label
					otel.AggregateLabels("sum", "version"),
				),
				otel.RenameMetric("otelcol_process_memory_rss", "agent/memory_usage",
					// remove service.version label
					otel.AggregateLabels("sum"),
				),
				otel.RenameMetric("grpc_client_attempt_duration_count", "agent/api_request_count",
					// TODO: below is proposed new configuration for the metrics transform processor
					// ignore any non "google.monitoring" RPCs (note there won't be any other RPCs for now)
					// - action: select_label_values
					//   label: grpc_client_method
					//   value_regexp: ^google\.monitoring
					otel.RenameLabel("grpc_status", "state"),
					// delete grpc_client_method dimension & service.version label, retaining only state
					otel.AggregateLabels("sum", "state"),
				),
				otel.RenameMetric("googlecloudmonitoring_point_count", "agent/monitoring/point_count",
					// change data type from double -> int64
					otel.ToggleScalarDataType,
					// Remove service.version label
					otel.AggregateLabels("sum", "status"),
				),
				otel.AddPrefix("agent.googleapis.com"),
			),
		}},
	}
}

func (r AgentSelfMetrics) LoggingSubmodulePipeline() otel.ReceiverPipeline {
	return otel.ReceiverPipeline{
		Receiver: otel.Component{
			Type: "prometheus",
			Config: map[string]interface{}{
				"config": map[string]interface{}{
					"scrape_configs": []map[string]interface{}{{
						"job_name":        "logging-collector",
						"scrape_interval": "1m",
						"metrics_path":    "/metrics",
						"static_configs": []map[string]interface{}{{
							// TODO(b/196990135): Customization for the port number
							"targets": []string{fmt.Sprintf("0.0.0.0:%d", r.Port)},
						}},
					}},
				},
			},
		},
		ExporterTypes: map[string]otel.ExporterType{
			"metrics": otel.System,
		},
		Processors: map[string][]otel.Component{"metrics": {
			otel.MetricsFilter(
				"include",
				"strict",
				"fluentbit_uptime",
				"fluentbit_stackdriver_requests_total",
				"fluentbit_stackdriver_proc_records_total",
				"fluentbit_stackdriver_retried_records_total",
			),
			otel.MetricsTransform(
				otel.RenameMetric("fluentbit_uptime", "agent/uptime",
					// change data type from double -> int64
					otel.ToggleScalarDataType,
					otel.AddLabel("version", r.Version),
					// remove service.version label
					otel.AggregateLabels("sum", "version"),
				),
				otel.RenameMetric("fluentbit_stackdriver_requests_total", "agent/request_count",
					// change data type from double -> int64
					otel.ToggleScalarDataType,
					otel.RenameLabel("status", "response_code"),
					otel.AggregateLabels("sum", "response_code"),
				),
				otel.RenameMetric("fluentbit_stackdriver_proc_records_total", "agent/log_entry_count",
					// change data type from double -> int64
					otel.ToggleScalarDataType,
					otel.RenameLabel("status", "response_code"),
					otel.AggregateLabels("sum", "response_code"),
				),
				otel.RenameMetric("fluentbit_stackdriver_retried_records_total", "agent/log_entry_retry_count",
					// change data type from double -> int64
					otel.ToggleScalarDataType,
					otel.RenameLabel("status", "response_code"),
					otel.AggregateLabels("sum", "response_code"),
				),
				otel.AddPrefix("agent.googleapis.com"),
			),
		}},
	}
}

// intentionally not registered as a component because this is not created by users
