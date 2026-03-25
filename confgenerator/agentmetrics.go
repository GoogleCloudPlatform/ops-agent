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
	"context"
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

var grpcCamelToSnake = map[string]string{
	"OK":                 "OK",
	"Canceled":           "CANCELLED",
	"Unknown":            "UNKNOWN",
	"InvalidArgument":    "INVALID_ARGUMENT",
	"DeadlineExceeded":   "DEADLINE_EXCEEDED",
	"NotFound":           "NOT_FOUND",
	"AlreadyExists":      "ALREADY_EXISTS",
	"PermissionDenied":   "PERMISSION_DENIED",
	"ResourceExhausted":  "RESOURCE_EXHAUSTED",
	"FailedPrecondition": "FAILED_PRECONDITION",
	"Aborted":            "ABORTED",
	"OutOfRange":         "OUT_OF_RANGE",
	"Unimplemented":      "UNIMPLEMENTED",
	"Internal":           "INTERNAL",
	"Unavailable":        "UNAVAILABLE",
	"DataLoss":           "DATA_LOSS",
	"Unauthenticated":    "UNAUTHENTICATED",
}

func (r AgentSelfMetrics) AddSelfMetricsPipelines(receiverPipelines map[string]otel.ReceiverPipeline, pipelines map[string]otel.Pipeline, ctx context.Context) {
	// Receiver pipelines names should have 1 underscore to avoid collision with user configurations.
	receiverPipelines["agent_prometheus"] = r.PrometheusMetricsPipeline(ctx)

	// Pipeline names should have no underscores to avoid collision with user configurations.
	pipelines["otel"] = otel.Pipeline{
		Type:                 "metrics",
		ReceiverPipelineName: "agent_prometheus",
		Processors:           r.OtelPipelineProcessors(ctx),
	}

	pipelines["fluentbit"] = otel.Pipeline{
		Type:                 "metrics",
		ReceiverPipelineName: "agent_prometheus",
		Processors:           r.FluentBitPipelineProcessors(),
	}

	pipelines["loggingmetrics"] = otel.Pipeline{
		Type:                 "metrics",
		ReceiverPipelineName: "agent_prometheus",
		Processors:           r.LoggingMetricsPipelineProcessors(ctx),
	}

	receiverPipelines["ops_agent"] = r.OpsAgentPipeline(ctx)
	pipelines["opsagent"] = otel.Pipeline{
		Type:                 "metrics",
		ReceiverPipelineName: "ops_agent",
	}
}

func (r AgentSelfMetrics) PrometheusMetricsPipeline(ctx context.Context) otel.ReceiverPipeline {
	return ConvertGCMSystemExporterToOtlpExporter(otel.ReceiverPipeline{
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
					otel.DeleteMetricResourceAttribute("service_name"),
					otel.DeleteMetricResourceAttribute("service_version"),
					otel.DeleteMetricResourceAttribute("service_instance_id"),
					otel.DeleteMetricResourceAttribute("server_port"),
					otel.DeleteMetricResourceAttribute("url_scheme"),
				),
			},
		},
	}, ctx)
}

