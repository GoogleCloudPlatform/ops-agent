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

type MetricsReceiverDcgm struct {
	confgenerator.ConfigComponent `yaml:",inline"`
	confgenerator.MetricsReceiverShared `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty"`
   ProfilingMetrics bool `yaml:"profiling_metrics"`
}

const defaultDcgmEndpoint = "localhost:5555"

func (r MetricsReceiverDcgm) Type() string {
	return "dcgm"
}

func (r MetricsReceiverDcgm) Pipelines() []otel.Pipeline {
   if r.Endpoint == "" {
      r.Endpoint = defaultDcgmEndpoint
   }

   metrics := map[string]interface{}{
     "dcgm.gpu.utilization": map[string]bool {
        "enabled": true,
     },
     "dcgm.gpu.memory.bytes_used": map[string]bool {
        "enabled": true,
     },
   }

   if r.ProfilingMetrics {
      metrics = map[string]interface{}{
         "dcgm.gpu.profiling.sm_utilization": map[string]bool {
            "enabled": true,
         },
         "dcgm.gpu.profiling.sm_occupancy": map[string]bool {
            "enabled": true,
         },
         "dcgm.gpu.profiling.pipe_utilization": map[string]bool {
            "enabled": true,
         },
         "dcgm.gpu.profiling.dram_utilization": map[string]bool {
            "enabled": true,
         },
         "dcgm.gpu.profiling.pcie_traffic_rate": map[string]bool {
            "enabled": true,
         },
         "dcgm.gpu.profiling.nvlink_traffic_rate": map[string]bool {
            "enabled": true,
         },
      }
   }

	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "dcgm",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
            "endpoint": r.Endpoint,
            "metrics": metrics,
			},
		},
		Processors: []otel.Component{
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.RenameMetric(
					"dcgm.gpu.utilization",
					"dcgm/gpu/utilization",
				),
				otel.RenameMetric(
					"dcgm.gpu.memory.bytes_used",
					"dcgm/gpu/memory/bytes_used",
				),
				otel.RenameMetric(
					"dcgm.gpu.profiling.sm_utilization",
					"dcgm/gpu/profiling/sm_utilization",
				),
				otel.RenameMetric(
					"dcgm.gpu.profiling.sm_occupancy",
					"dcgm/gpu/profiling/sm_occupancy",
				),
				otel.RenameMetric(
					"dcgm.gpu.profiling.pipe_utilization",
					"dcgm/gpu/profiling/pipe_utilization",
				),
				otel.RenameMetric(
					"dcgm.gpu.profiling.dram_utilization",
					"dcgm/gpu/profiling/dram_utilization",
				),
				otel.RenameMetric(
					"dcgm.gpu.profiling.pcie_traffic_rate",
					"dcgm/gpu/profiling/pcie_traffic_rate",
				),
				otel.RenameMetric(
					"dcgm.gpu.profiling.nvlink_traffic_rate",
					"dcgm/gpu/profiling/nvlink_traffic_rate",
				),
				otel.AddPrefix("workload.googleapis.com"),
			),
		},
	}}
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverDcgm{} }, "linux")
}


