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

// Package fluentbit provides data structures to represent and generate fluentBit configuration.
package fluentbit

import (
	"strings"
	"text/template"
)

var mainConfTemplate = template.Must(template.New("fluentBitMainConf").Parse(`[SERVICE]
    # https://docs.fluentbit.io/manual/administration/configuring-fluent-bit/configuration-file#config_section
    # Flush logs every 1 second, even if the buffer is not full to minimize log entry arrival delay.
    Flush      1
    # We use systemd to manage Fluent Bit instead.
    Daemon     off
    # Log_File is set by Fluent Bit systemd unit (e.g. /var/log/google-cloud-ops-agent/subagents/logging-module.log).
    Log_Level  info

    # https://docs.fluentbit.io/manual/administration/monitoring
    # Enable a built-in HTTP server that can be used to query internal information and monitor metrics of each running plugin.
    HTTP_Server  On
    HTTP_Listen  0.0.0.0
    HTTP_PORT    2020

    # https://docs.fluentbit.io/manual/administration/buffering-and-storage#service-section-configuration
    # storage.path is set by Fluent Bit systemd unit (e.g. /var/lib/google-cloud-ops-agent/fluent-bit/buffers).
    storage.sync               normal
    # Enable the data integrity check when writing and reading data from the filesystem.
    storage.checksum           on
    # The maximum amount of data to load into the memory when processing old chunks from the backlog that is from previous Fluent Bit processes (e.g. Fluent Bit may have crashed or restarted).
    storage.backlog.mem_limit  50M
    # Enable storage metrics in the built-in HTTP server.
    storage.metrics            on
    # This is exclusive to filesystem storage type. It specifies the number of chunks (every chunk is a file) that can be up in memory.
    # Every chunk is a file, so having it up in memory means having an open file descriptor. In case there are thousands of chunks,
    # we don't want them to all be loaded into the memory.
    storage.max_chunks_up      128

{{range .TailConfigSections -}}
{{.}}

{{end}}
{{- range .SyslogConfigSections -}}
{{.}}

{{end}}
{{- range .WineventlogConfigSections -}}
{{.}}

{{end}}
{{- range .FilterParserConfigSections -}}
{{.}}

{{end}}
{{- range .FilterModifyAddLogNameConfigSections -}}
{{.}}

{{end}}
{{- range .FilterRewriteTagSections -}}
{{.}}

{{end}}
{{- range .FilterModifyRemoveLogNameConfigSections -}}
{{.}}

{{end}}
{{- range .StackdriverConfigSections -}}
{{.}}

{{end}}
{{- define "filter_modify_add_log_name" -}}
[FILTER]
    Name  modify
    Match {{.Match}}
    Add   logName {{.LogName}}
{{- end -}}
{{- define "filter_modify_remove_log_name" -}}
[FILTER]
    Name   modify
    Match  {{.Match}}
    Remove logName
{{- end -}}
{{- define "filter_parser" -}}
[FILTER]
    Name     parser
    Match    {{.Match}}
    Key_Name {{.KeyName}}
    Parser   {{.Parser}}
{{- end -}}
{{- define "filter_rewrite_tag" -}}
[FILTER]
    Name                  rewrite_tag
    Match                 {{.Match}}
    Rule                  $logName .* $logName false
    Emitter_Storage.type  filesystem
    Emitter_Mem_Buf_Limit 10M
{{- end -}}
{{- define "tail" -}}
[INPUT]
    # https://docs.fluentbit.io/manual/pipeline/inputs/tail#config
    Name               tail
    Tag                {{.Tag}}
    Path               {{.Path}}
    DB                 {{.DB}}
    Read_from_Head     True
    # Set the chunk limit conservatively to avoid exceeding the recommended chunk size of 5MB per write request.
    Buffer_Chunk_Size  512k
    # Set the max size a bit larger to accommodate for long log lines.
    Buffer_Max_Size    5M
    # When a message is unstructured (no parser applied), append it under a key named "message".
    Key                message
    # Increase this to 30 seconds so log rotations are handled more gracefully.
    Rotate_Wait        30
    # Skip long lines instead of skipping the entire file when a long line exceeds buffer size.
    Skip_Long_Lines    On
{{- if (ne .ExcludePath "")}}
    # Exclude files matching this criteria.
    Exclude_Path       {{.ExcludePath}}
{{- end}}

    # https://docs.fluentbit.io/manual/administration/buffering-and-storage#input-section-configuration
    # Buffer in disk to improve reliability.
    storage.type       filesystem

    # https://docs.fluentbit.io/manual/administration/backpressure#mem_buf_limit
    # This controls how much data the input plugin can hold in memory once the data is ingested into the core.
    # This is used to deal with backpressure scenarios (e.g: cannot flush data for some reason).
    # When the input plugin hits "mem_buf_limit", because we have enabled filesystem storage type, mem_buf_limit acts
    # as a hint to set "how much data can be up in memory", once the limit is reached it continues writing to disk.
    Mem_Buf_Limit      10M
{{- end -}}
{{- define "syslog" -}}
[INPUT]
    # https://docs.fluentbit.io/manual/pipeline/inputs/syslog
    Name           syslog
    Tag            {{.Tag}}
    Mode           {{.Mode}}
    Listen         {{.Listen}}
    Port           {{.Port}}
    Parser         lib:default_message_parser

    # https://docs.fluentbit.io/manual/administration/buffering-and-storage#input-section-configuration
    # Buffer in disk to improve reliability.
    storage.type   filesystem

    # https://docs.fluentbit.io/manual/administration/backpressure#mem_buf_limit
    # This controls how much data the input plugin can hold in memory once the data is ingested into the core.
    # This is used to deal with backpressure scenarios (e.g: cannot flush data for some reason).
    # When the input plugin hits "mem_buf_limit", because we have enabled filesystem storage type, mem_buf_limit acts
    # as a hint to set "how much data can be up in memory", once the limit is reached it continues writing to disk.
    Mem_Buf_Limit  10M
{{- end -}}
{{- define "wineventlog" -}}
[INPUT]
    # https://docs.fluentbit.io/manual/pipeline/inputs/windows-event-log
    Name           winlog
    Tag            {{.Tag}}
    Channels       {{.Channels}}
    Interval_Sec   1
    DB             {{.DB}}
{{- end -}}
{{- define "stackdriver" -}}
[OUTPUT]
    # https://docs.fluentbit.io/manual/pipeline/outputs/stackdriver
    Name              stackdriver
    Match_Regex       ^({{.Match}})$
    resource          gce_instance
    stackdriver_agent {{.UserAgent}}
    {{- if .Workers}}
    workers           {{.Workers}}
    {{- end}}

    # https://docs.fluentbit.io/manual/administration/scheduling-and-retries
    # After 3 retries, a given chunk will be discarded. So bad entries don't accidentally stay around forever.
    Retry_Limit  3

    # https://docs.fluentbit.io/manual/administration/security
    # Enable TLS support.
    tls         On
    # Do not force certificate validation.
    tls.verify  Off
{{- end -}}
`))

