// Copyright 2023 Google LLC
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
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

type MetricsReceiverNvml struct {
	confgenerator.ConfigComponent       `yaml:",inline"`
	confgenerator.MetricsReceiverShared `yaml:",inline"`
}

func (r MetricsReceiverNvml) Type() string {
	return "nvml"
}

func (r MetricsReceiverNvml) Pipelines() []otel.ReceiverPipeline {
	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type: "nvml",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
			},
		},
		Processors: map[string][]otel.Component{"metrics": {
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.RenameMetric(
					"nvml.gpu.utilization",
					"gpu/utilization",
					otel.ScaleValue(100),
				),
				otel.RenameMetric(
					"nvml.gpu.memory.bytes_used",
					"gpu/memory/bytes_used",
				),
				otel.RenameMetric(
					"nvml.gpu.processes.utilization",
					"gpu/processes/utilization",
					otel.ScaleValue(100),
				),
				otel.RenameMetric(
					"nvml.gpu.processes.max_bytes_used",
					"gpu/processes/max_bytes_used",
				),
				otel.AddPrefix("agent.googleapis.com"),
			),
		}},
	}}
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverNvml{} }, platform.Linux)
}
