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
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverMssql struct {
	ConfigComponent `yaml:",inline"`

	MetricsReceiverShared `yaml:",inline"`
}

func (MetricsReceiverMssql) Type() string {
	return "mssql"
}

func (m MetricsReceiverMssql) Pipelines() []otel.Pipeline {
	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "windowsperfcounters",
			Config: map[string]interface{}{
				"collection_interval": m.CollectionIntervalString(),
				"perfcounters": []map[string]interface{}{
					{
						"object":    "SQLServer:General Statistics",
						"instances": []string{"_Total"},
						"counters":  []string{"User Connections"},
					},
					{
						"object":    "SQLServer:Databases",
						"instances": []string{"_Total"},
						"counters": []string{
							"Transactions/sec",
							"Write Transactions/sec",
						},
					},
				},
			},
		},
		Processors: []otel.Component{{
			Type: "metricstransform",
			Config: map[string]interface{}{
				"transforms": []map[string]interface{}{
					{
						"include":  `\SQLServer:General Statistics(_Total)\User Connections`,
						"action":   "update",
						"new_name": "mssql/connections/user",
					},
					{
						"include":  `\SQLServer:Databases(_Total)\Transactions/sec`,
						"action":   "update",
						"new_name": "mssql/connections/transaction_rate",
					},
					{
						"include":  `\SQLServer:Databases(_Total)\Write Transactions/sec`,
						"action":   "update",
						"new_name": "mssql/connections/write_transaction_rate",
					},
				},
			},
		}},
	}}
}

func init() {
	metricsReceiverTypes.registerType(func() component { return &MetricsReceiverMssql{} }, "windows")
}