var parserConfTemplate = template.Must(template.New("fluentBitParserConf").Parse(`[PARSER]
    Name        lib:default_message_parser
    Format      regex
    Regex       ^(?<message>.*)$

[PARSER]
    Name        lib:apache
    Format      regex
    Regex       ^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")?$
    Time_Key    time
    Time_Format %d/%b/%Y:%H:%M:%S %z

[PARSER]
    Name        lib:apache2
    Format      regex
    Regex       ^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^ ]*) +\S*)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>.*)")?$
    Time_Key    time
    Time_Format %d/%b/%Y:%H:%M:%S %z

[PARSER]
    Name   lib:apache_error
    Format regex
    Regex  ^\[[^ ]* (?<time>[^\]]*)\] \[(?<level>[^\]]*)\](?: \[pid (?<pid>[^\]]*)\])?( \[client (?<client>[^\]]*)\])? (?<message>.*)$

[PARSER]
    Name        lib:mongodb
    Format      regex
    Regex       ^(?<time>[^ ]*)\s+(?<severity>\w)\s+(?<component>[^ ]+)\s+\[(?<context>[^\]]+)]\s+(?<message>.*?) *(?<ms>(\d+))?(:?ms)?$
    Time_Key    time
    Time_Format %Y-%m-%dT%H:%M:%S.%L

[PARSER]
    Name        lib:nginx
    Format      regex
    Regex       ^(?<remote>[^ ]*) (?<host>[^ ]*) (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")
    Time_Key    time
    Time_Format %d/%b/%Y:%H:%M:%S %z

[PARSER]
    Name        lib:syslog-rfc5424
    Format      regex
    Regex       ^\<(?<pri>[0-9]{1,5})\>1 (?<time>[^ ]+) (?<host>[^ ]+) (?<ident>[^ ]+) (?<pid>[-0-9]+) (?<msgid>[^ ]+) (?<extradata>(\[(.*?)\]|-)) (?<message>.+)$
    Time_Key    time
    Time_Format %Y-%m-%dT%H:%M:%S.%L%Z

[PARSER]
    Name        lib:syslog-rfc3164
    Format      regex
    Regex       /^\<(?<pri>[0-9]+)\>(?<time>[^ ]* {1,2}[^ ]* [^ ]*) (?<host>[^ ]*) (?<ident>[a-zA-Z0-9_\/\.\-]*)(?:\[(?<pid>[0-9]+)\])?(?:[^\:]*\:)? *(?<message>.*)$/
    Time_Key    time
    Time_Format %b %d %H:%M:%S

{{range .JSONParserConfigSections -}}
{{.}}

{{end}}
{{- range .RegexParserConfigSections -}}
{{.}}

{{end}}
{{- define "parserJSON" -}}
[PARSER]
    Name        {{.Name}}
    Format      json
{{- if (ne .TimeKey "")}}
    Time_Key    {{.TimeKey}}
{{- end}}
{{- if (ne .TimeFormat "")}}
    Time_Format {{.TimeFormat}}
{{- end}}
{{- end -}}
{{- define "parserRegex" -}}
[PARSER]
    Name        {{.Name}}
    Format      regex
    Regex       {{.Regex}}
{{- if (ne .TimeKey "")}}
    Time_Key    {{.TimeKey}}
{{- end}}
{{- if (ne .TimeFormat "")}}
    Time_Format {{.TimeFormat}}
{{- end}}
{{- end -}}
`))

