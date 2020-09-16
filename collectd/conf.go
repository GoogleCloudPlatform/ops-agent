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
  "strings"
)

type Metrics struct {
  Interval uint32   `yaml:"interval"`
  Scrape   []string `yaml:"scrape"`
}

const (
  defaultScrapeInterval = uint32(60)

  scrapeIntervalConfigFormat = "Interval %d\n"

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
  "swap": `
LoadPlugin swap
<Plugin "swap">
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
  "process": `
LoadPlugin processes
LoadPlugin match_regex
LoadPlugin match_throttle_metadata_keys
<Plugin "processes">
  ProcessMatch "all" ".*"
  Detail "ps_cputime"
  Detail "ps_disk_octets"
  Detail "ps_rss"
  Detail "ps_vm"
</Plugin>

PostCacheChain "PostCache"
<Chain "PostCache">
  <Rule "processes">
    <Match "regex">
      Plugin "^processes$"
      Type "^(ps_cputime|disk_octets|ps_rss|ps_vm)$"
    </Match>
    <Target "jump">
      Chain "MaybeThrottleProcesses"
    </Target>
    Target "stop"
  </Rule>

  <Rule "otherwise">
    <Match "throttle_metadata_keys">
      OKToThrottle false
      HighWaterMark 5700000000  # 950M * 6
      LowWaterMark 4800000000  # 800M * 6
    </Match>
    <Target "write">
       Plugin "write_gcm"
    </Target>
  </Rule>
</Chain>

<Chain "MaybeThrottleProcesses">
  <Rule "default">
    <Match "throttle_metadata_keys">
      OKToThrottle true
      TrackedMetadata "processes:pid"
      TrackedMetadata "processes:command"
      TrackedMetadata "processes:command_line"
      TrackedMetadata "processes:owner"
    </Match>
    <Target "write">
       Plugin "write_gcm"
    </Target>
  </Rule>
</Chain>
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
}

func GenerateCollectdConfig(metrics Metrics) (string, error) {
  var sb strings.Builder

  // -- SCRAPE INTERVAL --
  // Write the configuration line for the scrape interval. If the user didn't
  // specify a value, or if the value is 0, use the default value.
  interval := defaultScrapeInterval
  if metrics.Interval != 0 {
    interval = metrics.Interval
  }
  sb.WriteString(fmt.Sprintf(scrapeIntervalConfigFormat, interval))

  // -- FIXED CONFIG --
  sb.WriteString(fixedConfig)
  // sb.WriteString(default_conf)

  // -- CUSTOM CONFIG --
  // Write the configuration for each user-specified metric to scrape.
  for _, metric := range metrics.Scrape {
    if config, ok := translation[metric]; ok {
      sb.WriteString(config)
    } else {
      return "", fmt.Errorf("metric input '%s' not in known values: %v", metric, reflect.ValueOf(translation).MapKeys())
    }
  }

  return sb.String(), nil
}
