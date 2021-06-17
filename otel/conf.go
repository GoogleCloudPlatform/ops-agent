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

const (
	confTemplate = `receivers:
  {{range .ReceiversConfigSection -}}
  {{.}}
  {{end}}
processors:
  {{range .ProcessorsConfigSection -}}
  {{.}}
  {{end}}
exporters:
  {{range .ExportersConfigSection -}}
  {{.}}
  {{end}}
extensions:
  {{range .ExtensionsConfigSection -}}
  {{.}}
  {{end}}
service:
  pipelines:
    {{range .ServiceConfigSection -}}
    {{.}}
    {{end}}`

	hostmetricsReceiverConf = `hostmetrics/{{.HostMetricsID}}:
    collection_interval: {{.CollectionInterval}}
    scrapers:
      cpu:
      load:
      memory:
      disk:
      filesystem:
      network:
      swap:
      process:`

	iisReceiverConf = `windowsperfcounters/iis_{{.IISID}}:
    collection_interval: {{.CollectionInterval}}
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
          - Total Trace Requests`

	mssqlReceiverConf = `windowsperfcounters/mssql_{{.MSSQLID}}:
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
          - Write Transactions/sec`

	stackdriverExporterConf = `stackdriver/{{.StackdriverID}}:
    user_agent: {{.UserAgent}}
    metric:
      prefix: {{.Prefix}}`

	agentReceiverConf = `prometheus/agent:
    config:
      scrape_configs:
      - job_name: 'otel-collector'
        scrape_interval: 1m
        static_configs:
        - targets: ['0.0.0.0:8888']`

	agentServiceConf = `# reports agent self-observability metrics to cloud monitoring
    metrics/agent:
      receivers:
        - prometheus/agent
      processors:
        - filter/agent
        - metricstransform/agent
        - resourcedetection
      exporters:
        - stackdriver/agent`

	serviceConf = `metrics/{{.ID}}:
      receivers:  {{.Receivers}}
      processors: {{.Processors}}
      exporters: {{.Exporters}}`

	processorsConf = `resourcedetection:
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
          - system.network.dropped_packets

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
      # system.disk.ops -> disk/operation_count
      - metric_name: system.disk.ops
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
      # system.network.tcp_connections -> network/tcp_connections
      - metric_name: system.network.tcp_connections
        action: update
        new_name: network/tcp_connections
        operations:
          # change data type from int64 -> double
          - action: toggle_scalar_data_type
          # change label state -> tcp_state
          - action: update_label
            label: state
            new_label: tcp_state
      # system.swap.usage -> swap/bytes_used
      - metric_name: system.swap.usage
        action: update
        new_name: swap/bytes_used
        operations:
          # change data type from int64 -> double
          - action: toggle_scalar_data_type
      # system.swap.utilization -> swap/percent_used
      - metric_name: system.swap.utilization
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
      # system.swap.paging_ops -> swap/io
      - metric_name: system.swap.paging_ops
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
            new_value: {{.Version}}
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
          - action: toggle_scalar_data_type`
)

type configSections struct {
	ReceiversConfigSection  []string
	ProcessorsConfigSection []string
	ExportersConfigSection  []string
	ExtensionsConfigSection []string
	ServiceConfigSection    []string
}

type emptyFieldErr struct {
	plugin string
	field  string
}

func (e emptyFieldErr) Error() string {
	return fmt.Sprintf("%q plugin should not have empty field: %q", e.plugin, e.field)
}

func validateCollectionInterval(collectionInterval string, pluginName string) (bool, error) {
	t, err := time.ParseDuration(collectionInterval)
	if err != nil {
		return false, fmt.Errorf("parameter \"collection_interval\" in metrics receiver %q has invalid value %q that is not an interval (e.g. \"60s\"). Detailed error: %s", pluginName, collectionInterval, err)
	}
	interval := t.Seconds()
	if interval < 10 {
		return false, fmt.Errorf("parameter \"collection_interval\" in metrics receiver %q has invalid value %vs that is below the minimum threshold of \"10s\".", pluginName, interval)
	}
	return true, nil
}

type MSSQL struct {
	MSSQLID            string
	CollectionInterval string
}

var mssqlTemplate = template.Must(template.New("mssql").Parse(mssqlReceiverConf))

func (m MSSQL) renderConfig() (string, error) {
	if m.CollectionInterval == "" {
		return "", emptyFieldErr{
			plugin: "mssql",
			field:  "collection_interval",
		}
	}
	if v, err := validateCollectionInterval(m.CollectionInterval, m.MSSQLID); !v {
		return "", err
	}
	var renderedMSSQLConfig strings.Builder
	if err := mssqlTemplate.Execute(&renderedMSSQLConfig, m); err != nil {
		return "", err
	}
	return renderedMSSQLConfig.String(), nil
}

type IIS struct {
	IISID              string
	CollectionInterval string
}

var iisTemplate = template.Must(template.New("iis").Parse(iisReceiverConf))

