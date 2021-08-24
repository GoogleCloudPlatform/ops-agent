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

var mainConfTemplate = template.Must(
	template.New("fluentBitMainConf").
		Funcs(template.FuncMap{
			"join":   strings.Join,
			"dbpath": DBPath,
		}).
		Parse(`@SET buffers_dir={{.StateDir}}/buffers
@SET logs_dir={{.LogsDir}}

[SERVICE]
    Daemon                    off
    Flush                     1
    HTTP_Listen               0.0.0.0
    HTTP_PORT                 2020
    HTTP_Server               On
    Log_Level                 info
    storage.backlog.mem_limit 50M
    storage.checksum          on
    storage.max_chunks_up     128
    storage.metrics           on
    storage.sync              normal

{{- range .Inputs}}

{{.Generate}}
{{- end}}
{{- range .Filters}}

{{.Generate}}
{{- end}}
{{- range .Outputs}}

{{.Generate}}
{{- end}}
{{- define "filter_modify_add_log_name" -}}
[FILTER]
    Add   logName {{.LogName}}
    Match {{.Match}}
    Name  modify
{{- end -}}
{{- define "filter_modify_remove_log_name" -}}
[FILTER]
    Match  {{.Match}}
    Name   modify
    Remove logName
{{- end -}}
{{- define "filter_parser" -}}
[FILTER]
    Key_Name {{.KeyName}}
    Match    {{.Match}}
    Name     parser
    Parser   {{.Parser}}
{{- end -}}
{{- define "filter_rewrite_tag" -}}
[FILTER]
    Emitter_Mem_Buf_Limit 10M
    Emitter_Storage.type  filesystem
    Match                 {{.Match}}
    Name                  rewrite_tag
    Rule                  $logName .* $logName false
{{- end -}}
{{- define "tail" -}}
[INPUT]
    Buffer_Chunk_Size 512k
    Buffer_Max_Size   5M
    DB                {{dbpath .Tag}}
{{- with .ExcludePaths}}
    Exclude_Path      {{join . ","}}
{{- end}}
    Key               message
    Mem_Buf_Limit     10M
    Name              tail
    Path              {{join .IncludePaths ","}}
    Read_from_Head    True
    Rotate_Wait       30
    Skip_Long_Lines   On
    Tag               {{.Tag}}
    storage.type      filesystem
{{- end -}}
{{- define "syslog" -}}
[INPUT]
    Listen        {{.Listen}}
    Mem_Buf_Limit 10M
    Mode          {{.Mode}}
    Name          syslog
    Parser        lib:default_message_parser
    Port          {{.Port}}
    Tag           {{.Tag}}
    storage.type  filesystem
{{- end -}}
{{- define "wineventlog" -}}
[INPUT]
    Channels     {{join .Channels ","}}
    DB           {{dbpath .Tag}}
    Interval_Sec 1
    Name         winlog
    Tag          {{.Tag}}
{{- end -}}
{{- define "stackdriver" -}}
[OUTPUT]
    Match_Regex       ^({{.Match}})$
    Name              stackdriver
    Retry_Limit       3
    resource          gce_instance
    stackdriver_agent {{.UserAgent}}
    tls               On
    tls.verify        Off
    {{- if .Workers}}
    workers           {{.Workers}}
    {{- end}}
{{- end -}}
`))

var parserConfTemplate = template.Must(template.New("fluentBitParserConf").Parse(`{{- range .Parsers -}}
{{.Generate}}

{{end -}}
{{define "parserJSON" -}}
[PARSER]
    Format      json
    Name        {{.Name}}
{{- if (ne .TimeFormat "")}}
    Time_Format {{.TimeFormat}}
{{- end}}
{{- if (ne .TimeKey "")}}
    Time_Key    {{.TimeKey}}
{{- end}}
{{- end -}}
{{- define "parserRegex" -}}
[PARSER]
    Format      regex
    Name        {{.Name}}
    Regex       {{.Regex}}
{{- if (ne .TimeFormat "")}}
    Time_Format {{.TimeFormat}}
{{- end}}
{{- if (ne .TimeKey "")}}
    Time_Key    {{.TimeKey}}
{{- end}}
{{- end -}}
`))

var DefaultParsers = []Parser{
	&ParserRegex{
		Name:  "lib:default_message_parser",
		Regex: `^(?<message>.*)$`,
	},
	&ParserRegex{
		Name:       "lib:apache",
		Regex:      `^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")?$`,
		TimeKey:    "time",
		TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
	},
	&ParserRegex{
		Name:       "lib:apache2",
		Regex:      `^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^ ]*) +\S*)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>.*)")?$`,
		TimeKey:    "time",
		TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
	},
	&ParserRegex{
		Name:  "lib:apache_error",
		Regex: `^\[[^ ]* (?<time>[^\]]*)\] \[(?<level>[^\]]*)\](?: \[pid (?<pid>[^\]]*)\])?( \[client (?<client>[^\]]*)\])? (?<message>.*)$`,
	},
	&ParserRegex{
		Name:       "lib:mongodb",
		Regex:      `^(?<time>[^ ]*)\s+(?<severity>\w)\s+(?<component>[^ ]+)\s+\[(?<context>[^\]]+)]\s+(?<message>.*?) *(?<ms>(\d+))?(:?ms)?$`,
		TimeKey:    "time",
		TimeFormat: "%Y-%m-%dT%H:%M:%S.%L",
	},
	&ParserRegex{
		Name:       "lib:nginx",
		Regex:      `^(?<remote>[^ ]*) (?<host>[^ ]*) (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")`,
		TimeKey:    "time",
		TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
	},
	&ParserRegex{
		Name:       "lib:syslog-rfc5424",
		Regex:      `^\<(?<pri>[0-9]{1,5})\>1 (?<time>[^ ]+) (?<host>[^ ]+) (?<ident>[^ ]+) (?<pid>[-0-9]+) (?<msgid>[^ ]+) (?<extradata>(\[(.*?)\]|-)) (?<message>.+)$`,
		TimeKey:    "time",
		TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%Z",
	},
	&ParserRegex{
		Name:       "lib:syslog-rfc3164",
		Regex:      `/^\<(?<pri>[0-9]+)\>(?<time>[^ ]* {1,2}[^ ]* [^ ]*) (?<host>[^ ]*) (?<ident>[a-zA-Z0-9_\/\.\-]*)(?:\[(?<pid>[0-9]+)\])?(?:[^\:]*\:)? *(?<message>.*)$/`,
		TimeKey:    "time",
		TimeFormat: "%b %d %H:%M:%S",
	},
}

