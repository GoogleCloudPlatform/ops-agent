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
}

func (r MetricsReceiverIis) Type() string {
	return "iis"
}

func (r MetricsReceiverIis) Pipelines() []otel.Pipeline {
	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "windowsperfcounters",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
				"perfcounters": []map[string]interface{}{
					{
						"object":    "Web Service",
						"instances": []string{"_Total"},
						"counters": []string{
							"Current Connections",
							"Total Bytes Received",
							"Total Bytes Sent",
							"Total Connection Attempts (all instances)",
							"Total Delete Requests",
							"Total Get Requests",
							"Total Head Requests",
							"Total Options Requests",
							"Total Post Requests",
							"Total Put Requests",
							"Total Trace Requests",
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

type LoggingProcessorIis struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (*LoggingProcessorIis) Type() string {
	return "iis_access"
}

const (
	iisMergeRecordFieldsLuaFunction       = `iis_merge_fields`
	iisMergeRecordFieldsLuaScriptContents = `
	function iis_merge_fields(tag, timestamp, record)
	  record["http_request_serverIp"] = table.concat({record["http_request_serverIp"], ":", record["s_port"]})
	  if (record["cs_uri_query"] == nil or record["cs_uri_query"] == '') then
		record["http_request_requestUrl"] = record["cs_uri_stem"]
	  else
		record["http_request_requestUrl"] = table.concat({record["cs_uri_stem"], "?", record["cs_uri_query"]})
	  end
	  return 2, timestamp, record
	end
	`
)

func (p *LoggingProcessorIis) Components(tag, uid string) []fluentbit.Component {
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
				"sc_win32_status":     "integer",
				"sc_substatus":        "integer",
				"http_request_status": "integer",
				"time_taken":          "integer",
			},
		},
	}.Components(tag, uid)
	// iis logs "-" when a field does not have a value. Remove the field entirely when this happens.
	for _, field := range []string{
		"cs_uri_query",
		"http_request_referer",
		"cs_username",
	} {
		c = append(c, fluentbit.Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":      "modify",
				"Match":     tag,
				"Condition": fmt.Sprintf("Key_Value_Equals %s -", field),
				"Remove":    field,
			},
		})
	}

	c = append(c, fluentbit.LuaFilterComponents(tag, iisMergeRecordFieldsLuaFunction, iisMergeRecordFieldsLuaScriptContents)...)

	// Remove fields that were merged
	for _, field := range []string{
		"cs_uri_query",
		"cs_uri_stem",
		"s_port",
	} {
		c = append(c, fluentbit.Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":       "record_modifier",
				"Match":      tag,
				"Remove_key": field,
			},
		})
	}

	c = append(c, []fluentbit.Component{
		{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":    "grep",
				"Match":   tag,
				"Exclude": "message ^#(?:Fields|Date|Version|Software):",
			},
		},
		// Generate the httpRequest structure.
		{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":          "nest",
				"Match":         tag,
				"Operation":     "nest",
				"Wildcard":      "http_request_*",
				"Nest_under":    "logging.googleapis.com/http_request",
				"Remove_prefix": "http_request_",
			},
		},
	}...)
	return c
}

type AccessLoggingReceiverIis struct {
	LoggingProcessorIis                     `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r AccessLoggingReceiverIis) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			"/inetpub/logs/LogFiles/W3SVC1/u_ex*",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorIis.Components(tag, "iis_access")...)
	return c
}

func init() {
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &AccessLoggingReceiverIis{} }, "windows")
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorIis{} }, "windows")

}
