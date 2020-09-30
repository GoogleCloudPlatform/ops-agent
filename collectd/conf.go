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
  "fmt"
  "reflect"
  "sort"
  "strings"
  "text/template"
  "time"
)

type Metrics struct {
  Interval string `yaml:"interval"` // time.Duration format
  Input    Input  `yaml:"input"`
}

type Input struct {
  Include []string `yaml:"include"`
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

LoadPlugin stackdriver_agent
LoadPlugin write_gcm
<Plugin "write_gcm">
  PrettyPrintJSON false
</Plugin>
`
)

var translation = map[string]string{
  "cpu": `
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

func GenerateCollectdConfig(metrics Metrics) (string, error) {
  var sb strings.Builder

  // -- SCRAPE INTERVAL --
  // Write the configuration line for the scrape interval. If the user didn't
  // specify a value, use the default value. Collectd configuration requires
  // this value to be in seconds. Minimum allowed value is 10 seconds.
  // NOTE: Internally, collectd parses this value with strtod(...). If this
  // fails, it will silently fall back to 10 seconds. See:
  // https://github.com/Stackdriver/collectd/blob/stackdriver-agent-5.8.1/src/daemon/configfile.c#L909-L911
  interval := defaultScrapeInterval
  if metrics.Interval != "" {
    t, err := time.ParseDuration(metrics.Interval)
    if err != nil {
      return "", fmt.Errorf("invalid scrape interval: %v", err)
    }
    interval = t.Seconds()
    if interval < 10 {
      return "", fmt.Errorf("minimum allowed scrape interval is 10s, got %vs", interval)
    }
  }
  sb.WriteString(fmt.Sprintf(scrapeIntervalConfigFormat, interval))

  // -- FIXED CONFIG --
  sb.WriteString(fixedConfig)

  // -- CUSTOM CONFIG --
  // Write the configuration for each user-specified metric to scrape.
  sort.Strings(metrics.Input.Include)
  for _, metric := range metrics.Input.Include {
    if config, ok := translation[metric]; ok {
      sb.WriteString(config)
    } else {
      return "", fmt.Errorf("metric input '%s' not in known values: %v", metric, reflect.ValueOf(translation).MapKeys())
    }
  }

  // -- PROCESSES PLUGIN CONFIG
  err := appendProcessesPluginConfig(&sb, metrics.Input)
  if err != nil {
    return "", fmt.Errorf("failed to generate 'processes' plugin config: %w", err)
  }

  return sb.String(), nil
}

func appendProcessesPluginConfig(configBuilder *strings.Builder, metrics Input) error {
  var includeProcess, includePerProcess bool

  for _, metric := range metrics.Include {
    if metric == "process" {
      includeProcess = true
    } else if metric == "perprocess" {
      includePerProcess = true
    }
  }

  if !includeProcess && !includePerProcess {
    return nil
  }

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
    struct{ IncludeProcess, IncludePerProcess bool }{includeProcess, includePerProcess})
}