type mainConfigSections struct {
	TailConfigSections                      []string
	SyslogConfigSections                    []string
	WineventlogConfigSections               []string
	FilterParserConfigSections              []string
	FilterModifyAddLogNameConfigSections    []string
	FilterRewriteTagSections                []string
	FilterModifyRemoveLogNameConfigSections []string
	StackdriverConfigSections               []string
}

type parserConfigSections struct {
	JSONParserConfigSections  []string
	RegexParserConfigSections []string
}

type Config struct {
	Tails                      []*Tail
	Syslogs                    []*Syslog
	Wineventlogs               []*WindowsEventlog
	FilterParserGroups         []FilterParserGroup
	FilterModifyAddLogNames    []*FilterModifyAddLogName
	FilterRewriteTags          []*FilterRewriteTag
	FilterModifyRemoveLogNames []*FilterModifyRemoveLogName
	Stackdrivers               []*Stackdriver
	JsonParsers                []*ParserJSON
	RegexParsers               []*ParserRegex
	UserAgent                  string
}

func (c Config) generateMain() (string, error) {
	tailConfigSections := []string{}
	syslogConfigSections := []string{}
	wineventlogConfigSections := []string{}
	filterParserConfigSections := []string{}
	filterModifyAddLogNameConfigSections := []string{}
	filterRewriteTagSections := []string{}
	filterModifyRemoveLogNameConfigSections := []string{}
	stackdriverConfigSections := []string{}
	for _, t := range c.Tails {
		configSection, err := t.renderConfig()
		if err != nil {
			return "", err
		}
		tailConfigSections = append(tailConfigSections, configSection)
	}
	for _, s := range c.Syslogs {
		configSection, err := s.renderConfig()
		if err != nil {
			return "", err
		}
		syslogConfigSections = append(syslogConfigSections, configSection)
	}
	for _, w := range c.Wineventlogs {
		configSection, err := w.renderConfig()
		if err != nil {
			return "", err
		}
		wineventlogConfigSections = append(wineventlogConfigSections, configSection)
	}
	for _, filterParsers := range c.FilterParserGroups {
		var filters []string
		for _, f := range filterParsers {
			configSection, err := f.renderConfig()
			if err != nil {
				return "", err
			}
			filters = append(filters, configSection)
		}
		filterParserConfigSections = append(filterParserConfigSections, strings.Join(filters, "\n\n"))
	}
	for _, f := range c.FilterModifyAddLogNames {
		configSection, err := f.renderConfig()
		if err != nil {
			return "", err
		}
		filterModifyAddLogNameConfigSections = append(filterModifyAddLogNameConfigSections, configSection)
	}
	for _, f := range c.FilterRewriteTags {
		configSection, err := f.renderConfig()
		if err != nil {
			return "", err
		}
		filterRewriteTagSections = append(filterRewriteTagSections, configSection)
	}
	for _, f := range c.FilterModifyRemoveLogNames {
		configSection, err := f.renderConfig()
		if err != nil {
			return "", err
		}
		filterModifyRemoveLogNameConfigSections = append(filterModifyRemoveLogNameConfigSections, configSection)
	}
	for _, s := range c.Stackdrivers {
		s.UserAgent = c.UserAgent
		configSection, err := s.renderConfig()
		if err != nil {
			return "", err
		}
		stackdriverConfigSections = append(stackdriverConfigSections, configSection)
	}

	configSections := mainConfigSections{
		TailConfigSections:                      tailConfigSections,
		SyslogConfigSections:                    syslogConfigSections,
		WineventlogConfigSections:               wineventlogConfigSections,
		FilterParserConfigSections:              filterParserConfigSections,
		FilterModifyAddLogNameConfigSections:    filterModifyAddLogNameConfigSections,
		FilterRewriteTagSections:                filterRewriteTagSections,
		FilterModifyRemoveLogNameConfigSections: filterModifyRemoveLogNameConfigSections,
		StackdriverConfigSections:               stackdriverConfigSections,
	}

	var mainConfigBuilder strings.Builder
	if err := mainConfTemplate.Execute(&mainConfigBuilder, configSections); err != nil {
		return "", err
	}
	return mainConfigBuilder.String(), nil
}