func (i IIS) renderConfig() (string, error) {
	if i.CollectionInterval == "" {
		return "", emptyFieldErr{
			plugin: "iis",
			field:  "collection_interval",
		}
	}
	if v, err := validateCollectionInterval(i.CollectionInterval, i.IISID); !v {
		return "", err
	}
	var renderedIISConfig strings.Builder
	if err := iisTemplate.Execute(&renderedIISConfig, i); err != nil {
		return "", err
	}
	return renderedIISConfig.String(), nil
}

type HostMetrics struct {
	HostMetricsID      string
	CollectionInterval string
}

var hostMetricsTemplate = template.Must(template.New("hostmetrics").Parse(hostmetricsReceiverConf))

func (h HostMetrics) renderConfig() (string, error) {
	if h.CollectionInterval == "" {
		return "", emptyFieldErr{
			plugin: "hostmetrics",
			field:  "collection_interval",
		}
	}
	if v, err := validateCollectionInterval(h.CollectionInterval, h.HostMetricsID); !v {
		return "", err
	}
	var renderedHostMetricsConfig strings.Builder
	if err := hostMetricsTemplate.Execute(&renderedHostMetricsConfig, h); err != nil {
		return "", err
	}
	return renderedHostMetricsConfig.String(), nil
}

type Service struct {
	ID         string
	Processors string
	Receivers  string
	Exporters  string
}

var serviceTemplate = template.Must(template.New("service").Parse(serviceConf))

func (s Service) renderConfig() (string, error) {
	var renderedServiceConfig strings.Builder
	if err := serviceTemplate.Execute(&renderedServiceConfig, s); err != nil {
		return "", err
	}
	return renderedServiceConfig.String(), nil
}

type Processors struct {
	Version string
}

var processorsTemplate = template.Must(template.New("processors").Parse(processorsConf))

func (p Processors) renderConfig() (string, error) {
	var renderedConfig strings.Builder
	if err := processorsTemplate.Execute(&renderedConfig, p); err != nil {
		return "", err
	}
	return renderedConfig.String(), nil
}

type Stackdriver struct {
	StackdriverID string
	UserAgent     string
	Prefix        string
}

var stackdriverTemplate = template.Must(template.New("stackdriver").Parse(stackdriverExporterConf))

func (s Stackdriver) renderConfig() (string, error) {
	var renderedStackdriverConfig strings.Builder
	if err := stackdriverTemplate.Execute(&renderedStackdriverConfig, s); err != nil {
		return "", err
	}
	return renderedStackdriverConfig.String(), nil
}

func GenerateOtelConfig(hostMetricsList []*HostMetrics, mssqlList []*MSSQL, iisList []*IIS, stackdriverList []*Stackdriver, serviceList []*Service, userAgent string, versionLabel string) (string, error) {
	receiversConfigSection := []string{}
	exportersConfigSection := []string{}
	processorsConfigSection := []string{}
	serviceConfigSection := []string{}
	receiversConfigSection = append(receiversConfigSection, agentReceiverConf)
	agentExporter := Stackdriver{
		StackdriverID: "agent",
		Prefix:        "agent.googleapis.com/",
	}
	stackdriverList = append(stackdriverList, &agentExporter)
	serviceConfigSection = append(serviceConfigSection, agentServiceConf)
	for _, h := range hostMetricsList {
		configSection, err := h.renderConfig()
		if err != nil {
			return "", err
		}
		receiversConfigSection = append(receiversConfigSection, configSection)
	}
	for _, m := range mssqlList {
		configSection, err := m.renderConfig()
		if err != nil {
			return "", err
		}
		receiversConfigSection = append(receiversConfigSection, configSection)
	}
	for _, i := range iisList {
		configSection, err := i.renderConfig()
		if err != nil {
			return "", err
		}
		receiversConfigSection = append(receiversConfigSection, configSection)
	}
	for _, s := range stackdriverList {
		s.UserAgent = userAgent
		configSection, err := s.renderConfig()
		if err != nil {
			return "", err
		}
		exportersConfigSection = append(exportersConfigSection, configSection)
	}
	for _, s := range serviceList {
		configSection, err := s.renderConfig()
		if err != nil {
			return "", err
		}
		serviceConfigSection = append(serviceConfigSection, configSection)
	}
	p := Processors{
		Version: versionLabel,
	}
	configSection, err := p.renderConfig()
	if err != nil {
		return "", err
	}
	processorsConfigSection = append(processorsConfigSection, configSection)
	configSections := configSections{
		ReceiversConfigSection:  receiversConfigSection,
		ProcessorsConfigSection: processorsConfigSection,
		ExportersConfigSection:  exportersConfigSection,
		ServiceConfigSection:    serviceConfigSection,
	}
	conf, err := template.New("otelConf").Parse(confTemplate)
	if err != nil {
		return "", err
	}

	var configBuilder strings.Builder
	if err := conf.Execute(&configBuilder, configSections); err != nil {
		return "", err
	}
	return configBuilder.String(), nil
}
