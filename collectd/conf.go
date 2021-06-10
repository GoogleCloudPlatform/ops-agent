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

// Package collectd provides data structures to represent and generate collectd
// configuration.
package collectd

import (
	"errors"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/config"
)

// Collectd internal config related.
type collectdConf struct {
	scrapeInternal    float64
	enableHostMetrics bool
}

const (
	defaultScrapeInterval = float64(60)

	scrapeIntervalConfigFormat = "Interval %v\n"

	fixedConfig = `
# Explicitly set hostname to "" to indicate the default resource.
Hostname ""

# The Stackdriver agent does not use fully qualified domain names.
FQDNLookup false

# Collectd processes its config in order, so this must be loaded first in order
# to catch messages from other plugins during configuration.
LoadPlugin syslog
<Plugin "syslog">
  LogLevel "info"
</Plugin>

LoadPlugin logfile
<Plugin "logfile">
  LogLevel "info"
  File "{{.LogsDir}}/metrics-module.log"
  Timestamp true
</Plugin>

LoadPlugin stackdriver_agent
LoadPlugin write_gcm
<Plugin "write_gcm">
  PrettyPrintJSON false
</Plugin>
`
)

var translation = map[string]string{
	"cpu": `
LoadPlugin load
LoadPlugin cpu
<Plugin "cpu">
  ValuesPercentage true
  ReportByCpu true
  ReportByState true
</Plugin>
`,
	// ---
	"disk": `
LoadPlugin disk
<Plugin "disk">
</Plugin>

LoadPlugin df
<Plugin "df">
  FSType "devfs"
  IgnoreSelected true
  ReportByDevice true
  ValuesPercentage true
</Plugin>
`,
	// ---
	"memory": `
LoadPlugin memory
<Plugin "memory">
  ValuesPercentage true
</Plugin>
`,
	// ---
	"network": `
LoadPlugin interface
<Plugin "interface">
</Plugin>

LoadPlugin tcpconns
<Plugin "tcpconns">
  AllPortsSummary true
</Plugin>
`,
	// ---
	"swap": `
LoadPlugin swap
<Plugin "swap">
  ValuesPercentage true
</Plugin>
`,
	// --- Known metrics whose translations are handled outside of this map.
	"perprocess": ``,
	"process":    ``,
}

// reservedIdPrefixError returns an error message for when the user tries to define a reserved ID.
// The component should be "receiver", "processor", or "exporter".
func reservedIdPrefixError(component, id string) error {
	// e.g. metrics receiver id %q is not allowed because prefix 'lib:' is reserved for pre-defined receivers.
	return fmt.Errorf(`metrics %s id %q is not allowed because prefix 'lib:' is reserved for pre-defined %ss.`,
		component, id, component)
}

func GenerateCollectdConfig(metrics *config.Metrics, logsDir string) (string, error) {
	var sb strings.Builder

	collectdConf, err := validatedCollectdConfig(metrics)
	if err != nil {
		return "", err
	}

	appendScrapeIntervalConfig(&sb, collectdConf.scrapeInternal)
	err = appendFixedConfig(&sb, logsDir)
	if err != nil {
		return "", err
	}

	if collectdConf.enableHostMetrics {
		err = appendHostMetricsConfig(&sb)
		if err != nil {
			return "", err
		}
	}

	return sb.String(), nil
}

func validatedCollectdConfig(metrics *config.Metrics) (*collectdConf, error) {
	collectdConf := collectdConf{
		scrapeInternal:    defaultScrapeInterval,
		enableHostMetrics: false,
	}
	definedReceiverIDs := map[string]bool{}
	definedExporterIDs := map[string]bool{}

	// Skip validation if metrics config is not set.
	// In other words receivers, exporters and pipelines are all empty.
	if metrics == nil || (len(metrics.Receivers) == 0 && len(metrics.Exporters) == 0 && len(metrics.Service.Pipelines) == 0) {
		return &collectdConf, nil
	}

	// Validate Metrics.Receivers.
	if len(metrics.Receivers) > 1 {
		return nil, errors.New(`at most one metrics receiver with type "hostmetrics" is allowed.`)
	}
	for receiverID, receiver := range metrics.Receivers {
		if strings.HasPrefix(receiverID, "lib:") {
			return nil, reservedIdPrefixError("receiver", receiverID)
		}
		if receiver.Type != "hostmetrics" {
			return nil, fmt.Errorf("metrics receiver %q with type %q is not supported. Supported metrics receiver types: [hostmetrics].", receiverID, receiver.Type)
		}
		collectdConf.enableHostMetrics = true

		if receiver.CollectionInterval != "" {
			t, err := time.ParseDuration(receiver.CollectionInterval)
			if err != nil {
				return nil, fmt.Errorf("parameter \"collection_interval\" in metrics receiver %q has invalid value %q that is not an interval (e.g. \"60s\"). Detailed error: %s", receiverID, receiver.CollectionInterval, err)
			}
			interval := t.Seconds()
			if interval < 10 {
				return nil, fmt.Errorf("parameter \"collection_interval\" in metrics receiver %q has invalid value \"%vs\" that is below the minimum threshold of \"10s\".", receiverID, interval)
			}
			collectdConf.scrapeInternal = interval
		}
		definedReceiverIDs[receiverID] = true
	}

	// Validate Metrics.Exporters.
	if len(metrics.Exporters) != 1 {
		return nil, errors.New("exactly one metrics exporter with type 'google_cloud_monitoring' is required.")
	}
	for exporterID, exporter := range metrics.Exporters {
		if strings.HasPrefix(exporterID, "lib:") {
			return nil, reservedIdPrefixError("exporter", exporterID)
		}
		if exporter.Type != "google_cloud_monitoring" {
			return nil, fmt.Errorf("metrics exporter %q with type %q is not supported. Supported metrics exporter types: [google_cloud_monitoring].", exporterID, exporter.Type)
		}
		definedExporterIDs[exporterID] = true
	}

	// Validate Metrics.Service.
	if len(metrics.Service.Pipelines) != 1 {
		return nil, errors.New("exactly one metrics service pipeline is required.")
	}
	for pipelineID, pipeline := range metrics.Service.Pipelines {
		if strings.HasPrefix(pipelineID, "lib:") {
			return nil, reservedIdPrefixError("pipeline", pipelineID)
		}
		if len(pipeline.ReceiverIDs) != 1 {
			return nil, errors.New("exactly one receiver id is required in the metrics service pipeline receiver id list.")
		}
		invalidReceiverIDs := findInvalid(definedReceiverIDs, pipeline.ReceiverIDs)
		if len(invalidReceiverIDs) > 0 {
			return nil, fmt.Errorf("metrics receiver %q from pipeline %q is not defined.", invalidReceiverIDs[0], pipelineID)
		}

		if len(pipeline.ExporterIDs) != 1 {
			return nil, errors.New("exactly one exporter id is required in the metrics service pipeline exporter id list.")
		}
		invalidExporterIDs := findInvalid(definedExporterIDs, pipeline.ExporterIDs)
		if len(invalidExporterIDs) > 0 {
			return nil, fmt.Errorf("metrics exporter %q from pipeline %q is not defined.", invalidExporterIDs[0], pipelineID)
		}
	}
	return &collectdConf, nil
}

