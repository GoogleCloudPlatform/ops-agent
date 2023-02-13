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

type MetricsReceiverMssql struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`

	confgenerator.VersionedReceivers `yaml:",inline"`
}

func (MetricsReceiverMssql) Type() string {
	return "mssql"
}

func (m MetricsReceiverMssql) Pipelines() []otel.ReceiverPipeline {
	if m.ReceiverVersion == "2" {
		return []otel.ReceiverPipeline{{
			Receiver: otel.Component{
				Type: "sqlserver",
				Config: map[string]interface{}{
					"collection_interval": m.CollectionIntervalString(),
				},
			},
			Processors: map[string][]otel.Component{"metrics": {
				otel.MetricsTransform(
					otel.RenameMetric(
						"sqlserver.transaction_log.usage",
						"sqlserver.transaction_log.percent_used",
					),
					otel.AddPrefix("workload.googleapis.com"),
				),
				otel.TransformationMetrics(
					otel.FlattenResourceAttribute("sqlserver.database.name", "database"),
				),
				otel.NormalizeSums(),
				otel.ModifyInstrumentationScope(m.Type(), "2.0"),
			}},
		}}
	}

	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type: "windowsperfcounters",
			Config: map[string]interface{}{
				"collection_interval": m.CollectionIntervalString(),
				"perfcounters": []map[string]interface{}{
					{
						"object":    "SQLServer:General Statistics",
						"instances": []string{"_Total"},
						"counters":  []map[string]string{{"name": "User Connections"}},
					},
					{
						"object":    "SQLServer:Databases",
						"instances": []string{"_Total"},
						"counters": []map[string]string{
							{"name": "Transactions/sec"},
							{"name": "Write Transactions/sec"},
						},
					},
				},
			},
		},
		Type: otel.System,
		Processors: map[string][]otel.Component{"metrics": {
			otel.MetricsTransform(
				otel.RenameMetric(
					`\SQLServer:General Statistics(_Total)\User Connections`,
					"mssql/connections/user",
				),
				otel.RenameMetric(
					`\SQLServer:Databases(_Total)\Transactions/sec`,
					"mssql/transaction_rate",
				),
				otel.RenameMetric(
					`\SQLServer:Databases(_Total)\Write Transactions/sec`,
					"mssql/write_transaction_rate",
				),
				otel.AddPrefix("agent.googleapis.com"),
			),
			otel.ModifyInstrumentationScope(m.Type(), "1.0"),
		}},
	}}
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverMssql{} }, "windows")
}