func (c Config) generateParser() (string, error) {
	jsonParserConfigSections := []string{}
	for _, j := range c.JsonParsers {
		configSection, err := j.renderConfig()
		if err != nil {
			return "", err
		}
		jsonParserConfigSections = append(jsonParserConfigSections, configSection)
	}

	regexParserConfigSections := []string{}
	for _, r := range c.RegexParsers {
		configSection, err := r.renderConfig()
		if err != nil {
			return "", err
		}
		regexParserConfigSections = append(regexParserConfigSections, configSection)
	}

	parsers := parserConfigSections{
		JSONParserConfigSections:  jsonParserConfigSections,
		RegexParserConfigSections: regexParserConfigSections,
	}

	var parserConfigBuilder strings.Builder
	if err := parserConfTemplate.Execute(&parserConfigBuilder, parsers); err != nil {
		return "", err
	}
	return parserConfigBuilder.String(), nil
}

func (c Config) Generate() (mainConfig string, parserConfig string, err error) {
	mainConfig, err = c.generateMain()
	if err != nil {
		return "", "", err
	}

	parserConfig, err = c.generateParser()
	if err != nil {
		return "", "", err
	}

	return mainConfig, parserConfig, nil
}

// A FilterParserGroup represents a list of filters to be applied in order.
type FilterParserGroup []*FilterParser

// A FilterParser represents the configuration data for fluentBit's filter parser plugin.
type FilterParser struct {
	Match   string
	KeyName string
	Parser  string
}

func (f FilterParser) renderConfig() (string, error) {
	var renderedFilterParserConfig strings.Builder
	if err := mainConfTemplate.ExecuteTemplate(&renderedFilterParserConfig, "filter_parser", f); err != nil {
		return "", err
	}
	return renderedFilterParserConfig.String(), nil
}

// A FilterModifyAddLogName represents the configuration data for using fluentBit's Modify
// filter to add logName.
type FilterModifyAddLogName struct {
	Match   string
	LogName string
}

func (f FilterModifyAddLogName) renderConfig() (string, error) {
	var renderedFilterModifyAddLogNameConfig strings.Builder
	if err := mainConfTemplate.ExecuteTemplate(&renderedFilterModifyAddLogNameConfig, "filter_modify_add_log_name", f); err != nil {
		return "", err
	}
	return renderedFilterModifyAddLogNameConfig.String(), nil
}

