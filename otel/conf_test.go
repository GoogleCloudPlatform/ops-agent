package otel

import (
	"testing"

	"github.com/kylelemons/godebug/diff"
)

func TestHostMetrics(t *testing.T) {
	tests := []struct {
		hostmetrics               HostMetrics
		expectedHostMetricsConfig string
	}{
		{
			hostmetrics: HostMetrics{
				HostMetricsID:      "hostmetrics",
				CollectionInterval: "60s",
			},
			expectedHostMetricsConfig: `hostmetrics/hostmetrics:
    collection_interval: 60s
    scrapers:
      cpu:
      load:
      memory:
      disk:
      filesystem:
      network:
      paging:
      process:`,
		},
	}
	for _, tc := range tests {
		got, err := tc.hostmetrics.renderConfig()
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return
		}
		if diff := diff.Diff(tc.expectedHostMetricsConfig, got); diff != "" {
			t.Errorf("Tail %v: ran hostmetrics.renderConfig() returned unexpected diff (-want +got):\n%s", tc.hostmetrics, diff)
		}
	}
}

func TestHostMetricsErrors(t *testing.T) {
	tests := []struct {
		name        string
		hostmetrics HostMetrics
	}{
		{
			name: "empty collection interval",
			hostmetrics: HostMetrics{
				HostMetricsID: "hostmetrics",
				//CollectionInterval: "60s",
			},
		},
		{
			name: "invalid collection interval",
			hostmetrics: HostMetrics{
				HostMetricsID:      "hostmetrics",
				CollectionInterval: "60",
			},
		},
		{
			name: "collection interval too short",
			hostmetrics: HostMetrics{
				HostMetricsID:      "hostmetrics",
				CollectionInterval: "1s",
			},
		},
	}
	for _, tc := range tests {
		if _, err := tc.hostmetrics.renderConfig(); err == nil {
			t.Errorf("test %q: hostmetrics.renderConfig() succeeded, want error.", tc.name)
		}
	}

}

func TestIIS(t *testing.T) {
	tests := []struct {
		iis               IIS
		expectedIISConfig string
	}{
		{
			iis: IIS{
				IISID:              "iis",
				CollectionInterval: "60s",
			},
			expectedIISConfig: `windowsperfcounters/iis_iis:
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
	}
	for _, tc := range tests {
		got, err := tc.iis.renderConfig()
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return
		}
		if diff := diff.Diff(tc.expectedIISConfig, got); diff != "" {
			t.Errorf("Tail %v: ran iis.renderConfig() returned unexpected diff (-want +got):\n%s", tc.iis, diff)
		}
	}
}

func TestMSSQL(t *testing.T) {
	tests := []struct {
		mssql               MSSQL
		expectedMSSQLConfig string
	}{
		{
			mssql: MSSQL{
				MSSQLID:            "mssql",
				CollectionInterval: "60s",
			},
			expectedMSSQLConfig: `windowsperfcounters/mssql_mssql:
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
	}
	for _, tc := range tests {
		got, err := tc.mssql.renderConfig()
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return
		}
		if diff := diff.Diff(tc.expectedMSSQLConfig, got); diff != "" {
			t.Errorf("Tail %v: ran mssql.renderConfig() returned unexpected diff (-want +got):\n%s", tc.mssql, diff)
		}
	}
}

func TestStackdriver(t *testing.T) {
	tests := []struct {
		stackdriver               Stackdriver
		expectedStackdriverConfig string
	}{
		{
			stackdriver: Stackdriver{
				StackdriverID: "agent",
				UserAgent:     "$USERAGENT",
				Prefix:        "agent.googleapis.com/",
			},
			expectedStackdriverConfig: `googlecloud/agent:
    user_agent: $USERAGENT
    metric:
      prefix: agent.googleapis.com/`,
		},
	}
	for _, tc := range tests {
		got, err := tc.stackdriver.renderConfig()
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return
		}
		if diff := diff.Diff(tc.expectedStackdriverConfig, got); diff != "" {
			t.Errorf("Tail %v: ran stackdriver.renderConfig() returned unexpected diff (-want +got):\n%s", tc.stackdriver, diff)
		}
	}
}