// Checks if any string in a []string type slice is not in an allowed slice.
func findInvalid(allowed map[string]bool, actual []string) []string {
	var invalid []string
	for _, v := range actual {
		if !allowed[v] {
			invalid = append(invalid, v)
		}
	}
	return invalid
}

// Write the configuration line for the scrape interval. If the user didn't
// specify a value, use the default value. Collectd configuration requires
// this value to be in seconds. Minimum allowed value is 10 seconds.
// NOTE: Internally, collectd parses this value with strtod(...). If this
// fails, it will silently fall back to 10 seconds. See:
// https://github.com/Stackdriver/collectd/blob/stackdriver-agent-5.8.1/src/daemon/configfile.c#L909-L911
func appendScrapeIntervalConfig(configBuilder *strings.Builder, interval float64) {
	configBuilder.WriteString(fmt.Sprintf(scrapeIntervalConfigFormat, interval))
}

func appendFixedConfig(configBuilder *strings.Builder, logsDir string) error {
	var fixedConfigBuilder strings.Builder

	fixedConfigTemplate, err := template.New("collectdFixedConf").Parse(fixedConfig)
	if err != nil {
		return err
	}
	if err = fixedConfigTemplate.Execute(&fixedConfigBuilder, struct{ LogsDir string }{logsDir}); err != nil {
		return err
	}
	configBuilder.WriteString(fixedConfigBuilder.String())
	return nil
}

func appendHostMetricsConfig(configBuilder *strings.Builder) error {
	// TODO(lingshi): Add logic to inspect user input to determine what subgroups areis included instead of hard coding
	// when we settle down the design.
	for _, metricGroup := range []string{"cpu", "disk", "memory", "network", "swap"} {
		configBuilder.WriteString(translation[metricGroup])
	}

	// -- PROCESSES PLUGIN CONFIG --
	err := appendProcessesPluginConfig(configBuilder)
	if err != nil {
		return fmt.Errorf("failed to generate 'processes' plugin config: %w", err)
	}

	return nil
}

func appendProcessesPluginConfig(configBuilder *strings.Builder) error {
	processesPluginTemplate, err := template.New("processesPlugin").Parse(`
LoadPlugin processes
LoadPlugin match_regex
<Plugin "processes">
  ProcessMatch "all" ".*"
  {{- if .IncludePerProcess }}
  Detail "ps_cputime"
  Detail "ps_disk_octets"
  Detail "ps_rss"
  Detail "ps_vm"
  {{- end }}
</Plugin>

PostCacheChain "PostCache"
<Chain "PostCache">
  # Send all expected process metrics to the output plugin.
  <Rule "processes">
    <Match "regex">
      Plugin "^processes$"
      {{- if and .IncludePerProcess .IncludeProcess}}
      Type "^(ps_cputime|disk_octets|ps_rss|ps_vm|fork_rate|ps_state)$"
      {{- else if .IncludePerProcess}}
      Type "^(ps_cputime|disk_octets|ps_rss|ps_vm)$"
      {{- else }}
      Type "^(fork_rate|ps_state)$"
      {{- end }}
    </Match>
    <Target "jump">
      Chain "WriteAndStop"
    </Target>
  </Rule>
  # Stop processing on (do not write) all unexpected process metrics.
  <Rule "processes_exclude">
    <Match "regex">
      Plugin "^processes$"
    </Match>
    Target "stop"
  </Rule>
  # Send all other metrics to the output plugin.
  <Target "jump">
    Chain "WriteAndStop"
  </Target>
</Chain>

<Chain "WriteAndStop">
  <Rule "write">
    <Target "write">
      Plugin "write_gcm"
    </Target>
  </Rule>
  Target "stop"
</Chain>
`)

	if err != nil {
		return err
	}

	return processesPluginTemplate.Execute(
		configBuilder,
		struct{ IncludeProcess, IncludePerProcess bool }{true, true})
}
