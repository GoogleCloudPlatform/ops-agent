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
	"path/filepath"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel/ottl"
)

// AgentSelfMetrics provides the agent.googleapis.com/agent/ metrics.
// It is never referenced in the config file, and instead is forcibly added in confgenerator.go.
// Therefore, it does not need to implement any interfaces.
type AgentSelfMetrics struct {
	MetricsVersionLabel string
	LoggingVersionLabel string
	FluentBitPort       int
	OtelPort            int
	OtelRuntimeDir      string
	OtelLogging         bool
}

// Following reference : https://github.com/googleapis/googleapis/blob/master/google/rpc/code.proto
var grpcToHTTPStatus = map[string]string{
	"OK":                  "200",
	"INVALID_ARGUMENT":    "400",
	"FAILED_PRECONDITION": "400",
	"OUT_OF_RANGE":        "400",
	"UNAUTHENTICATED":     "401",
	"PERMISSION_DENIED":   "403",
	"NOT_FOUND":           "404",
	"ALREADY_EXISTS":      "409",
	"ABORTED":             "409",
	"RESOURCE_EXHAUSTED":  "429",
	"CANCELLED":           "499",
	"UNKNOWN":             "500",
	"INTERNAL":            "500",
	"DATA_LOSS":           "500",
	"UNIMPLEMENTED":       "501",
	"UNAVAILABLE":         "503",
	"DEADLINE_EXCEEDED":   "504",
}

func (r AgentSelfMetrics) AddSelfMetricsPipelines(receiverPipelines map[string]otel.ReceiverPipeline, pipelines map[string]otel.Pipeline) {
	receiverPipelines["agent_prometheus_metrics"] = r.PrometheusMetricsPipeline()

	pipelines["otel"] = otel.Pipeline{
		Type:                 "metrics",
		ReceiverPipelineName: "agent_prometheus_metrics",
		Processors:           r.OtelPipelineProcessors(),
	}

	pipelines["fluentbit"] = otel.Pipeline{
		Type:                 "metrics",
		ReceiverPipelineName: "agent_prometheus_metrics",
		Processors:           r.FluentBitPipelineProcessors(),
	}

	pipelines["logging_metrics"] = otel.Pipeline{
		Type:                 "metrics",
		ReceiverPipelineName: "agent_prometheus_metrics",
		Processors:           r.LoggingMetricsPipelineProcessors(),
	}

	receiverPipelines["ops_agent"] = r.OpsAgentPipeline()
	pipelines["ops_agent"] = otel.Pipeline{
		Type:                 "metrics",
		ReceiverPipelineName: "ops_agent",
	}
}

func (r AgentSelfMetrics) PrometheusMetricsPipeline() otel.ReceiverPipeline {
	return otel.ReceiverPipeline{
		Receiver: otel.Component{
			Type: "prometheus",
			Config: map[string]interface{}{
				"config": map[string]interface{}{
					"scrape_configs": []map[string]interface{}{
						{
							"job_name":        "logging-collector",
							"scrape_interval": "1m",
							"metrics_path":    "/metrics",
							"static_configs": []map[string]interface{}{{
								// TODO(b/196990135): Customization for the port number
								"targets": []string{fmt.Sprintf("0.0.0.0:%d", r.FluentBitPort)},
							}},
						},
						{
							"job_name":        "otel-collector",
							"scrape_interval": "1m",
							"static_configs": []map[string]interface{}{{
								// TODO(b/196990135): Customization for the port number
								"targets": []string{fmt.Sprintf("0.0.0.0:%d", r.OtelPort)},
							}},
						},
					},
				},
			},
		},
		ExporterTypes: map[string]otel.ExporterType{
			"metrics": otel.System,
		},
		Processors: map[string][]otel.Component{
			"metrics": {
				otel.TransformationMetrics(
					otel.DeleteMetricResourceAttribute("service.name"),
					otel.DeleteMetricResourceAttribute("service.version"),
					otel.DeleteMetricResourceAttribute("service.instance.id"),
					otel.DeleteMetricResourceAttribute("server.port"),
					otel.DeleteMetricResourceAttribute("url.scheme"),
				),
			},
		},
	}
}