func TestService(t *testing.T) {
	tests := []struct {
		service               Service
		expectedServiceConfig string
	}{
		{
			service: Service{
				ID:         "system",
				Processors: "[agentmetrics/system,filter/system,metricstransform/system,resourcedetection]",
				Receivers:  "[hostmetrics/hostmetrics]",
				Exporters:  "[googlecloud/google]",
			},
			expectedServiceConfig: `metrics/system:
      receivers:  [hostmetrics/hostmetrics]
      processors: [agentmetrics/system,filter/system,metricstransform/system,resourcedetection]
      exporters: [googlecloud/google]`,
		},
	}
	for _, tc := range tests {
		got, err := tc.service.renderConfig()
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return
		}
		if diff := diff.Diff(tc.expectedServiceConfig, got); diff != "" {
			t.Errorf("Tail %v: ran service.renderConfig() returned unexpected diff (-want +got):\n%s", tc.service, diff)
		}
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
				UserAgent:     "$USERAGENT",
				Prefix:        "agent.googleapis.com/",
			}},
			serviceList: []*Service{{
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
    # 1. converts up down sum types to gauges
    # 2. combines resource process metrics into metrics with processes as labels
    # 3. splits "disk.io" metrics into read & write metrics
    # 4. creates utilization metrics from usage metrics

  # filter out metrics not currently supported by cloud monitoring
  filter/system:
    metrics:
      exclude:
        match_type: strict
        metric_names:
          - system.network.dropped
          - system.filesystem.inodes.usage

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
          # take mean over cpu_number dimension, retaining only cpu_state
          - action: aggregate_labels
            label_set: [ cpu_state ]
            aggregation_type: mean
      # system.cpu.utilization -> cpu/utilization
      - metric_name: system.cpu.utilization
        action: update
        new_name: cpu/utilization
        operations:
          # change label cpu -> cpu_number
          - action: update_label
            label: cpu
            new_label: cpu_number
          # change label state -> cpu_state
          - action: update_label
            label: state
            new_label: cpu_state
          # take mean over cpu_number dimension, retaining only cpu_state
          - action: aggregate_labels
            label_set: [ cpu_state ]
            aggregation_type: mean
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
          # change data type from double -> int64
          - action: toggle_scalar_data_type
      # system.disk.operation_time -> disk/operation_time
      - metric_name: system.disk.operation_time
        action: update
        new_name: disk/operation_time
        operations:
          # change data type from double -> int64
          - action: toggle_scalar_data_type
      # system.disk.pending_operations -> disk/pending_operations
      - metric_name: system.disk.pending_operations
        action: update
        new_name: disk/pending_operations
        operations:
          # change data type from int64 -> double
          - action: toggle_scalar_data_type
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
            label_set: [ state ]
            aggregation_type: sum
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
      # process.cpu.time -> processes/cpu_time
      - metric_name: process.cpu.time
        action: update
        new_name: processes/cpu_time
        operations:
          # change data type from double -> int64
          - action: toggle_scalar_data_type
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
      # process.disk.write_io (as named after custom split logic) -> processes/disk/write_bytes_count
      - metric_name: process.disk.write_io
        action: update
        new_name: processes/disk/write_bytes_count
      # process.memory.physical_usage -> processes/rss_usage
      - metric_name: process.memory.physical_usage
        action: update
        new_name: processes/rss_usage
        operations:
          # change data type from int64 -> double
          - action: toggle_scalar_data_type
      # process.memory.virtual_usage -> processes/vm_usage
      - metric_name: process.memory.virtual_usage
        action: update
        new_name: processes/vm_usage
        operations:
          # change data type from int64 -> double
          - action: toggle_scalar_data_type

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
            new_value: $USERAGENT
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
    user_agent: Google-Cloud-Ops-Agent-Collector/latest (BuildDistro=build_distro;Platform=windows;ShortName=win_platform;ShortVersion=win_platform_version,gzip(gfe))
    metric:
      prefix: agent.googleapis.com/
  googlecloud/agent:
    user_agent: Google-Cloud-Ops-Agent-Collector/latest (BuildDistro=build_distro;Platform=windows;ShortName=win_platform;ShortVersion=win_platform_version,gzip(gfe))
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
			got, err := GenerateOtelConfig(tc.hostMetricsList, tc.mssqlList, tc.iisList, tc.stackdriverList, tc.serviceList, "Google-Cloud-Ops-Agent-Collector/latest (BuildDistro=build_distro;Platform=windows;ShortName=win_platform;ShortVersion=win_platform_version,gzip(gfe))")
			if err != nil {
				t.Errorf("got error: %v, want no error", err)
				return
			}
			if diff := diff.Diff(tc.want, got); diff != "" {
				t.Errorf("test %q: ran GenerateOtelConfig returned unexpected diff (-want +got):\n%s", tc.name, diff)
			}
		})
	}
}
