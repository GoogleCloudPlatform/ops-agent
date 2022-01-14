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

// MetricsReceiverLogging provides the agent.googleapis.com/agent/ logging metrics.
// It is never referenced in the config file, and instead is forcibly added in confgenerator.go.
// Therefore, it does not need to implement any interfaces.
type MetricsReceiverLogging struct {
	Version string
}

func (r MetricsReceiverLogging) Pipeline() otel.Pipeline {
	return otel.Pipeline{
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
							"targets": []string{"0.0.0.0:2222"},
						}},
					}},
				},
			},
		},
		Processors: []otel.Component{
			otel.MetricsFilter(
				"include",
				"strict",
				"fluentbit_uptime",
			),
			otel.MetricsTransform(
				otel.RenameMetric("fluentbit_uptime", "agent/uptime",
					// change data type from double -> int64
					otel.ToggleScalarDataType,
					otel.AddLabel("version", r.Version),
					// remove service.version label
					otel.AggregateLabels("sum", "version"),
				),
				otel.AddPrefix("agent.googleapis.com"),
			),
		},
	}
}

// intentionally not registered as a component because this is not created by users
