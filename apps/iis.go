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

package apps

import (
	"context"
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel/ottl"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

type MetricsReceiverIis struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`

	confgenerator.VersionedReceivers `yaml:",inline"`
}

func (r MetricsReceiverIis) Type() string {
	return "iis"
}

func (r MetricsReceiverIis) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	if r.ReceiverVersion == "2" {
		return []otel.ReceiverPipeline{confgenerator.ConvertGCMOtelExporterToOtlpExporter(otel.ReceiverPipeline{
			Receiver: otel.Component{
				Type: "iis",
				Config: map[string]interface{}{
					"collection_interval": r.CollectionIntervalString(),
				},
			},
			Processors: map[string][]otel.Component{"metrics": {
				otel.TransformationMetrics(
					otel.FlattenResourceAttribute("iis.site", "site"),
					otel.FlattenResourceAttribute("iis.application_pool", "app_pool"),
					otel.SetScopeName("agent.googleapis.com/"+r.Type()),
					otel.SetScopeVersion("2.0"),
				),
				otel.MetricsRemoveServiceAttributes(),
				// Drop all resource keys; Must be done in a separate transform,
				// otherwise the above flatten resource attribute queries will only
				// work for the first datapoint
				otel.TransformationMetrics(
					otel.RetainResource(),
				),
				otel.CondenseResourceMetrics(),
				otel.MetricsTransform(
					otel.UpdateMetricRegexp("^iis",
						otel.AggregateLabels(
							"sum",
							"direction",
							"request",
						),
					),
					otel.AddPrefix("workload.googleapis.com"),
				),
				otel.NormalizeSums(),
			}},
		}, ctx)}, nil
	}

	// Return version 1 if version is anything other than 2
	return []otel.ReceiverPipeline{confgenerator.ConvertGCMSystemExporterToOtlpExporter(otel.ReceiverPipeline{
		Receiver: otel.Component{
			Type: "windowsperfcounters",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
				"perfcounters": []map[string]interface{}{
					{
						"object":    "Web Service",
						"instances": []string{"_Total"},
						"counters": []map[string]string{
							{"name": "Current Connections"},
							{"name": "Total Bytes Received"},
							{"name": "Total Bytes Sent"},
							{"name": "Total Connection Attempts (all instances)"},
							{"name": "Total Delete Requests"},
							{"name": "Total Get Requests"},
							{"name": "Total Head Requests"},
							{"name": "Total Options Requests"},
							{"name": "Total Post Requests"},
							{"name": "Total Put Requests"},
							{"name": "Total Trace Requests"},
						},
					},
				},
			},
		},
		ExporterTypes: map[string]otel.ExporterType{
			"metrics": otel.System,
		},
		Processors: map[string][]otel.Component{"metrics": {
			otel.MetricsTransform(
				otel.RenameMetric(
					`\Web Service(_Total)\Current Connections`,
					"iis/current_connections",
				),
				// $ needs to be escaped because reasons.
				// https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/metricstransformprocessor#rename-multiple-metrics-using-substitution
				otel.CombineMetrics(
					`^\\Web Service\(_Total\)\\Total Bytes (?P<direction>.*)$$`,
					"iis/network/transferred_bytes_count",
					// change data type from double -> int64
					otel.ToggleScalarDataType,
				),
				otel.RenameMetric(
					`\Web Service(_Total)\Total Connection Attempts (all instances)`,
					"iis/new_connection_count",
					// change data type from double -> int64
					otel.ToggleScalarDataType,
				),
				otel.CombineMetrics(
					`^\\Web Service\(_Total\)\\Total (?P<http_method>.*) Requests$$`,
					"iis/request_count",
					// change data type from double -> int64
					otel.ToggleScalarDataType,
				),
				otel.AddPrefix("agent.googleapis.com"),
			),
			otel.CastToSum(
				"agent.googleapis.com/iis/network/transferred_bytes_count",
				"agent.googleapis.com/iis/new_connection_count",
				"agent.googleapis.com/iis/request_count",
			),
			otel.NormalizeSums(),
			otel.TransformationMetrics(
				otel.SetScopeName("agent.googleapis.com/"+r.Type()),
				otel.SetScopeVersion("1.0"),
			),
		}},
	}, ctx)}, nil
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverIis{} }, platform.Windows)
}

type LoggingProcessorMacroIisAccess struct {
}

func (LoggingProcessorMacroIisAccess) Type() string {
	return "iis_access"
}

// IISConcatFields handles field concatenation for IIS logs
type IISConcatFields struct{}

// Components implements the Fluent-bit concatenation using Lua
func (IISConcatFields) Components(ctx context.Context, tag, uid string) []fluentbit.Component {
	iisMergeRecordFieldsLuaFunction := `iis_merge_fields`
	iisMergeRecordFieldsLuaScriptContents := `
	function iis_merge_fields(tag, timestamp, record)

	  if (record["cs_uri_query"] == "-") then
	    record["cs_uri_query"] = nil
	  end
	  if (record["http_request_referer"] == "-") then
	    record["http_request_referer"] = nil
	  end
	  if (record["user"] == "-") then
	    record["user"] = nil
	  end

	  record["http_request_serverIp"] = table.concat({record["http_request_serverIp"], ":", record["s_port"]})
	  if (record["cs_uri_query"] == nil or record["cs_uri_query"] == '') then
		record["http_request_requestUrl"] = record["cs_uri_stem"]
	  else
		record["http_request_requestUrl"] = table.concat({record["cs_uri_stem"], "?", record["cs_uri_query"]})
	  end

	  record["cs_uri_query"] = nil
	  record["cs_uri_stem"] = nil
	  record["s_port"] = nil
	  return 2, timestamp, record
	end
	`

	return fluentbit.LuaFilterComponents(tag, iisMergeRecordFieldsLuaFunction, iisMergeRecordFieldsLuaScriptContents)
}

// Processors implements the OTEL concatenation using ModifyFields + CustomConvertFunc
func (IISConcatFields) Processors(ctx context.Context) ([]otel.Component, error) {
	// Required OTTL fields
	bodyHttpRequestServerIp := ottl.LValue{"body", "http_request_serverIp"}
	bodySPort := ottl.LValue{"body", "s_port"}
	bodyUriQuery := ottl.LValue{"body", "cs_uri_query"}
	bodyUriStem := ottl.LValue{"body", "cs_uri_stem"}

	modifyFields := confgenerator.LoggingProcessorModifyFields{
		Fields: map[string]*confgenerator.ModifyField{
			// Omit fields equal to "-"
			"jsonPayload.cs_uri_query": {
				OmitIf: `jsonPayload.cs_uri_query = "-"`,
			},
			"jsonPayload.http_request_referer": {
				OmitIf: `jsonPayload.referer = "-"`,
			},
			"jsonPayload.user": {
				OmitIf: `jsonPayload.user = "-"`,
			},
			// Concatenate serverIp with port
			"jsonPayload.http_request_serverIp": {
				CopyFrom: "jsonPayload.http_request_serverIp",
				CustomConvertFunc: func(v ottl.LValue) ottl.Statements {
					return ottl.NewStatements(
						v.Set(ottl.Concat([]ottl.Value{bodyHttpRequestServerIp, bodySPort}, ":")),
						bodySPort.Delete(),
					)
				},
			},
			// Build requestUrl from stem and query
			"jsonPayload.http_request_requestUrl": {
				CopyFrom: "jsonPayload.cs_uri_stem",
				CustomConvertFunc: func(v ottl.LValue) ottl.Statements {
					return ottl.NewStatements(
						// Set requestUrl to stem when query is empty/"-"
						v.SetIf(
							bodyUriStem,
							ottl.And(
								bodyUriStem.IsPresent(),
								ottl.Or(
									ottl.Not(ottl.IsNotNil(bodyUriQuery)),
									ottl.Equals(bodyUriQuery, ottl.StringLiteral("")),
									ottl.Equals(bodyUriQuery, ottl.StringLiteral("-")),
								),
							),
						),
						// Set requestUrl to stem + "?" + query when query has content
						v.SetIf(
							ottl.Concat([]ottl.Value{bodyUriStem, bodyUriQuery}, "?"),
							ottl.And(bodyUriStem.IsPresent(), bodyUriQuery.IsPresent()),
						),
						// Clean up intermediate fields
						bodyUriQuery.Delete(),
						bodyUriStem.Delete(),
					)
				},
			},
		},
	}

	return modifyFields.Processors(ctx)
}

func (p LoggingProcessorMacroIisAccess) Expand(ctx context.Context) []confgenerator.InternalLoggingProcessor {
	parseRegex := confgenerator.LoggingProcessorParseRegex{
		// Microsoft updated the default format in Feb 2026.
		// The new format now has fields sc_bytes and cs_bytes added right before time_taken
		// To ensure backwards compatibility, we added an optional capture group right before time_taken
		// Documentation:
		// https://docs.microsoft.com/en-us/windows/win32/http/w3c-logging
		// sample line old format: 2022-03-10 17:26:30 ::1 GET /iisstart.png - 80 - ::1 Mozilla/5.0+(Windows+NT+10.0;+WOW64;+Trident/7.0;+rv:11.0)+like+Gecko http://localhost/ 200 0 0 18
		// sample line old format: 2022-03-10 17:26:30 ::1 GET / - 80 - ::1 Mozilla/5.0+(Windows+NT+10.0;+WOW64;+Trident/7.0;+rv:11.0)+like+Gecko - 200 0 0 352
		// sample line old format: 2022-03-10 17:26:32 ::1 GET /favicon.ico - 80 - ::1 Mozilla/5.0+(Windows+NT+10.0;+WOW64;+Trident/7.0;+rv:11.0)+like+Gecko - 404 0 2 49
		// sample line new format: 2026-02-19 10:28:49 ::1 GET /forbidden something=something 80 - ::1 Mozilla/5.0+(Windows+NT;+Windows+NT+10.0;+en-US)+WindowsPowerShell/5.1.26100.32370 - 404 0 2 5035 184 189
		Regex: `^(?<timestamp>\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2})\s(?<http_request_serverIp>[^\s]+)\s(?<http_request_requestMethod>[^\s]+)\s(?<cs_uri_stem>\/[^\s]*)\s(?<cs_uri_query>[^\s]*)\s(?<s_port>\d*)\s(?<user>[^\s]+)\s(?<http_request_remoteIp>[^\s]+)\s(?<http_request_userAgent>[^\s]+)\s(?<http_request_referer>[^\s]+)\s(?<http_request_status>\d{3})\s(?<sc_substatus>\d+)\s(?<sc_win32_status>\d+)(?:\s(?<sc_bytes>\d+)\s(?<cs_bytes>\d+))?\s(?<time_taken>\d+)$`,
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "timestamp",
			TimeFormat: "%Y-%m-%d %H:%M:%S",
			Types: map[string]string{
				"http_request_status": "integer",
			},
		},
	}

	// Handle field concatenation (serverIp+port, requestUrl)
	concatFields := IISConcatFields{}

	// This is used to exclude the header lines above the logs
	// EXAMPLE LINES:
	// #Software: Microsoft Internet Information Services 10.0
	// #Version: 1.0
	// #Date: 2022-04-11 12:53:50
	excludeLogs := confgenerator.LoggingProcessorExcludeLogs{
		MatchAny: []string{`jsonPayload.message=~"^#(?:Fields|Date|Version|Software):"`},
	}

	// Create fields map for simple field operations and moves
	fields := map[string]*confgenerator.ModifyField{
		InstrumentationSourceLabel: instrumentationSourceValue(p.Type()),
	}

	// Generate the httpRequest structure field moves
	for _, field := range []string{
		"serverIp",
		"remoteIp",
		"requestMethod",
		"requestUrl",
		"status",
		"referer",
		"userAgent",
	} {
		fields[fmt.Sprintf("httpRequest.%s", field)] = &confgenerator.ModifyField{
			MoveFrom: fmt.Sprintf("jsonPayload.http_request_%s", field),
		}
	}

	modifyFields := confgenerator.LoggingProcessorModifyFields{
		Fields: fields,
	}

	return []confgenerator.InternalLoggingProcessor{
		parseRegex,
		concatFields,
		excludeLogs,
		modifyFields,
	}
}

func loggingReceiverFilesMixinIisAccess() confgenerator.LoggingReceiverFilesMixin {
	return confgenerator.LoggingReceiverFilesMixin{
		IncludePaths: []string{
			`C:\inetpub\logs\LogFiles\W3SVC1\u_ex*`,
		},
	}
}

func init() {
	confgenerator.RegisterLoggingFilesProcessorMacro[LoggingProcessorMacroIisAccess](
		loggingReceiverFilesMixinIisAccess,
		platform.Windows,
	)
}
