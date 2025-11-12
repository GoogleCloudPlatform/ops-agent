// Copyright 2022 Google LLC
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
	"context"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

type MetricsReceiverDcgm struct {
	confgenerator.ConfigComponent       `yaml:",inline"`
	confgenerator.MetricsReceiverShared `yaml:",inline"`
	confgenerator.VersionedReceivers    `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,hostname_port"`
}

const defaultDcgmEndpoint = "localhost:5555"

func (r MetricsReceiverDcgm) Type() string {
	return "dcgm"
}

func (r MetricsReceiverDcgm) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	if r.Endpoint == "" {
		r.Endpoint = defaultDcgmEndpoint
	}

	if r.ReceiverVersion == "2" {
		return []otel.ReceiverPipeline{{
			Receiver: otel.Component{
				Type: "dcgm",
				Config: map[string]interface{}{
					"collection_interval": r.CollectionIntervalString(),
					"endpoint":            r.Endpoint,
				},
			},
			Processors: map[string][]otel.Component{"metrics": {
				otel.MetricsTransform(
					otel.UpdateMetric(
						"gpu.dcgm.pipe.utilization",
						otel.RenameLabel("gpu.pipe", "pipe"),
					),
					otel.UpdateMetric(
						"gpu.dcgm.memory.bytes_used",
						otel.RenameLabel("gpu.memory.state", "state"),
					),
					otel.UpdateMetric(
						"gpu.dcgm.nvlink.io",
						otel.RenameLabel("network.io.direction", "direction"),
					),
					otel.UpdateMetric(
						"gpu.dcgm.pcie.io",
						otel.RenameLabel("network.io.direction", "direction"),
					),
					otel.UpdateMetric(
						"gpu.dcgm.clock.throttle_duration.time",
						otel.RenameLabel("gpu.clock.violation", "violation"),
					),
					otel.UpdateMetric(
						"gpu.dcgm.ecc_errors",
						otel.RenameLabel("gpu.error.type", "error_type"),
					),
					otel.UpdateMetric(
						"gpu.dcgm.xid_errors",
						otel.RenameLabel("gpu.error.xid", "xid"),
					),
				),
				otel.MetricsTransform(
					otel.AddPrefix("workload.googleapis.com"),
				),
				otel.TransformationMetrics(
					otel.FlattenResourceAttribute("gpu.model", "model"),
					otel.FlattenResourceAttribute("gpu.number", "gpu_number"),
					otel.FlattenResourceAttribute("gpu.uuid", "uuid"),
					otel.SetScopeName("agent.googleapis.com/"+r.Type()),
					otel.SetScopeVersion("2.0"),
				),
			}},
		}}, nil
	}

	disabledV1Metrics := []string{
		"gpu.dcgm.utilization",
		"gpu.dcgm.codec.encoder.utilization",
		"gpu.dcgm.codec.decoder.utilization",
		"gpu.dcgm.memory.bytes_used",
		"gpu.dcgm.energy_consumption",
		"gpu.dcgm.temperature",
		"gpu.dcgm.clock.frequency",
		"gpu.dcgm.clock.throttle_duration.time",
		"gpu.dcgm.ecc_errors",
		"gpu.dcgm.xid_errors",
	}
	enabledV1Metrics := []string{
		"gpu.dcgm.sm.utilization",
		"gpu.dcgm.sm.occupancy",
		"gpu.dcgm.pipe.utilization",
		"gpu.dcgm.memory.bandwidth_utilization",
		"gpu.dcgm.pcie.io",
		"gpu.dcgm.nvlink.io",
	}

	metricsConfig := make(map[string]interface{})
	for _, m := range disabledV1Metrics {
		metricsConfig[m] = map[string]bool{
			"enabled": false,
		}
	}
	for _, m := range enabledV1Metrics {
		metricsConfig[m] = map[string]bool{
			"enabled": true,
		}
	}

	return []otel.ReceiverPipeline{confgenerator.ConvertToOtlpExporter(otel.ReceiverPipeline{
		Receiver: otel.Component{
			Type: "dcgm",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
				"endpoint":            r.Endpoint,
				"metrics":             metricsConfig,
			},
		},
		Processors: map[string][]otel.Component{"metrics": {
			otel.MetricsTransform(
				otel.RenameMetric(
					"gpu.dcgm.memory.bandwidth_utilization",
					"dcgm.gpu.profiling.dram_utilization",
				),
				otel.RenameMetric(
					"gpu.dcgm.nvlink.io",
					"dcgm.gpu.profiling.nvlink_traffic_rate",
					otel.RenameLabel("network.io.direction", "direction"),
					otel.RenameLabelValues("direction", map[string]string{
						"receive":  "rx",
						"transmit": "tx",
					}),
				),
				otel.RenameMetric(
					"gpu.dcgm.pcie.io",
					"dcgm.gpu.profiling.pcie_traffic_rate",
					otel.RenameLabel("network.io.direction", "direction"),
					otel.RenameLabelValues("direction", map[string]string{
						"receive":  "rx",
						"transmit": "tx",
					}),
				),
				otel.RenameMetric(
					"gpu.dcgm.pipe.utilization",
					"dcgm.gpu.profiling.pipe_utilization",
					otel.RenameLabel("gpu.pipe", "pipe"),
				),
				otel.RenameMetric(
					"gpu.dcgm.sm.occupancy",
					"dcgm.gpu.profiling.sm_occupancy",
				),
				otel.RenameMetric(
					"gpu.dcgm.sm.utilization",
					"dcgm.gpu.profiling.sm_utilization",
				),
			),
			otel.CumulativeToDelta(
				"dcgm.gpu.profiling.nvlink_traffic_rate",
				"dcgm.gpu.profiling.pcie_traffic_rate",
			),
			otel.DeltaToRate(
				"dcgm.gpu.profiling.nvlink_traffic_rate",
				"dcgm.gpu.profiling.pcie_traffic_rate",
			),
			otel.MetricsTransform(
				otel.UpdateMetric(
					"dcgm.gpu.profiling.nvlink_traffic_rate",
					otel.ToggleScalarDataType,
				),
				otel.UpdateMetric(
					"dcgm.gpu.profiling.pcie_traffic_rate",
					otel.ToggleScalarDataType,
				),
			),
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com"),
			),
			otel.TransformationMetrics(
				otel.FlattenResourceAttribute("gpu.model", "model"),
				otel.FlattenResourceAttribute("gpu.number", "gpu_number"),
				otel.FlattenResourceAttribute("gpu.uuid", "uuid"),
				otel.SetScopeName("agent.googleapis.com/"+r.Type()),
				otel.SetScopeVersion("1.0"),
			),
		},
		},
	}, ctx)}, nil
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverDcgm{} }, platform.Linux)
}
