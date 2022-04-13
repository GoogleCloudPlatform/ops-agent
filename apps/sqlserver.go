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

type MetricsReceiverSqlServer struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`
}

func (MetricsReceiverSqlServer) Type() string {
	return "sqlserver"
}

func (r MetricsReceiverSqlServer) Pipelines() []otel.Pipeline {
	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "sqlserver",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
			},
		},
		Processors: []otel.Component{
			otel.MetricsTransform(
				otel.RenameMetric(
					"sqlserver.transaction_log.usage",
					"sqlserver.transaction_log.percent_used",
				),
				otel.AddPrefix("workload.googleapis.com"),
			),
			otel.CastToSum(
				"workload.googleapis.com/sqlserver.transaction_log.growth.count",
				"workload.googleapis.com/sqlserver.transaction_log.shrink.count",
			),
			otel.NormalizeSums(),
		},
	}}
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverSqlServer{} }, "windows")
}
