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

//Package otel provides data structures to represent and generate otel configuration.
package otel

import (
	"fmt"
	"strings"
	"text/template"
	"time"
)

var templateFunctions = template.FuncMap{
	"notEmpty":                   notEmpty,
	"validateCollectionInterval": validateCollectionInterval,
}

var confTemplate = template.Must(template.New("conf").Funcs(templateFunctions).Parse(
	`receivers:
  {{template "agentreceiver" .}}
{{- range .HostMetrics}}
  {{template "hostmetrics" .}}
{{- end}}
{{- range .MSSQL}}
  {{template "mssql" .}}
{{- end}}
{{- range .IIS}}
  {{template "iis" .}}
{{- end}}
processors:
  {{template "defaultprocessor" .}}
exporters:
{{- range .Stackdriver}}
  {{template "stackdriver" .}}
{{- end}}
extensions:
service:
  pipelines:
    {{template "agentservice" .}}
{{- range .Service}}
    {{template "service" .}}
{{- end}}
{{define "hostmetrics" -}}
  hostmetrics/{{.HostMetricsID}}:
    collection_interval: {{.CollectionInterval | validateCollectionInterval "hostmetrics"}}
    scrapers:
      cpu:
      load:
      memory:
      disk:
      filesystem:
      network:
      paging:
      process:
{{- end -}}

{{define "iis" -}}
windowsperfcounters/iis_{{.IISID}}:
    collection_interval: {{.CollectionInterval | validateCollectionInterval "iis"}}
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
{{- end -}}

{{define "mssql" -}}
windowsperfcounters/mssql_{{.MSSQLID}}:
    collection_interval: {{.CollectionInterval}}
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
{{- end -}}

{{define "stackdriver" -}}
  googlecloud/{{.StackdriverID}}:
    user_agent: {{.UserAgent}}
    metric:
      prefix: {{.Prefix}}
{{- end -}}

{{define "agentreceiver" -}}
  prometheus/agent:
    config:
      scrape_configs:
      - job_name: 'otel-collector'
        scrape_interval: 1m
        static_configs:
        - targets: ['0.0.0.0:8888']
{{- end -}}

{{define "agentservice" -}}
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
{{- end -}}

{{define "service" -}}
    metrics/{{.ID}}:
      receivers:  {{.Receivers}}
      processors: {{.Processors}}
      exporters: {{.Exporters}}
{{- end -}}

{{define "defaultprocessor" -}}
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
          - system.paging.faults

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
          # change label cpu -> cpu_number
          - action: update_label
            label: cpu
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
          # change data type from double -> int64
          - action: toggle_scalar_data_type
      # system.disk.weighted_io_time -> disk/weighted_io_time
      - metric_name: system.disk.weighted_io_time
        action: update
        new_name: disk/weighted_io_time
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
          # change data type from double -> int64
          - action: toggle_scalar_data_type
          - action: add_label
            new_label: process
            new_value: all
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
{{- end -}}`))

type emptyFieldErr struct {
	plugin string
	field  string
}

func (e emptyFieldErr) Error() string {
	return fmt.Sprintf("%q plugin should not have empty field: %q", e.plugin, e.field)
}

func notEmpty(plugin, field, value string) (string, error) {
	if value == "" {
		return "", emptyFieldErr{
			plugin: plugin,
			field:  field,
		}
	}
	return value, nil
}

func validateCollectionInterval(pluginName, collectionInterval string) (string, error) {
	if _, err := notEmpty(pluginName, "collection_interval", collectionInterval); err != nil {
		return "", err
	}
	t, err := time.ParseDuration(collectionInterval)
	if err != nil {
		return "", fmt.Errorf("parameter \"collection_interval\" in metrics receiver %q has invalid value %q that is not an interval (e.g. \"60s\"). Detailed error: %s", pluginName, collectionInterval, err)
	}
	interval := t.Seconds()
	if interval < 10 {
		return "", fmt.Errorf("parameter \"collection_interval\" in metrics receiver %q has invalid value %vs that is below the minimum threshold of \"10s\".", pluginName, interval)
	}
	return collectionInterval, nil
}

type MSSQL struct {
	MSSQLID            string
	CollectionInterval string
}

type IIS struct {
	IISID              string
	CollectionInterval string
}

type HostMetrics struct {
	HostMetricsID      string
	CollectionInterval string
}

type Service struct {
	ID         string
	Processors string
	Receivers  string
	Exporters  string
}

type Stackdriver struct {
	StackdriverID string
	UserAgent     string
	Prefix        string
}

type Config struct {
	HostMetrics []*HostMetrics
	MSSQL       []*MSSQL
	IIS         []*IIS
	Stackdriver []*Stackdriver
	Service     []*Service

	UserAgent string
	Windows   bool
}

func (c Config) Generate() (string, error) {
	c.Stackdriver = append(c.Stackdriver, &Stackdriver{
		StackdriverID: "agent",
		Prefix:        "agent.googleapis.com/",
		UserAgent:     c.UserAgent,
	})

	for _, s := range c.Stackdriver {
		s.UserAgent = c.UserAgent
	}

	var configBuilder strings.Builder
	if err := confTemplate.Execute(&configBuilder, c); err != nil {
		return "", err
	}
	return configBuilder.String(), nil
}
