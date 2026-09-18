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
	OtelPort            int
	OtelRuntimeDir      string
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

var otelErrorTypeToStatus = map[string]string{
	"OK":                 "OK",
	"Canceled":           "CANCELLED",
	"Cancelled":          "CANCELLED",
	"canceled":           "CANCELLED",
	"cancelled":          "CANCELLED",
	"Unknown":            "UNKNOWN",
	"Shutdown":           "UNKNOWN",
	"shutdown":           "UNKNOWN",
	"InvalidArgument":    "INVALID_ARGUMENT",
	"DeadlineExceeded":   "DEADLINE_EXCEEDED",
	"Deadline_Exceeded":  "DEADLINE_EXCEEDED",
	"deadline_exceeded":  "DEADLINE_EXCEEDED",
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
	return otel.ReceiverPipeline{
		Receiver: otel.Component{
			Type: "prometheus",
			Config: map[string]interface{}{
				"config": map[string]interface{}{
					"scrape_configs": []map[string]interface{}{

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
		Processors: map[string][]otel.Component{
			"metrics": {
				otel.TransformationMetrics(
					otel.DeleteMetricResourceAttribute("service.name"),
					otel.DeleteMetricResourceAttribute("service.version"),
					otel.DeleteMetricResourceAttribute("service.instance.id"),
					otel.DeleteMetricResourceAttribute("server.port"),
					otel.DeleteMetricResourceAttribute("url.scheme"),
				),
				otel.MetricsRemoveInstrumentationLibraryLabelsAttributes(),
				otel.MetricsRemoveServiceAttributes(),
			},
		},
	}
}

func (r AgentSelfMetrics) OtelPipelineProcessors(ctx context.Context) []otel.Component {
	durationMetric := "rpc.client.call.duration"
	durationCountMetric := "rpc.client.call.duration_count"
	filteredMetrics := []string{
		"otelcol_exporter_sent_metric_points",
		"otelcol_exporter_send_failed_metric_points",
		durationCountMetric,
	}
	extraTransforms := []map[string]interface{}{
		otel.UpdateMetric("otelcol_exporter_sent_metric_points",
			otel.ToggleScalarDataType,
			otel.AddLabel("status", "OK"),
			otel.AggregateLabels("sum", "status"),
		),
		otel.UpdateMetric("otelcol_exporter_send_failed_metric_points",
			otel.ToggleScalarDataType,
			otel.RenameLabel("error.type", "status"),
			otel.RenameLabelValues("status", otelErrorTypeToStatus),
			otel.AggregateLabels("sum", "status"),
		),
	}
	// b/468059325: Factor in partial success after upstream bug is fixed.
	pointCountMetric := otel.CombineMetrics("otelcol_exporter_sent_metric_points|otelcol_exporter_send_failed_metric_points", "agent/monitoring/point_count",
		otel.AggregateLabels("sum", "status"))
	apiRequestCount := otel.RenameMetric(durationCountMetric, "agent/api_request_count",
		otel.RenameLabelValues("rpc.response.status_code", otelErrorTypeToStatus),
		otel.RenameLabel("rpc.response.status_code", "state"),
		// delete all other labels, retaining only state
		otel.AggregateLabels("sum", "state"))

	metricFilter := otel.MetricsOTTLFilter([]string{}, []string{
		// Filter out histogram datapoints where the rpc.service is not related to monitoring.
		`metric.name == "` + durationCountMetric + `" and (not IsMatch(datapoint.attributes["rpc.method"], "opentelemetry.proto.collector.metrics.v1.MetricsService/Export"))`,
	})

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

func (r AgentSelfMetrics) LoggingMetricsPipelineProcessors(ctx context.Context) []otel.Component {
	durationMetric := "rpc.client.call.duration"
	durationCountMetric := "rpc.client.call.duration_count"

	metricFilter := otel.MetricsOTTLFilter([]string{}, []string{
		// Filter out histogram datapoints where the rpc.method is not related to logging.
		`metric.name == "` + durationCountMetric + `" and (not IsMatch(datapoint.attributes["rpc.method"], "opentelemetry.proto.collector.logs.v1.LogsService/Export"))`,
	})

	agentRequestCount := otel.RenameMetric(durationCountMetric, "agent/request_count",
		otel.RenameLabelValues("rpc.response.status_code", otelErrorTypeToStatus),
		otel.RenameLabel("rpc.response.status_code", "response_code"),
		otel.RenameLabelValues("response_code", grpcToHTTPStatus),
		otel.AggregateLabels("sum", "response_code"),
	)

	return []otel.Component{
		otel.Transform("metric", "metric",
			ottl.ExtractCountMetric(true, durationMetric),
		),
		metricFilter,
		otel.MetricsFilter(
			"include",
			"strict",
			"otelcol_exporter_sent_log_records",
			"otelcol_exporter_send_failed_log_records",
			durationCountMetric,
		),
		// Format otel logging metrics directly to target agent.googleapis.com/agent/ metrics.
		otel.MetricsTransform(
			otel.DuplicateMetric("otelcol_exporter_send_failed_log_records", "agent/log_entry_retry_count",
				// change data type from double -> int64
				otel.ToggleScalarDataType,
				otel.AddLabel("response_code", "400"),
				otel.AggregateLabels("sum", "response_code"),
			),
			agentRequestCount,
			otel.RenameMetric("otelcol_exporter_sent_log_records", "agent/log_entry_count",
				// change data type from double -> int64
				otel.ToggleScalarDataType,
				otel.AddLabel("response_code", "200"),
				otel.AggregateLabels("sum", "response_code"),
			),
			otel.RenameMetric("otelcol_exporter_send_failed_log_records", "agent/log_entry_count",
				// change data type from double -> int64
				otel.ToggleScalarDataType,
				otel.AddLabel("response_code", "400"),
				otel.AggregateLabels("sum", "response_code"),
			),
			// Merge response_code dimensions under a single agent/log_entry_count metric object.
			otel.CombineMetrics(`^agent/log_entry_count$`, "agent/log_entry_count",
				otel.AggregateLabels("sum", "response_code")),
		),
		otel.TransformationMetrics(
			// Set unit = "1" to metrics who may not have it.
			otel.TransformQuery{
				Context:   otel.Metric,
				Statement: `set(unit, "1")`,
			},
		),
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
		"start_at":      "beginning",
	}
	return otel.ReceiverPipeline{
		Receiver: otel.Component{
			Type:   "otlpjsonfile",
			Config: receiverConfig,
		},
		Processors: map[string][]otel.Component{
			"metrics": {
				otel.Transform("metric", "datapoint", []ottl.Statement{"set(time, Now())"}),
				otel.MetricsRemoveInstrumentationLibraryLabelsAttributes(),
				otel.MetricsRemoveServiceAttributes(),
			},
		},
	}
}

// intentionally not registered as a component because this is not created by users