func (r AgentSelfMetrics) OtelPipelineProcessors() []otel.Component {
	return []otel.Component{
		otel.Transform("metric", "metric",
			[]ottl.Statement{
				ottl.ExtractCountMetric(true, "grpc.client.attempt.duration"),
			},
		),
		otel.MetricsOTTLFilter([]string{}, []string{
			// Filter out histogram datapoints where the grpc.target is not related to monitoring.
			`metric.name == "grpc.client.attempt.duration_count" and (not IsMatch(datapoint.attributes["grpc.target"], "monitoring.googleapis"))`,
		}),
		otel.MetricsFilter(
			"include",
			"strict",
			"otelcol_process_uptime",
			"otelcol_process_memory_rss",
			"grpc.client.attempt.duration_count",
			"googlecloudmonitoring/point_count",
		),
		otel.MetricsTransform(
			otel.RenameMetric("otelcol_process_uptime", "agent/uptime",
				// change data type from double -> int64
				otel.ToggleScalarDataType,
				otel.AddLabel("version", r.MetricsVersionLabel),
				// remove service.version label
				otel.AggregateLabels("sum", "version"),
			),
			otel.RenameMetric("otelcol_process_memory_rss", "agent/memory_usage",
				// remove service.version label
				otel.AggregateLabels("sum"),
			),
			otel.RenameMetric("grpc.client.attempt.duration_count", "agent/api_request_count",
				otel.RenameLabel("grpc.status", "state"),
				// delete grpc_client_method dimension & service.version label, retaining only state
				otel.AggregateLabels("sum", "state"),
			),
			otel.RenameMetric("googlecloudmonitoring/point_count", "agent/monitoring/point_count",
				// change data type from double -> int64
				otel.ToggleScalarDataType,
				// Remove service.version label
				otel.AggregateLabels("sum", "status"),
			),
			otel.AddPrefix("agent.googleapis.com"),
		),
	}
}

func (r AgentSelfMetrics) FluentBitPipelineProcessors() []otel.Component {
	return []otel.Component{
		otel.MetricsFilter(
			"include",
			"strict",
			"fluentbit_uptime",
		),
		otel.MetricsTransform(
			otel.RenameMetric("fluentbit_uptime", "agent/uptime",
				// change data type from double -> int64
				otel.ToggleScalarDataType,
				otel.AddLabel("version", r.LoggingVersionLabel),
				// remove service.version label
				otel.AggregateLabels("sum", "version"),
			),
			otel.AddPrefix("agent.googleapis.com"),
		),
	}
}

