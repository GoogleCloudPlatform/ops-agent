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

type MetricsReceiverHostmetrics struct {
	ConfigComponent `yaml:",inline"`

	MetricsReceiverShared `yaml:",inline"`
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
					"process":    struct{}{},
					"processes":  struct{}{},
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
			metricsFilter(
				"exclude",
				"strict",
				// Temporarily exclude system.cpu.time (cpu/usage_time)
				"system.cpu.time",
				"system.network.dropped",
				"system.filesystem.inodes.usage",
				"system.paging.faults",
				"system.disk.operation_time",
				"system.processes.count",
			),
			metricsTransform(
				renameMetric(
					"system.cpu.time",
					"cpu/usage_time",
					// change data type from double -> int64
					toggleScalarDataType,
					renameLabel("cpu", "cpu_number"),
					renameLabel("state", "cpu_state"),
				),
				renameMetric(
					"system.cpu.utilization",
					"cpu/utilization",
					// take avg over cpu dimension, retaining only state label
					aggregateLabels(
						"mean",
						"state",
						"blank",
					),
					// add blank cpu_number label
					renameLabel("blank", "cpu_number"),
					// change label state -> cpu_state
					renameLabel("state", "cpu_state"),
				),
				renameMetric(
					"system.cpu.load_average.1m",
					"cpu/load_1m",
				),
				renameMetric(
					"system.cpu.load_average.5m",
					"cpu/load_5m",
				),
				renameMetric(
					"system.cpu.load_average.15m",
					"cpu/load_15m",
				),
				renameMetric(
					"system.disk.read_io", // as named after custom split logic
					"disk/read_bytes_count",
				),
				renameMetric(
					"system.disk.write_io", // as named after custom split logic
					"disk/write_bytes_count",
				),
				renameMetric(
					"system.disk.operations",
					"disk/operation_count",
				),
				renameMetric(
					"system.disk.io_time",
					"disk/io_time",
					// convert s to ms
					scaleValue(1000),
					// change data type from double -> int64
					toggleScalarDataType,
				),
				renameMetric(
					"system.disk.weighted_io_time",
					"disk/weighted_io_time",
					// convert s to ms
					scaleValue(1000),
					// change data type from double -> int64
					toggleScalarDataType,
				),
				renameMetric(
					"system.disk.average_operation_time",
					"disk/operation_time",
					// convert s to ms
					scaleValue(1000),
					// change data type from double -> int64
					toggleScalarDataType,
				),
				renameMetric(
					"system.disk.pending_operations",
					"disk/pending_operations",
					// change data type from int64 -> double
					toggleScalarDataType,
				),
				renameMetric(
					"system.disk.merged",
					"disk/merged_operations",
				),
				renameMetric(
					"system.filesystem.usage",
					"disk/bytes_used",
					// change data type from int64 -> double
					toggleScalarDataType,
					// take sum over mode, mountpoint & type dimensions, retaining only device & state
					aggregateLabels("sum", "device", "state"),
				),
				renameMetric(
					"system.filesystem.utilization",
					"disk/percent_used",
					aggregateLabels("sum", "device", "state"),
				),
				renameMetric(
					"system.memory.usage",
					"memory/bytes_used",
					// change data type from int64 -> double
					toggleScalarDataType,
					// aggregate state label values: slab_reclaimable & slab_unreclaimable -> slab (note this is not currently supported)
					aggregateLabelValues("sum", "state", "slab", "slab_reclaimable", "slab_unreclaimable"),
				),
				renameMetric(
					"system.memory.utilization",
					"memory/percent_used",
					// sum state label values: slab = slab_reclaimable + slab_unreclaimable
					aggregateLabelValues("sum", "state", "slab", "slab_reclaimable", "slab_unreclaimable"),
				),
				renameMetric(
					"system.network.io",
					"interface/traffic",
					renameLabel("interface", "device"),
					renameLabelValues("direction", map[string]string{
						"receive":  "rx",
						"transmit": "tx",
					}),
				),
				renameMetric(
					"system.network.errors",
					"interface/errors",
					renameLabel("interface", "device"),
					renameLabelValues("direction", map[string]string{
						"receive":  "rx",
						"transmit": "tx",
					}),
				),
				renameMetric(
					"system.network.packets",
					"interface/packets",
					renameLabel("interface", "device"),
					renameLabelValues("direction", map[string]string{
						"receive":  "rx",
						"transmit": "tx",
					}),
				),
				renameMetric(
					"system.network.connections",
					"network/tcp_connections",
					// change data type from int64 -> double
					toggleScalarDataType,
					// remove udp data
					deleteLabelValue("protocol", "udp"),
					renameLabel("state", "tcp_state"),
					// remove protocol label
					aggregateLabels("sum", "tcp_state"),
					addLabel("port", "all"),
				),
				renameMetric(
					"system.processes.created",
					"processes/fork_count",
				),
				renameMetric(
					"system.paging.usage",
					"swap/bytes_used",
					// change data type from int64 -> double
					toggleScalarDataType,
				),
				renameMetric(
					"system.paging.utilization",
					"swap/percent_used",
				),
				// duplicate swap/percent_used -> pagefile/percent_used
				duplicateMetric(
					"swap/percent_used",
					"pagefile/percent_used",
					// take sum over device dimension, retaining only state
					aggregateLabels("sum", "state"),
				),
				renameMetric(
					"system.paging.operations",
					"swap/io",
					// delete single-valued type dimension, retaining only direction
					aggregateLabels("sum", "direction"),
					renameLabelValues("direction", map[string]string{
						"page_in":  "in",
						"page_out": "out",
					}),
				),
				renameMetric(
					"process.cpu.time",
					"processes/cpu_time",
					// scale from seconds to microseconds
					scaleValue(1000000),
					// change data type from double -> int64
					toggleScalarDataType,
					addLabel("process", "all"),
					// retain only user and syst state label values
					deleteLabelValue("state", "wait"),
					renameLabel("state", "user_or_syst"),
					renameLabelValues("user_or_syst", map[string]string{
						"system": "syst",
					}),
				),
				renameMetric(
					"process.disk.read_io", // as named after custom split logic
					"processes/disk/read_bytes_count",
					addLabel("process", "all"),
				),
				renameMetric(
					"process.disk.write_io", // as named after custom split logic
					"processes/disk/write_bytes_count",
					addLabel("process", "all"),
				),
				renameMetric(
					"process.memory.physical_usage",
					"processes/rss_usage",
					// change data type from int64 -> double
					toggleScalarDataType,
					addLabel("process", "all"),
				),
				renameMetric(
					"process.memory.virtual_usage",
					"processes/vm_usage",
					// change data type from int64 -> double
					toggleScalarDataType,
					addLabel("process", "all"),
				),
			),
		},
	}}
}

func init() {
	metricsReceiverTypes.registerType(func() component { return &MetricsReceiverHostmetrics{} })
}
