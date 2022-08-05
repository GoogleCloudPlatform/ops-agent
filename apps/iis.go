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
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverIis struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`
	ReceiverVersion                     string `yaml:"receiver_version,omitempty"`
}

func (r MetricsReceiverIis) Type() string {
	return "iis"
}

func (r MetricsReceiverIis) Pipelines() []otel.Pipeline {
	if r.ReceiverVersion == "2" {
		return []otel.Pipeline{{
			Receiver: otel.Component{
				Type: "iis",
				Config: map[string]interface{}{
					"collection_interval": r.CollectionIntervalString(),
				},
			},
			Processors: []otel.Component{
				otel.NormalizeSums(),
				otel.MetricsTransform(
					otel.AddPrefix("workload.googleapis.com"),
				),
			},
		}}
	}

	// Return version 1 if version is anything other than 2
	return []otel.Pipeline{{
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
		Processors: []otel.Component{
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
		},
	}}
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverIis{} }, "windows")
}

type LoggingProcessorIisAccess struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (*LoggingProcessorIisAccess) Type() string {
	return "iis_access"
}

const (
	iisMergeRecordFieldsLuaFunction       = `iis_merge_fields`
	iisMergeRecordFieldsLuaScriptContents = `
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
)

func (p *LoggingProcessorIisAccess) Components(tag, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseRegex{
		// Documentation:
		// https://docs.microsoft.com/en-us/windows/win32/http/w3c-logging
		// sample line: 2022-03-10 17:26:30 ::1 GET /iisstart.png - 80 - ::1 Mozilla/5.0+(Windows+NT+10.0;+WOW64;+Trident/7.0;+rv:11.0)+like+Gecko http://localhost/ 200 0 0 18
		// sample line: 2022-03-10 17:26:30 ::1 GET / - 80 - ::1 Mozilla/5.0+(Windows+NT+10.0;+WOW64;+Trident/7.0;+rv:11.0)+like+Gecko - 200 0 0 352
		// sample line: 2022-03-10 17:26:32 ::1 GET /favicon.ico - 80 - ::1 Mozilla/5.0+(Windows+NT+10.0;+WOW64;+Trident/7.0;+rv:11.0)+like+Gecko - 404 0 2 49
		Regex: `^(?<timestamp>\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2})\s(?<http_request_serverIp>[^\s]+)\s(?<http_request_requestMethod>[^\s]+)\s(?<cs_uri_stem>\/[^\s]*)\s(?<cs_uri_query>[^\s]*)\s(?<s_port>\d*)\s(?<user>[^\s]+)\s(?<http_request_remoteIp>[^\s]+)\s(?<http_request_userAgent>[^\s]+)\s(?<http_request_referer>[^\s]+)\s(?<http_request_status>\d{3})\s(?<sc_substatus>\d+)\s(?<sc_win32_status>\d+)\s(?<time_taken>\d+)$`,
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "timestamp",
			TimeFormat: " %Y-%m-%d %H:%M:%S",
			Types: map[string]string{
				"http_request_status": "integer",
			},
		},
	}.Components(tag, uid)

	// iis logs "-" when a field does not have a value. Remove the field entirely when this happens.
	c = append(c, fluentbit.LuaFilterComponents(tag, iisMergeRecordFieldsLuaFunction, iisMergeRecordFieldsLuaScriptContents)...)

	c = append(c, []fluentbit.Component{
		// This is used to exlude the header lines above the logs

		// EXAMPLE LINES:
		// #Software: Microsoft Internet Information Services 10.0
		// #Version: 1.0
		// #Date: 2022-04-11 12:53:50
		{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":    "grep",
				"Match":   tag,
				"Exclude": "message ^#(?:Fields|Date|Version|Software):",
			},
		},
	}...)

	fields := map[string]*confgenerator.ModifyField{
		InstrumentationSourceLabel: instrumentationSourceValue(p.Type()),
	}

	// Generate the httpRequest structure.
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

	c = append(c,
		confgenerator.LoggingProcessorModifyFields{
			Fields: fields,
		}.Components(tag, uid)...,
	)
	return c
}

type LoggingReceiverIisAccess struct {
	LoggingProcessorIisAccess               `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverIisAccess) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			`C:\inetpub\logs\LogFiles\W3SVC1\u_ex*`,
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorIisAccess.Components(tag, "iis_access")...)
	return c
}

func init() {
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverIisAccess{} }, "windows")
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorIisAccess{} }, "windows")
}