func (r AgentSelfMetrics) OtelPipelineProcessors(ctx context.Context) []otel.Component {
	durationMetric := "grpc_client_attempt_duration"
	durationCountMetric := "grpc_client_attempt_duration_count"
	filteredMetrics := []string{
		durationCountMetric,
		"googlecloudmonitoring/point_count",
	}
	pointCountMetric := otel.RenameMetric("googlecloudmonitoring/point_count", "agent/monitoring/point_count",
		// change data type from double -> int64
		otel.ToggleScalarDataType,
		// Remove service.version label
		otel.AggregateLabels("sum", "status"),
	)
	apiRequestCount := otel.RenameMetric(durationCountMetric, "agent/api_request_count",
		otel.RenameLabel("grpc_status", "state"),
		// delete grpc_client_method dimension & service.version label, retaining only state
		otel.AggregateLabels("sum", "state"),
	)
	metricFilter := otel.MetricsOTTLFilter([]string{}, []string{
		// Filter out histogram datapoints where the grpc_target is not related to monitoring.
		`metric.name == "` + durationCountMetric + `" and (not IsMatch(datapoint.attributes["grpc_target"], "monitoring.googleapis"))`,
	})

	expOtlpExporter := experimentsFromContext(ctx)["otlp_exporter"]
	var extraTransforms []map[string]interface{}
	if expOtlpExporter {
		durationMetric = "rpc_client_call_duration"
		durationCountMetric = "rpc_client_call_duration_count"
		filteredMetrics = []string{
			"otelcol_exporter_sent_metric_points",
			"otelcol_exporter_send_failed_metric_points",
			durationCountMetric,
		}
		extraTransforms = []map[string]interface{}{
			otel.UpdateMetric("otelcol_exporter_sent_metric_points",
				otel.ToggleScalarDataType,
				otel.AddLabel("status", "OK"),
			),
			otel.UpdateMetric("otelcol_exporter_send_failed_metric_points",
				otel.ToggleScalarDataType,
				otel.AddLabel("status", "UNKNOWN"),
			),
		}
		// b/468059325: Factor in partial success after upstream bug is fixed.
		pointCountMetric = otel.CombineMetrics("otelcol_exporter_sent_metric_points|otelcol_exporter_send_failed_metric_points", "agent/monitoring/point_count",
			otel.AggregateLabels("sum", "status"))
		apiRequestCount = otel.RenameMetric(durationCountMetric, "agent/api_request_count",
			otel.RenameLabelValues("rpc_response_status_code", grpcCamelToSnake),
			otel.RenameLabel("rpc_response_status_code", "state"),
			// delete all other labels, retaining only state
			otel.AggregateLabels("sum", "state"))

		metricFilter = otel.MetricsOTTLFilter([]string{}, []string{
			// Filter out histogram datapoints where the rpc.service is not related to monitoring.
			`metric.name == "` + durationCountMetric + `" and (not IsMatch(datapoint.attributes["rpc_method"], "opentelemetry.proto.collector.metrics.v1.MetricsService/Export"))`,
		})
	}

	transforms := []map[string]interface{}{
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
		apiRequestCount,
	}

	transforms = append(transforms, extraTransforms...)
	transforms = append(transforms, pointCountMetric)
	transforms = append(transforms, otel.AddPrefix("agent.googleapis.com"))

	return []otel.Component{
		otel.Transform("metric", "metric",
			ottl.ExtractCountMetric(true, durationMetric),
		),
		metricFilter,
		otel.MetricsFilter(
			"include",
			"strict",
			append([]string{
				"otelcol_process_uptime",
				"otelcol_process_memory_rss",
			}, filteredMetrics...,
			)...,
		),
		otel.MetricsTransform(transforms...),
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

func (r AgentSelfMetrics) LoggingMetricsPipelineProcessors(ctx context.Context) []otel.Component {
	durationMetric := "grpc_client_attempt_duration"
	durationCountMetric := "grpc_client_attempt_duration_count"

	metricFilter := otel.MetricsOTTLFilter([]string{}, []string{
		// Filter out histogram datapoints where the grpc_target is not related to logging.
		`metric.name == "` + durationCountMetric + `" and (not IsMatch(datapoint.attributes["grpc_target"], "logging.googleapis"))`,
	})

	otelRequestCount := otel.RenameMetric(durationCountMetric, "otel_request_count",
		otel.RenameLabel("grpc_status", "response_code"),
		otel.RenameLabelValues("response_code", grpcToHTTPStatus),
		// delete grpc_client_method dimension & service.version label, retaining only response_code
		otel.AggregateLabels("sum", "response_code"),
	)

	expOtlpExporter := experimentsFromContext(ctx)["otlp_exporter"]
	if expOtlpExporter {
		durationMetric = "rpc_client_call_duration"
		durationCountMetric = "rpc_client_call_duration_count"

		metricFilter = otel.MetricsOTTLFilter([]string{}, []string{
			// Filter out histogram datapoints where the rpc.method is not related to logging.
			`metric.name == "` + durationCountMetric + `" and (not IsMatch(datapoint.attributes["rpc_method"], "opentelemetry.proto.collector.logs.v1.LogsService/Export"))`,
		})
		otelRequestCount = otel.RenameMetric(durationCountMetric, "otel_request_count",
			otel.RenameLabelValues("rpc_response_status_code", grpcCamelToSnake),
			otel.RenameLabel("rpc_response_status_code", "response_code"),
			otel.RenameLabelValues("response_code", grpcToHTTPStatus),
			otel.AggregateLabels("sum", "response_code"),
		)
	}

	return []otel.Component{
		otel.Transform("metric", "metric",
			ottl.ExtractCountMetric(true, durationMetric),
		),
		metricFilter,
		otel.MetricsFilter(
			"include",
			"strict",
			"fluentbit_stackdriver_requests_total",
			"fluentbit_stackdriver_proc_records_total",
			"fluentbit_stackdriver_retried_records_total",
			"otelcol_exporter_sent_log_records",
			"otelcol_exporter_send_failed_log_records",
			durationCountMetric,
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
			otelRequestCount,
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
		// Aggregating as delta metrics isolates data for only the most recent metric cumulative update.
		// Set `initial_value: drop" to always store the "first point" as "anchor" to set the "start_time" and
		// calculate next point values as difference with the "first point" value.
		otel.CumulativeToDeltaWithInitialValue("drop",
			"otel_log_entry_count", "otel_log_entry_retry_count", "otel_request_count",
			"fluentbit_log_entry_count", "fluentbit_log_entry_retry_count", "fluentbit_request_count",
		),
		otel.TransformationMetrics(
			// Set "start_time_unix_nano = 0" and "time = Now()" so "deltatocumulative" can sum all points
			// without "out of order" or "older start" errors.
			// TODO: b/445233472 - Update "deltatocumulative" processor with a new "strategy" for point aggreagation.
			otel.TransformQuery{
				Context:   otel.Datapoint,
				Statement: `set(time, Now())`,
			},
			otel.TransformQuery{
				Context:   otel.Datapoint,
				Statement: `set(start_time_unix_nano, 0)`,
			},
			// Set unit = "1" to metrics who may not have it.
			otel.TransformQuery{
				Context:   otel.Metric,
				Statement: `set(unit, "1")`,
			},
			// Rename metrics for aggregation by "deltatocumulative".
			otel.SetName("fluentbit_log_entry_count", "agent/log_entry_count"),
			otel.SetName("fluentbit_log_entry_retry_count", "agent/log_entry_retry_count"),
			otel.SetName("fluentbit_request_count", "agent/request_count"),
			otel.SetName("otel_log_entry_count", "agent/log_entry_count"),
			otel.SetName("otel_log_entry_retry_count", "agent/log_entry_retry_count"),
			otel.SetName("otel_request_count", "agent/request_count"),
		),
		// DeltaToCumulative keeps in memory information of previous delta points
		// to generate a valid cumulative monotonic metric.
		otel.DeltaToCumulative(),
		// The processor "interval" outputs the last point in each 1 minute interval.
		otel.Interval("1m"),
		otel.MetricsTransform(otel.AddPrefix("agent.googleapis.com")),
	}
}

func (r AgentSelfMetrics) OpsAgentPipeline(ctx context.Context) otel.ReceiverPipeline {
	receiverConfig := map[string]any{
		"include": []string{
			filepath.Join(r.OtelRuntimeDir, "enabled_receivers_otlp.json"),
			filepath.Join(r.OtelRuntimeDir, "feature_tracking_otlp.json")},
		"replay_file":   true,
		"poll_interval": time.Duration(60 * time.Second).String(),
	}
	return ConvertGCMSystemExporterToOtlpExporter(otel.ReceiverPipeline{
		Receiver: otel.Component{
			Type:   "otlpjsonfile",
			Config: receiverConfig,
		},
		ExporterTypes: map[string]otel.ExporterType{
			"metrics": otel.System,
		},
		Processors: map[string][]otel.Component{
			"metrics": {
				otel.Transform("metric", "datapoint", []ottl.Statement{"set(time, Now())"}),
			},
		},
	}, ctx)
}

// intentionally not registered as a component because this is not created by users
