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

type MetricsReceiverHostmetrics struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`
}

func (r MetricsReceiverHostmetrics) Type() string {
	return "hostmetrics"
}

func (r MetricsReceiverHostmetrics) Pipelines() []otel.Pipeline {
	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "hostmetrics",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
				"scrapers": map[string]interface{}{
					"cpu":        struct{}{},
					"load":       struct{}{},
					"memory":     struct{}{},
					"disk":       struct{}{},
					"filesystem": struct{}{},
					"network":    struct{}{},
					"paging":     struct{}{},
					"process": map[string]bool{
						"mute_process_name_error": true,
					},
					"processes": struct{}{},
				},
			},
		},
		Processors: []otel.Component{
			{
				// perform custom transformations that aren't supported by the metricstransform processor
				Type: "agentmetrics",
				Config: map[string]interface{}{
					// https://github.com/GoogleCloudPlatform/opentelemetry-operations-collector/blob/master/processor/agentmetricsprocessor/agentmetricsprocessor.go#L58
					"blank_label_metrics": []string{
						"system.cpu.utilization",
					},
				},
			},
			otel.MetricsFilter(
				"exclude",
				"strict",
				// Temporarily exclude system.cpu.time (cpu/usage_time)
				"system.cpu.time",
				"system.network.dropped",
				"system.filesystem.inodes.usage",
				"system.paging.faults",
				"system.disk.operation_time",
			),
			otel.MetricsTransform(
				otel.RenameMetric(
					"system.cpu.time",
					"cpu/usage_time",
					// change data type from double -> int64
					otel.ToggleScalarDataType,
					otel.RenameLabel("cpu", "cpu_number"),
					otel.RenameLabel("state", "cpu_state"),
				),
				otel.RenameMetric(
					"system.cpu.utilization",
					"cpu/utilization",
					// take avg over cpu dimension, retaining only state label
					otel.AggregateLabels(
						"mean",
						"state",
						"blank",
					),
					// add blank cpu_number label
					otel.RenameLabel("blank", "cpu_number"),
					// change label state -> cpu_state
					otel.RenameLabel("state", "cpu_state"),
				),
				otel.RenameMetric(
					"system.cpu.load_average.1m",
					"cpu/load_1m",
				),
				otel.RenameMetric(
					"system.cpu.load_average.5m",
					"cpu/load_5m",
				),
				otel.RenameMetric(
					"system.cpu.load_average.15m",
					"cpu/load_15m",
				),
				otel.RenameMetric(
					"system.disk.read_io", // as named after custom split logic
					"disk/read_bytes_count",
				),
				otel.RenameMetric(
					"system.disk.write_io", // as named after custom split logic
					"disk/write_bytes_count",
				),
				otel.RenameMetric(
					"system.disk.operations",
					"disk/operation_count",
				),
				otel.RenameMetric(
					"system.disk.io_time",
					"disk/io_time",
					// convert s to ms
					otel.ScaleValue(1000),
					// change data type from double -> int64
					otel.ToggleScalarDataType,
				),
				otel.RenameMetric(
					"system.disk.weighted_io_time",
					"disk/weighted_io_time",
					// convert s to ms
					otel.ScaleValue(1000),
					// change data type from double -> int64
					otel.ToggleScalarDataType,
				),
				otel.RenameMetric(
					"system.disk.average_operation_time",
					"disk/operation_time",
					// convert s to ms
					otel.ScaleValue(1000),
					// change data type from double -> int64
					otel.ToggleScalarDataType,
				),
				otel.RenameMetric(
					"system.disk.pending_operations",
					"disk/pending_operations",
					// change data type from int64 -> double
					otel.ToggleScalarDataType,
				),
				otel.RenameMetric(
					"system.disk.merged",
					"disk/merged_operations",
				),
				otel.RenameMetric(
					"system.filesystem.usage",
					"disk/bytes_used",
					// change data type from int64 -> double
					otel.ToggleScalarDataType,
					// take max over mode, mountpoint & type dimensions, retaining only device & state
					// there may be multiple mountpoints for the same device
					otel.AggregateLabels("max", "device", "state"),
				),
				otel.RenameMetric(
					"system.filesystem.utilization",
					"disk/percent_used",
					otel.AggregateLabels("max", "device", "state"),
				),
				otel.RenameMetric(
					"system.memory.usage",
					"memory/bytes_used",
					// change data type from int64 -> double
					otel.ToggleScalarDataType,
					// aggregate state label values: slab_reclaimable & slab_unreclaimable -> slab (note this is not currently supported)
					otel.AggregateLabelValues("sum", "state", "slab", "slab_reclaimable", "slab_unreclaimable"),
				),
				otel.RenameMetric(
					"system.memory.utilization",
					"memory/percent_used",
					// sum state label values: slab = slab_reclaimable + slab_unreclaimable
					otel.AggregateLabelValues("sum", "state", "slab", "slab_reclaimable", "slab_unreclaimable"),
				),
				otel.RenameMetric(
					"system.network.io",
					"interface/traffic",
					otel.RenameLabel("interface", "device"),
					otel.RenameLabelValues("direction", map[string]string{
						"receive":  "rx",
						"transmit": "tx",
					}),
				),
				otel.RenameMetric(
					"system.network.errors",
					"interface/errors",
					otel.RenameLabel("interface", "device"),
					otel.RenameLabelValues("direction", map[string]string{
						"receive":  "rx",
						"transmit": "tx",
					}),
				),
				otel.RenameMetric(
					"system.network.packets",
					"interface/packets",
					otel.RenameLabel("interface", "device"),
					otel.RenameLabelValues("direction", map[string]string{
						"receive":  "rx",
						"transmit": "tx",
					}),
				),
				otel.RenameMetric(
					"system.network.connections",
					"network/tcp_connections",
					// change data type from int64 -> double
					otel.ToggleScalarDataType,
					// remove udp data
					otel.DeleteLabelValue("protocol", "udp"),
					otel.RenameLabel("state", "tcp_state"),
					// remove protocol label
					otel.AggregateLabels("sum", "tcp_state"),
					otel.AddLabel("port", "all"),
				),
				otel.RenameMetric(
					"system.processes.created",
					"processes/fork_count",
				),
				otel.RenameMetric(
					"system.processes.count",
					"processes/count_by_state",
					// change data type from int64 -> double
					otel.ToggleScalarDataType,
					otel.RenameLabel("status", "state"),
				),
				otel.RenameMetric(
					"system.paging.usage",
					"swap/bytes_used",
					// change data type from int64 -> double
					otel.ToggleScalarDataType,
				),
				otel.RenameMetric(
					"system.paging.utilization",
					"swap/percent_used",
				),
				// duplicate swap/percent_used -> pagefile/percent_used
				otel.DuplicateMetric(
					"swap/percent_used",
					"pagefile/percent_used",
					// take sum over device dimension, retaining only state
					otel.AggregateLabels("sum", "state"),
				),
				otel.RenameMetric(
					"system.paging.operations",
					"swap/io",
					// delete single-valued type dimension, retaining only direction
					otel.AggregateLabels("sum", "direction"),
					otel.RenameLabelValues("direction", map[string]string{
						"page_in":  "in",
						"page_out": "out",
					}),
				),
				otel.RenameMetric(
					"process.cpu.time",
					"processes/cpu_time",
					// scale from seconds to microseconds
					otel.ScaleValue(1000000),
					// change data type from double -> int64
					otel.ToggleScalarDataType,
					otel.AddLabel("process", "all"),
					// retain only user and syst state label values
					otel.DeleteLabelValue("state", "wait"),
					otel.RenameLabel("state", "user_or_syst"),
					otel.RenameLabelValues("user_or_syst", map[string]string{
						"system": "syst",
					}),
				),
				otel.RenameMetric(
					"process.disk.read_io", // as named after custom split logic
					"processes/disk/read_bytes_count",
					otel.AddLabel("process", "all"),
				),
				otel.RenameMetric(
					"process.disk.write_io", // as named after custom split logic
					"processes/disk/write_bytes_count",
					otel.AddLabel("process", "all"),
				),
				otel.RenameMetric(
					"process.memory.physical_usage",
					"processes/rss_usage",
					// change data type from int64 -> double
					otel.ToggleScalarDataType,
					otel.AddLabel("process", "all"),
				),
				otel.RenameMetric(
					"process.memory.virtual_usage",
					"processes/vm_usage",
					// change data type from int64 -> double
					otel.ToggleScalarDataType,
					otel.AddLabel("process", "all"),
				),
				otel.AddPrefix("agent.googleapis.com"),
			),
		},
	}}
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverHostmetrics{} })
}