func (r AgentSelfMetrics) LoggingMetricsPipelineProcessors() []otel.Component {
	return []otel.Component{
		otel.Transform("metric", "metric",
			[]ottl.Statement{
				ottl.ExtractCountMetric(true, "grpc.client.attempt.duration"),
			},
		),
		otel.MetricsOTTLFilter([]string{}, []string{
			// Filter out histogram datapoints where the grpc.target is not related to logging.
			`metric.name == "grpc.client.attempt.duration_count" and (not IsMatch(datapoint.attributes["grpc.target"], "logging.googleapis"))`,
		}),
		otel.MetricsFilter(
			"include",
			"strict",
			"fluentbit_stackdriver_requests_total",
			"fluentbit_stackdriver_proc_records_total",
			"fluentbit_stackdriver_retried_records_total",
			"otelcol_exporter_sent_log_records",
			"otelcol_exporter_send_failed_log_records",
			"grpc.client.attempt.duration_count",
		),
		// Format fluentbit and otel logging metrics before aggregation.
		otel.MetricsTransform(
			otel.RenameMetric("fluentbit_stackdriver_retried_records_total", "fluentbit_log_entry_retry_count",
				// change data type from double -> int64
				otel.ToggleScalarDataType,
				otel.RenameLabel("status", "response_code"),
				otel.AggregateLabels("sum", "response_code"),
			),
			otel.DuplicateMetric("otelcol_exporter_send_failed_log_records", "otel_log_entry_retry_count",
				// change data type from double -> int64
				otel.ToggleScalarDataType,
				otel.AddLabel("response_code", "400"),
				otel.AggregateLabels("sum", "response_code"),
			),
			otel.RenameMetric("fluentbit_stackdriver_requests_total", "fluentbit_request_count",
				// change data type from double -> int64
				otel.ToggleScalarDataType,
				otel.RenameLabel("status", "response_code"),
				otel.AggregateLabels("sum", "response_code"),
			),
			otel.RenameMetric("grpc.client.attempt.duration_count", "otel_request_count",
				otel.RenameLabel("grpc.status", "response_code"),
				otel.RenameLabelValues("response_code", grpcToHTTPStatus),
				// delete grpc_client_method dimension & service.version label, retaining only response_code
				otel.AggregateLabels("sum", "response_code"),
			),
			otel.RenameMetric("fluentbit_stackdriver_proc_records_total", "fluentbit_log_entry_count",
				// change data type from double -> int64
				otel.ToggleScalarDataType,
				otel.RenameLabel("status", "response_code"),
				otel.AggregateLabels("sum", "response_code"),
			),
			otel.RenameMetric("otelcol_exporter_sent_log_records", "otel_log_entry_count",
				// change data type from double -> int64
				otel.ToggleScalarDataType,
				otel.AddLabel("response_code", "200"),
				otel.AggregateLabels("sum", "response_code"),
			),
			otel.RenameMetric("otelcol_exporter_send_failed_log_records", "otel_log_entry_count",
				// change data type from double -> int64
				otel.ToggleScalarDataType,
				otel.AddLabel("response_code", "400"),
				otel.AggregateLabels("sum", "response_code"),
			),
			otel.CombineMetrics("^otel_log_entry_count$$", "otel_log_entry_count",
				otel.AggregateLabels("sum", "response_code")),
		),
		// The processors "interval" and "groupbyattrs" batch metrics in a 1 minute interval.
		otel.Interval("1m"),
		otel.CondenseResourceMetrics(),
		// Some metrics are missing "unit". Needed to combine metrics.
		otel.Transform("metric", "metric",
			[]ottl.Statement{
				ottl.SetMetricUnitAll("1"),
			},
		),
		// Aggregating as delta metrics isolates data for the current 1m interval only.
		// Keep the initial value so the resulting cumulative metric has all data.
		otel.CumulativeToDeltaWithInitialValue("auto",
			"otel_log_entry_count", "otel_log_entry_retry_count", "otel_request_count",
			"fluentbit_log_entry_count", "fluentbit_log_entry_retry_count", "fluentbit_request_count",
		),
		// This standarizes all datapoints for aggregation which groups by "start_time", "time" and "labels".
		// 1) Align all datapoints timestamps within the batch to Now().
		// 2) Make sure all delta datapoints consist of a 1 minute inteval from "start_time" to "time".
		// 3) Truncate all timestamps to align for aggregation.
		otel.Transform("metric", "datapoint",
			[]ottl.Statement{
				`set(time, TruncateTime(Now(), Duration("1m")))`,
				`set(start_time, TruncateTime(time - Duration("1m")))`,
			},
		),
		// Combine fluent-bit and otel logging metric and sum their values per label.
		otel.MetricsTransform(
			otel.CombineMetrics("^.*_log_entry_retry_count$$", "agent/log_entry_retry_count",
				otel.AggregateLabels("sum", "response_code")),
			otel.CombineMetrics("^.*_request_count$$", "agent/request_count",
				otel.AggregateLabels("sum", "response_code")),
			otel.CombineMetrics("^.*_log_entry_count$$", "agent/log_entry_count",
				otel.AggregateLabels("sum", "response_code")),
		),
		// DeltaToCumulative keeps in memory information of previous delta points
		// to generate a valid cumulative monotonic metric.
		otel.DeltaToCumulative(),
		otel.MetricsTransform(otel.AddPrefix("agent.googleapis.com")),
	}
}

func (r AgentSelfMetrics) OpsAgentPipeline() otel.ReceiverPipeline {
	receiver_config := map[string]any{
		"include": []string{
			filepath.Join(r.OtelRuntimeDir, "enabled_receivers_otlp.json"),
			filepath.Join(r.OtelRuntimeDir, "feature_tracking_otlp.json")},
		"replay_file":   true,
		"poll_interval": time.Duration(60 * time.Second).String(),
	}
	return otel.ReceiverPipeline{
		Receiver: otel.Component{
			Type:   "otlpjsonfile",
			Config: receiver_config,
		},
		ExporterTypes: map[string]otel.ExporterType{
			"metrics": otel.System,
		},
		Processors: map[string][]otel.Component{
			"metrics": {
				otel.Transform("metric", "datapoint", []ottl.Statement{"set(time, Now())"}),
			},
		},
	}
}

// intentionally not registered as a component because this is not created by users