type Config struct {
	StateDir  string
	LogsDir   string
	Inputs    []Input
	Filters   []Filter
	Outputs   []Output
	Parsers   []Parser
	UserAgent string
}

func (c Config) generateMain() (string, error) {
	var mainConfigBuilder strings.Builder
	if err := mainConfTemplate.Execute(&mainConfigBuilder, c); err != nil {
		return "", err
	}
	return mainConfigBuilder.String(), nil
}

func (c Config) generateParser() (string, error) {
	parsers := Config{
		Parsers: append(DefaultParsers, c.Parsers...),
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

type Filter interface {
	Generate() (string, error)
}

// A FilterParserGroup represents a list of filters to be applied in order.
type FilterParserGroup []*FilterParser

func (g FilterParserGroup) Generate() (string, error) {
	var filters []string
	for _, f := range g {
		configSection, err := f.Generate()
		if err != nil {
			return "", err
		}
		filters = append(filters, configSection)
	}
	return strings.Join(filters, "\n\n"), nil
}

// A FilterParser represents the configuration data for fluentBit's filter parser plugin.
type FilterParser struct {
	Match   string
	KeyName string
	Parser  string
}

func (f FilterParser) Generate() (string, error) {
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

func (f FilterModifyAddLogName) Generate() (string, error) {
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

func (f FilterModifyRemoveLogName) Generate() (string, error) {
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

func (f FilterRewriteTag) Generate() (string, error) {
	var renderedFilterRewriteTagConfig strings.Builder
	if err := mainConfTemplate.ExecuteTemplate(&renderedFilterRewriteTagConfig, "filter_rewrite_tag", f); err != nil {
		return "", err
	}
	return renderedFilterRewriteTagConfig.String(), nil
}

type Parser interface {
	Generate() (string, error)
}

// A ParserJSON represents the configuration data for fluentBit's JSON parser.
type ParserJSON struct {
	Name       string
	TimeKey    string
	TimeFormat string
}

func (p ParserJSON) Generate() (string, error) {
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

func (p ParserRegex) Generate() (string, error) {
	var b strings.Builder
	if err := parserConfTemplate.ExecuteTemplate(&b, "parserRegex", p); err != nil {
		return "", err
	}
	return b.String(), nil
}

type Input interface {
	Generate() (string, error)
}

// A Tail represents the configuration data for fluentBit's tail input plugin.
type Tail struct {
	Tag          string
	IncludePaths []string
	ExcludePaths []string
}

func (t Tail) Generate() (string, error) {
	var renderedTailConfig strings.Builder
	if err := mainConfTemplate.ExecuteTemplate(&renderedTailConfig, "tail", t); err != nil {
		return "", err
	}
	return renderedTailConfig.String(), nil
}

// A Syslog represents the configuration data for fluentBit's syslog input plugin.
type Syslog struct {
	Tag    string
	Mode   string
	Listen string
	Port   uint16
}

func (s Syslog) Generate() (string, error) {
	var renderedSyslogConfig strings.Builder
	if err := mainConfTemplate.ExecuteTemplate(&renderedSyslogConfig, "syslog", s); err != nil {
		return "", err
	}
	return renderedSyslogConfig.String(), nil
}

// A WindowsEventlog represents the configuration data for fluentbit's winlog input plugin
type WindowsEventlog struct {
	Tag          string
	Channels     []string
	Interval_Sec string // XXX: stop ignoring?
}

func (w WindowsEventlog) Generate() (string, error) {
	var renderedWineventlogConfig strings.Builder
	if err := mainConfTemplate.ExecuteTemplate(&renderedWineventlogConfig, "wineventlog", w); err != nil {
		return "", err
	}
	return renderedWineventlogConfig.String(), nil
}

type Output interface {
	Generate() (string, error)
}

// A Stackdriver represents the configurable data for fluentBit's stackdriver output plugin.
type Stackdriver struct {
	Match     string
	UserAgent string
	Workers   int
}

func (s Stackdriver) Generate() (string, error) {
	var renderedStackdriverConfig strings.Builder
	if err := mainConfTemplate.ExecuteTemplate(&renderedStackdriverConfig, "stackdriver", s); err != nil {
		return "", err
	}
	return renderedStackdriverConfig.String(), nil
}
