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

type MetricsReceiverNvml struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`
   ProcessMetrics bool `yaml:"process_metrics"`
}

func (r MetricsReceiverNvml) Type() string {
	return "nvml"
}

func (r MetricsReceiverNvml) Pipelines() []otel.Pipeline {
   metrics := map[string]interface{}{
     "nvml.gpu.utilization": map[string]bool {
        "enabled": true,
     },
     "nvml.gpu.memory.bytes_used": map[string]bool {
        "enabled": true,
     },
   }

   if r.ProcessMetrics {
      metrics = map[string]interface{}{
         "nvml.processes.lifetime_gpu_utilization": map[string]bool {
            "enabled": true,
         },
         "nvml.processes.lifetime_gpu_max_bytes_used": map[string]bool {
            "enabled": true,
         },
      }
   }

	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "nvml",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
            "metrics": metrics,
			},
		},
		Processors: []otel.Component{
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.RenameMetric(
					"nvml.gpu.utilization",
					"gpu/utilization",
				),
				otel.RenameMetric(
					"nvml.gpu.memory.bytes_used",
					"gpu/memory/bytes_used",
				),
				otel.RenameMetric(
					"nvml.processes.lifetime_gpu_utilization",
					"processes/gpu/lifetime_utilization",
				),
				otel.RenameMetric(
					"nvml.processes.lifetime_gpu_max_bytes_used",
					"processes/gpu/lifetime_max_bytes_used",
				),
				otel.AddPrefix("workload.googleapis.com"),
			),
		},
	}}
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverNvml{} }, "linux")
}
