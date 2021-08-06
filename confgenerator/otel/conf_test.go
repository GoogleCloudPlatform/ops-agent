// Copyright 2020 Google LLC
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

package otel

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kylelemons/godebug/diff"
)

func TestSection(t *testing.T) {
	tests := []struct {
		name    string
		section interface{}
		want    string
	}{
		{
			section: HostMetrics{
				HostMetricsID:      "hostmetrics",
				CollectionInterval: "60s",
			},
			want: `hostmetrics/hostmetrics:
    collection_interval: 60s
    scrapers:
      cpu:
      load:
      memory:
      disk:
      filesystem:
      network:
      paging:
      process:
      processes:`,
		},
		{
			section: IIS{
				IISID:              "iis",
				CollectionInterval: "60s",
			},
			want: `windowsperfcounters/iis_iis:
    collection_interval: 60s
    perfcounters:
      - object: Web Service
        instances: _Total
        counters:
          - Current Connections
          - Total Bytes Received
          - Total Bytes Sent
          - Total Connection Attempts (all instances)
          - Total Delete Requests
          - Total Get Requests
          - Total Head Requests
          - Total Options Requests
          - Total Post Requests
          - Total Put Requests
          - Total Trace Requests`,
		},
		{
			section: MSSQL{
				MSSQLID:            "mssql",
				CollectionInterval: "60s",
			},
			want: `windowsperfcounters/mssql_mssql:
    collection_interval: 60s
    perfcounters:
      - object: SQLServer:General Statistics
        instances: _Total
        counters:
          - User Connections
      - object: SQLServer:Databases
        instances: _Total
        counters:
          - Transactions/sec
          - Write Transactions/sec`,
		},
		{
			section: Stackdriver{
				StackdriverID: "agent",
				UserAgent:     "$USERAGENT",
				Prefix:        "agent.googleapis.com/",
			},
			want: `googlecloud/agent:
    user_agent: $USERAGENT
    metric:
      prefix: agent.googleapis.com/`,
		},
		{
			section: Service{
				ID:         "system",
				Processors: "[agentmetrics/system,filter/system,metricstransform/system,resourcedetection]",
				Receivers:  "[hostmetrics/hostmetrics]",
				Exporters:  "[googlecloud/google]",
			},
			want: `metrics/system:
      receivers:  [hostmetrics/hostmetrics]
      processors: [agentmetrics/system,filter/system,metricstransform/system,resourcedetection]
      exporters: [googlecloud/google]`,
		},
	}
	for _, tc := range tests {
		typeObj := reflect.ValueOf(tc.section).Type()
		name := typeObj.Name()
		if tc.name != "" {
			name = name + "/" + tc.name
		}
		t.Run(name, func(t *testing.T) {
			var b strings.Builder
			err := confTemplate.ExecuteTemplate(&b, strings.ToLower(typeObj.Name()), tc.section)
			got := b.String()
			if tc.want != "" {
				if err != nil {
					t.Errorf("got error: %v, want no error", err)
					return
				}
				if diff := diff.Diff(tc.want, got); diff != "" {
					t.Errorf("service.renderConfig() returned unexpected diff (-want +got):\n%s", diff)
				}
			} else {
				if err == nil {
					t.Errorf("rendering configuration succeeded, want error.")
				}
			}
		})
	}
}

