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
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
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