// A FilterModifyRemoveLogName represents the configuration data for using fluentBit's Modify
// filter to remove logName.
type FilterModifyRemoveLogName struct {
	Match string
}

func (f FilterModifyRemoveLogName) renderConfig() (string, error) {
	var renderedFilterModifyRemoveLogNameConfig strings.Builder
	if err := mainConfTemplate.ExecuteTemplate(&renderedFilterModifyRemoveLogNameConfig, "filter_modify_remove_log_name", f); err != nil {
		return "", err
	}
	return renderedFilterModifyRemoveLogNameConfig.String(), nil
}

// A FilterRewriteTag represents the configuration data for fluentBit's RewriteTag filter.
type FilterRewriteTag struct {
	Match string
}

func (f FilterRewriteTag) renderConfig() (string, error) {
	var renderedFilterRewriteTagConfig strings.Builder
	if err := mainConfTemplate.ExecuteTemplate(&renderedFilterRewriteTagConfig, "filter_rewrite_tag", f); err != nil {
		return "", err
	}
	return renderedFilterRewriteTagConfig.String(), nil
}

// A ParserJSON represents the configuration data for fluentBit's JSON parser.
type ParserJSON struct {
	Name       string
	TimeKey    string
	TimeFormat string
}

// renderConfig generates a section for configure fluentBit JSON parser.
func (p ParserJSON) renderConfig() (string, error) {
	var b strings.Builder
	if err := parserConfTemplate.ExecuteTemplate(&b, "parserJSON", p); err != nil {
		return "", err
	}
	return b.String(), nil
}

// A ParserRegex represents the configuration data for fluentBit's Regex parser.
type ParserRegex struct {
	Name       string
	Regex      string
	TimeKey    string
	TimeFormat string
}

// renderConfig generates a section for configure fluentBit Regex parser.
func (p ParserRegex) renderConfig() (string, error) {
	var b strings.Builder
	if err := parserConfTemplate.ExecuteTemplate(&b, "parserRegex", p); err != nil {
		return "", err
	}
	return b.String(), nil
}

// A Tail represents the configuration data for fluentBit's tail input plugin.
type Tail struct {
	Tag         string
	Path        string
	ExcludePath string
	DB          string
}

// renderConfig generates a section for configure fluentBit tail parser.
func (t Tail) renderConfig() (string, error) {
	var renderedTailConfig strings.Builder
	if err := mainConfTemplate.ExecuteTemplate(&renderedTailConfig, "tail", t); err != nil {
		return "", err
	}
	return renderedTailConfig.String(), nil
}

// A Syslog represents the configuration data for fluentBit's syslog input plugin.
type Syslog struct {
	Mode   string
	Listen string
	Port   uint16
	Tag    string
}

// renderConfig generates a section for configure fluentBit syslog input plugin.
func (s Syslog) renderConfig() (string, error) {
	var renderedSyslogConfig strings.Builder
	if err := mainConfTemplate.ExecuteTemplate(&renderedSyslogConfig, "syslog", s); err != nil {
		return "", err
	}
	return renderedSyslogConfig.String(), nil
}

// A WindowsEventlog represents the configuration data for fluentbit's winlog input plugin
type WindowsEventlog struct {
	Tag          string
	Channels     string
	Interval_Sec string
	DB           string
}

// renderConfig generates a section for configure fluentBit wineventlog input plugin.
func (w WindowsEventlog) renderConfig() (string, error) {
	var renderedWineventlogConfig strings.Builder
	if err := mainConfTemplate.ExecuteTemplate(&renderedWineventlogConfig, "wineventlog", w); err != nil {
		return "", err
	}
	return renderedWineventlogConfig.String(), nil
}

// A Stackdriver represents the configurable data for fluentBit's stackdriver output plugin.
type Stackdriver struct {
	Match     string
	UserAgent string
	Workers   int
}

// renderConfig generates a section for configure fluentBit stackdriver output plugin.
func (s Stackdriver) renderConfig() (string, error) {
	var renderedStackdriverConfig strings.Builder
	if err := mainConfTemplate.ExecuteTemplate(&renderedStackdriverConfig, "stackdriver", s); err != nil {
		return "", err
	}
	return renderedStackdriverConfig.String(), nil
}