func TestGenerateOtelConfig(t *testing.T) {
	tests := []struct {
		name            string
		hostMetricsList []*HostMetrics
		mssqlList       []*MSSQL
		iisList         []*IIS
		stackdriverList []*Stackdriver
		serviceList     []*Service
		want            string
	}{
		{
			name: "default system metrics config",
			hostMetricsList: []*HostMetrics{{
				HostMetricsID:      "hostmetrics",
				CollectionInterval: "60s",
			}},
			mssqlList: []*MSSQL{{
				MSSQLID:            "mssql",
				CollectionInterval: "60s",
			}},
			iisList: []*IIS{{
				IISID:              "iis",
				CollectionInterval: "60s",
			}},
			stackdriverList: []*Stackdriver{{
				StackdriverID: "google",
				UserAgent:     "$IGNORED_VALUE",
				Prefix:        "agent.googleapis.com/",
			}},
			serviceList: []*Service{
				{
					ID:         "system",
					Receivers:  "[hostmetrics/hostmetrics]",
					Processors: "[agentmetrics/system,filter/system,metricstransform/system,resourcedetection]",
					Exporters:  "[googlecloud/google]",
				},
				{
					ID:         "mssql",
					Receivers:  "[windowsperfcounters/mssql_mssql]",
					Processors: "[metricstransform/mssql,resourcedetection]",
					Exporters:  "[googlecloud/google]",
				},
				{ID: "iis",
					Receivers:  "[windowsperfcounters/iis_iis]",
					Processors: "[metricstransform/iis,resourcedetection]",
					Exporters:  "[googlecloud/google]",
				},
			},
			want: `receivers:
  prometheus/agent:
    config:
      scrape_configs:
      - job_name: 'otel-collector'
        scrape_interval: 1m
        static_configs:
        - targets: ['0.0.0.0:8888']
  hostmetrics/hostmetrics:
    collection_interval: 60s
    scrapers:
      cpu:
      load:
      memory:
      disk:
      filesystem:
      network:
      paging:
      process:
      processes:
  windowsperfcounters/mssql_mssql:
    collection_interval: 60s
    perfcounters:
      - object: SQLServer:General Statistics
        instances: _Total
        counters:
          - User Connections
      - object: SQLServer:Databases
        instances: _Total
        counters:
          - Transactions/sec
          - Write Transactions/sec
  windowsperfcounters/iis_iis:
    collection_interval: 60s
    perfcounters:
      - object: Web Service
        instances: _Total
        counters:
          - Current Connections
          - Total Bytes Received
          - Total Bytes Sent
          - Total Connection Attempts (all instances)
          - Total Delete Requests
          - Total Get Requests
          - Total Head Requests
          - Total Options Requests
          - Total Post Requests
          - Total Put Requests
          - Total Trace Requests
processors:
  resourcedetection:
    detectors: [gce]

  # perform custom transformations that aren't supported by the metricstransform processor
  agentmetrics/system:
    # https://github.com/GoogleCloudPlatform/opentelemetry-operations-collector/blob/master/processor/agentmetricsprocessor/agentmetricsprocessor.go#L58
    blank_label_metrics:
    - system.cpu.utilization

  # filter out metrics not currently supported by cloud monitoring
  filter/system:
    metrics:
      exclude:
        match_type: strict
        metric_names:
          # Temporarily exclude system.cpu.time (cpu/usage_time)
          - system.cpu.time
          - system.network.dropped
          - system.filesystem.inodes.usage
          - system.paging.faults
          - system.disk.operation_time
          - system.processes.count

  # convert from opentelemetry metric formats to cloud monitoring formats
  metricstransform/system:
    transforms:
      # system.cpu.time -> cpu/usage_time
      - metric_name: system.cpu.time
        action: update
        new_name: cpu/usage_time
        operations:
          # change data type from double -> int64
          - action: toggle_scalar_data_type
          # change label cpu -> cpu_number
          - action: update_label
            label: cpu
            new_label: cpu_number
          # change label state -> cpu_state
          - action: update_label
            label: state
            new_label: cpu_state
      # system.cpu.utilization -> cpu/utilization
      - metric_name: system.cpu.utilization
        action: update
        new_name: cpu/utilization
        operations:
          # take avg over cpu dimension, retaining only state label
          - action: aggregate_labels
            label_set: [state, blank]
            aggregation_type: mean
          # add blank cpu_number label
          - action: update_label
            label: blank
            new_label: cpu_number
          # change label state -> cpu_state
          - action: update_label
            label: state
            new_label: cpu_state
      # system.cpu.load_average.1m -> cpu/load_1m
      - metric_name: system.cpu.load_average.1m
        action: update
        new_name: cpu/load_1m
      # system.cpu.load_average.5m -> cpu/load_5m
      - metric_name: system.cpu.load_average.5m
        action: update
        new_name: cpu/load_5m
      # system.cpu.load_average.15m -> cpu/load_15m
      - metric_name: system.cpu.load_average.15m
        action: update
        new_name: cpu/load_15m
      # system.disk.read_io (as named after custom split logic) -> disk/read_bytes_count
      - metric_name: system.disk.read_io
        action: update
        new_name: disk/read_bytes_count
      # system.disk.write_io (as named after custom split logic) -> processes/write_bytes_count
      - metric_name: system.disk.write_io
        action: update
        new_name: disk/write_bytes_count
      # system.disk.operations -> disk/operation_count
      - metric_name: system.disk.operations
        action: update
        new_name: disk/operation_count
      # system.disk.io_time -> disk/io_time
      - metric_name: system.disk.io_time
        action: update
        new_name: disk/io_time
        operations:
          # convert s to ms
          - action: experimental_scale_value
            experimental_scale: 1000
          # change data type from double -> int64
          - action: toggle_scalar_data_type
      # system.disk.weighted_io_time -> disk/weighted_io_time
      - metric_name: system.disk.weighted_io_time
        action: update
        new_name: disk/weighted_io_time
        operations:
          # convert s to ms
          - action: experimental_scale_value
            experimental_scale: 1000
          # change data type from double -> int64
          - action: toggle_scalar_data_type
      # system.disk.average_operation_time -> disk/operation_time
      - metric_name: system.disk.average_operation_time
        action: update
        new_name: disk/operation_time
        operations:
          # convert s to ms
          - action: experimental_scale_value
            experimental_scale: 1000
          # change data type from double -> int64
          - action: toggle_scalar_data_type
      # system.disk.pending_operations -> disk/pending_operations
      - metric_name: system.disk.pending_operations
        action: update
        new_name: disk/pending_operations
        operations:
          # change data type from int64 -> double
          - action: toggle_scalar_data_type
      # system.disk.merged -> disk/merged_operations
      - metric_name: system.disk.merged
        action: update
        new_name: disk/merged_operations
      # system.filesystem.usage -> disk/bytes_used
      - metric_name: system.filesystem.usage
        action: update
        new_name: disk/bytes_used
        operations:
          # change data type from int64 -> double
          - action: toggle_scalar_data_type
          # take sum over mode, mountpoint & type dimensions, retaining only device & state
          - action: aggregate_labels
            label_set: [ device, state ]
            aggregation_type: sum
      # system.filesystem.utilization -> disk/percent_used
      - metric_name: system.filesystem.utilization
        action: update
        new_name: disk/percent_used
        operations:
          # take sum over mode, mountpoint & type dimensions, retaining only device & state
          - action: aggregate_labels
            label_set: [ device, state ]
            aggregation_type: sum
      # system.memory.usage -> memory/bytes_used
      - metric_name: system.memory.usage
        action: update
        new_name: memory/bytes_used
        operations:
          # change data type from int64 -> double
          - action: toggle_scalar_data_type
          # aggregate state label values: slab_reclaimable & slab_unreclaimable -> slab (note this is not currently supported)
          - action: aggregate_label_values
            label: state
            aggregated_values: [slab_reclaimable, slab_unreclaimable]
            new_value: slab
            aggregation_type: sum
      # system.memory.utilization -> memory/percent_used
      - metric_name: system.memory.utilization
        action: update
        new_name: memory/percent_used
        operations:
          # sum state label values: slab = slab_reclaimable + slab_unreclaimable
          - action: aggregate_label_values
            label: state
            aggregated_values: [slab_reclaimable, slab_unreclaimable]
            new_value: slab
            aggregation_type: sum
      # system.network.io -> interface/traffic
      - metric_name: system.network.io
        action: update
        new_name: interface/traffic
        operations:
          # change label interface -> device
          - action: update_label
            label: interface
            new_label: device
          # change direction label values receive -> rx
          - action: update_label
            label: direction
            value_actions:
              # receive -> rx
              - value: receive
                new_value: rx
              # transmit -> tx
              - value: transmit
                new_value: tx
      # system.network.errors -> interface/errors
      - metric_name: system.network.errors
        action: update
        new_name: interface/errors
        operations:
          # change label interface -> device
          - action: update_label
            label: interface
            new_label: device
          # change direction label values receive -> rx
          - action: update_label
            label: direction
            value_actions:
              # receive -> rx
              - value: receive
                new_value: rx
              # transmit -> tx
              - value: transmit
                new_value: tx
      # system.network.packets -> interface/packets
      - metric_name: system.network.packets
        action: update
        new_name: interface/packets
        operations:
          # change label interface -> device
          - action: update_label
            label: interface
            new_label: device
          # change direction label values receive -> rx
          - action: update_label
            label: direction
            value_actions:
              # receive -> rx
              - value: receive
                new_value: rx
              # transmit -> tx
              - value: transmit
                new_value: tx
      # system.network.connections -> network/tcp_connections
      - metric_name: system.network.connections
        action: update
        new_name: network/tcp_connections
        operations:
          # change data type from int64 -> double
          - action: toggle_scalar_data_type
          # remove udp data
          - action: delete_label_value
            label: protocol
            label_value: udp
          # change label state -> tcp_state
          - action: update_label
            label: state
            new_label: tcp_state
          # remove protocol label
          - action: aggregate_labels
            label_set: [ tcp_state ]
            aggregation_type: sum
          - action: add_label
            new_label: port
            new_value: all
      # system.processes.created -> processes/fork_count
      - metric_name: system.processes.created
        action: update
        new_name: processes/fork_count
      # system.paging.usage -> swap/bytes_used
      - metric_name: system.paging.usage
        action: update
        new_name: swap/bytes_used
        operations:
          # change data type from int64 -> double
          - action: toggle_scalar_data_type
      # system.paging.utilization -> swap/percent_used
      - metric_name: system.paging.utilization
        action: update
        new_name: swap/percent_used
      # duplicate swap/percent_used -> pagefile/percent_used
      - metric_name: swap/percent_used
        action: insert
        new_name: pagefile/percent_used
        operations:
          # take sum over device dimension, retaining only state
          - action: aggregate_labels
            label_set: [ state ]
            aggregation_type: sum
      # system.paging.operations -> swap/io
      - metric_name: system.paging.operations
        action: update
        new_name: swap/io
        operations:
          # delete single-valued type dimension, retaining only direction
          - action: aggregate_labels
            label_set: [ direction ]
            aggregation_type: sum
          - action: update_label
            label: direction
            # change label value page_in -> in, page_out -> out
            value_actions:
              - value: page_in
                new_value: in
              - value: page_out
                new_value: out
      # process.cpu.time -> processes/cpu_time
      - metric_name: process.cpu.time
        action: update
        new_name: processes/cpu_time
        operations:
          # scale from seconds to microseconds
          - action: experimental_scale_value
            experimental_scale: 1000000
          # change data type from double -> int64
          - action: toggle_scalar_data_type
          - action: add_label
            new_label: process
            new_value: all
          # retain only user and syst state label values
          - action: delete_label_value
            label: state
            label_value: wait
          # change label state -> user_or_syst
          - action: update_label
            label: state
            new_label: user_or_syst
            # change label value system -> syst
            value_actions:
              - value: system
                new_value: syst
      # process.disk.read_io (as named after custom split logic) -> processes/disk/read_bytes_count
      - metric_name: process.disk.read_io
        action: update
        new_name: processes/disk/read_bytes_count
        operations:
          - action: add_label
            new_label: process
            new_value: all
      # process.disk.write_io (as named after custom split logic) -> processes/disk/write_bytes_count
      - metric_name: process.disk.write_io
        action: update
        new_name: processes/disk/write_bytes_count
        operations:
          - action: add_label
            new_label: process
            new_value: all
      # process.memory.physical_usage -> processes/rss_usage
      - metric_name: process.memory.physical_usage
        action: update
        new_name: processes/rss_usage
        operations:
          # change data type from int64 -> double
          - action: toggle_scalar_data_type
          - action: add_label
            new_label: process
            new_value: all
      # process.memory.virtual_usage -> processes/vm_usage
      - metric_name: process.memory.virtual_usage
        action: update
        new_name: processes/vm_usage
        operations:
          # change data type from int64 -> double
          - action: toggle_scalar_data_type
          - action: add_label
            new_label: process
            new_value: all

  # filter to include only agent metrics supported by cloud monitoring
  filter/agent:
    metrics:
      include:
        match_type: strict
        metric_names:
          - otelcol_process_uptime
          - otelcol_process_memory_rss
          - otelcol_grpc_io_client_completed_rpcs
          - otelcol_googlecloudmonitoring_point_count

  # convert from windows perf counter formats to cloud monitoring formats
  metricstransform/iis:
    transforms:
      - include: \Web Service(_Total)\Current Connections
        action: update
        new_name: iis/current_connections
      - include: ^\\Web Service\(_Total\)\\Total Bytes (?P<direction>.*)$
        match_type: regexp
        action: combine
        new_name: iis/network/transferred_bytes_count
        submatch_case: lower
      - include: \Web Service(_Total)\Total Connection Attempts (all instances)
        action: update
        new_name: iis/new_connection_count
      - include: ^\\Web Service\(_Total\)\\Total (?P<http_method>.*) Requests$
        match_type: regexp
        action: combine
        new_name: iis/request_count
        submatch_case: lower

  # convert from windows perf counter formats to cloud monitoring formats
  metricstransform/mssql:
    transforms:
      - include: \SQLServer:General Statistics(_Total)\User Connections
        action: update
        new_name: mssql/connections/user
      - include: \SQLServer:Databases(_Total)\Transactions/sec
        action: update
        new_name: mssql/transaction_rate
      - include: \SQLServer:Databases(_Total)\Write Transactions/sec
        action: update
        new_name: mssql/write_transaction_rate

  # convert from opentelemetry metric formats to cloud monitoring formats
  metricstransform/agent:
    transforms:
      # otelcol_process_uptime -> agent/uptime
      - metric_name: otelcol_process_uptime
        action: update
        new_name: agent/uptime
        operations:
          # change data type from double -> int64
          - action: toggle_scalar_data_type
          # add version label
          - action: add_label
            new_label: version
            new_value: google-cloud-ops-agent-metrics/latest-build_distro
      # otelcol_process_memory_rss -> agent/memory_usage
      - metric_name: otelcol_process_memory_rss
        action: update
        new_name: agent/memory_usage
      # otelcol_grpc_io_client_completed_rpcs -> agent/api_request_count
      - metric_name: otelcol_grpc_io_client_completed_rpcs
        action: update
        new_name: agent/api_request_count
        operations:
          # change data type from double -> int64
          - action: toggle_scalar_data_type
        # TODO: below is proposed new configuration for the metrics transform processor
          # ignore any non "google.monitoring" RPCs (note there won't be any other RPCs for now)
        # - action: select_label_values
        #   label: grpc_client_method
        #   value_regexp: ^google\.monitoring
          # change label grpc_client_status -> state
          - action: update_label
            label: grpc_client_status
            new_label: state
          # delete grpc_client_method dimension, retaining only state
          - action: aggregate_labels
            label_set: [ state ]
            aggregation_type: sum
      # otelcol_googlecloudmonitoring_point_count -> agent/monitoring/point_count
      - metric_name: otelcol_googlecloudmonitoring_point_count
        action: update
        new_name: agent/monitoring/point_count
        operations:
          # change data type from double -> int64
          - action: toggle_scalar_data_type
exporters:
  googlecloud/google:
    user_agent: Google-Cloud-Ops-Agent-Metrics/latest (BuildDistro=build_distro;Platform=windows;ShortName=win_platform;ShortVersion=win_platform_version)
    metric:
      prefix: agent.googleapis.com/
  googlecloud/agent:
    user_agent: Google-Cloud-Ops-Agent-Metrics/latest (BuildDistro=build_distro;Platform=windows;ShortName=win_platform;ShortVersion=win_platform_version)
    metric:
      prefix: agent.googleapis.com/
extensions:
service:
  pipelines:
    # reports agent self-observability metrics to cloud monitoring
    metrics/agent:
      receivers:
        - prometheus/agent
      processors:
        - filter/agent
        - metricstransform/agent
        - resourcedetection
      exporters:
        - googlecloud/agent
    metrics/system:
      receivers:  [hostmetrics/hostmetrics]
      processors: [agentmetrics/system,filter/system,metricstransform/system,resourcedetection]
      exporters: [googlecloud/google]
    metrics/mssql:
      receivers:  [windowsperfcounters/mssql_mssql]
      processors: [metricstransform/mssql,resourcedetection]
      exporters: [googlecloud/google]
    metrics/iis:
      receivers:  [windowsperfcounters/iis_iis]
      processors: [metricstransform/iis,resourcedetection]
      exporters: [googlecloud/google]
`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Config{
				HostMetrics: tc.hostMetricsList,
				MSSQL:       tc.mssqlList,
				IIS:         tc.iisList,
				Stackdriver: tc.stackdriverList,
				Service:     tc.serviceList,

				UserAgent: "Google-Cloud-Ops-Agent-Metrics/latest (BuildDistro=build_distro;Platform=windows;ShortName=win_platform;ShortVersion=win_platform_version)",
				Version:   "google-cloud-ops-agent-metrics/latest-build_distro",
				Windows:   true,
			}.Generate()
			if err != nil {
				t.Errorf("got error: %v, want no error", err)
				return
			}
			if diff := diff.Diff(got, tc.want); diff != "" {
				t.Errorf("test %q: ran GenerateOtelConfig returned unexpected diff (-got +want):\n%s", tc.name, diff)
			}
		})
	}
}
