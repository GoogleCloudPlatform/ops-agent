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

//Package conf provides data structures to represent and generate fluentBit configuration.
package conf

import (
	"fmt"
	"net"
	"strings"
	"text/template"
)

const (
	mainConfTemplate = `[SERVICE]
    Flush         5
    Grace         120
    Log_Level     debug
    Log_File      /var/log/ops_agents/logging_agent.log
    Daemon        off
    HTTP_Server   On
    HTTP_Listen   0.0.0.0

[OUTPUT]
    Name  stackdriver
    resource gce_instance
    Match *

{{range .TailConfigSections -}}
{{.}}

{{end}}
{{- range .SyslogConfigSections -}}
{{.}}

{{end}}`

	parserConfTemplate = `[PARSER]
    Name        syslog-rfc5424
    Format      regex
    Regex       ^\<(?<pri>[0-9]{1,5})\>1 (?<time>[^ ]+) (?<host>[^ ]+) (?<ident>[^ ]+) (?<pid>[-0-9]+) (?<msgid>[^ ]+) (?<extradata>(\[(.*?)\]|-)) (?<message>.+)$
    Time_Key    time
    Time_Format %Y-%m-%dT%H:%M:%S.%L%z

[PARSER]
    Name        syslog-rfc3164
    Format      regex
    Regex       /^\<(?<pri>[0-9]+)\>(?<time>[^ ]* {1,2}[^ ]* [^ ]*) (?<host>[^ ]*) (?<ident>[a-zA-Z0-9_\/\.\-]*)(?:\[(?<pid>[0-9]+)\])?(?:[^\:]*\:)? *(?<message>.*)$/
    Time_Key    time
    Time_Format %b %d %H:%M:%S

{{range .JSONParserConfigSections -}}
{{.}}

{{end}}
{{- range .RegexParserConfigSections -}}
{{.}}

{{end}}`

	parserJSONConf = `[PARSER]
    Name {{.Name}}
    Format json
{{- if (ne .TimeKey "")}}
    Time_Key {{.TimeKey}}
{{- end}}
{{- if (ne .TimeFormat "")}}
    Time_Format {{.TimeFormat}}
{{- end}}`

	parserRegexConf = `[PARSER]
    Name {{.Name}}
    Format regex
    Regex {{.Regex}}
{{- if (ne .TimeKey "")}}
    Time_Key {{.TimeKey}}
{{- end}}
{{- if (ne .TimeFormat "")}}
    Time_Format {{.TimeFormat}}
{{- end}}`

	tailConf = `[INPUT]
    Name tail
    DB {{.DB}}
    Path {{.Path}}
    Tag {{.Tag}}
    Buffer_Chunk_Size 32k
    Buffer_Max_Size 32k
    DB.Sync Full
    Refresh_Interval 60
    Rotate_Wait 5
    Skip_Long_Lines On
    Key log
{{- if (ne .ExcludePath "")}}
    Exclude_Path {{.ExcludePath}}
{{- end}}
{{- if (ne .Parser "")}}
    Parser {{.Parser}}
{{- end}}`

	syslogConf = `[INPUT]
    Name syslog
    Mode {{.Mode}}
    Listen {{.Listen}}
    Tag {{.Tag}}
    Port {{.Port}}
    Parser {{.Parser}}`
)

type mainConfigSections struct {
	TailConfigSections   []string
	SyslogConfigSections []string
}

type parserConfigSections struct {
	JSONParserConfigSections  []string
	RegexParserConfigSections []string
}

// GenerateFluentBitMainConfig generates a FluentBit main configuration.
func GenerateFluentBitMainConfig(tails []*Tail, syslogs []*Syslog) (string, error) {
	tailConfigSections := []string{}
	syslogConfigSections := []string{}
	for _, t := range tails {
		configSection, err := t.renderConfig()
		if err != nil {
			return "", err
		}
		tailConfigSections = append(tailConfigSections, configSection)
	}
	for _, s := range syslogs {
		configSection, err := s.renderConfig()
		if err != nil {
			return "", err
		}
		syslogConfigSections = append(syslogConfigSections, configSection)
	}
	configSections := mainConfigSections{
		TailConfigSections:   tailConfigSections,
		SyslogConfigSections: syslogConfigSections,
	}
	mt, err := template.New("fluentBitMainConf").Parse(mainConfTemplate)
	if err != nil {
		return "", err
	}

	var mainConfigBuilder strings.Builder
	if err := mt.Execute(&mainConfigBuilder, configSections); err != nil {
		return "", err
	}
	return mainConfigBuilder.String(), nil
}

// GenerateFluentBitParserConfig generates a FluentBit parser configuration.
func GenerateFluentBitParserConfig(jsonParsers []*ParserJSON, regexParsers []*ParserRegex) (string, error) {
	jsonParserConfigSections := []string{}
	for _, j := range jsonParsers {
		configSection, err := j.renderConfig()
		if err != nil {
			return "", err
		}
		jsonParserConfigSections = append(jsonParserConfigSections, configSection)
	}

	regexParserConfigSections := []string{}
	for _, r := range regexParsers {
		configSection, err := r.renderConfig()
		if err != nil {
			return "", err
		}
		regexParserConfigSections = append(regexParserConfigSections, configSection)
	}

	var parserConfigBuilder strings.Builder
	parserTemplate, err := template.New("fluentBitParserConf").Parse(parserConfTemplate)
	if err != nil {
		return "", err
	}
	parsers := parserConfigSections{
		JSONParserConfigSections:  jsonParserConfigSections,
		RegexParserConfigSections: regexParserConfigSections,
	}
	if err := parserTemplate.Execute(&parserConfigBuilder, parsers); err != nil {
		return "", err
	}
	return parserConfigBuilder.String(), nil
}

type emptyFieldErr struct {
	plugin string
	field  string
}

func (e emptyFieldErr) Error() string {
	return fmt.Sprintf("%q plugin should not have empty field: %q", e.plugin, e.field)
}

type nonPositiveFieldErr struct {
	plugin string
	field  string
}

func (e nonPositiveFieldErr) Error() string {
	return fmt.Sprintf("%q plugin's field %q should not be <= 0", e.plugin, e.field)
}

// A ParserJSON represents the configuration data for fluentBit's JSON parser.
type ParserJSON struct {
	Name       string
	TimeKey    string
	TimeFormat string
}

var parserJSONTemplate = template.Must(template.New("parserJSON").Parse(parserJSONConf))

// renderConfig generates a section for configure fluentBit JSON parser.
func (p ParserJSON) renderConfig() (string, error) {
	if p.Name == "" {
		return "", emptyFieldErr{
			plugin: "json parser",
			field:  "name",
		}
	}
	var b strings.Builder
	if err := parserJSONTemplate.Execute(&b, p); err != nil {
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

var parserRegexTemplate = template.Must(template.New("parserRegex").Parse(parserRegexConf))

// renderConfig generates a section for configure fluentBit Regex parser.
func (p ParserRegex) renderConfig() (string, error) {
	if p.Name == "" {
		return "", emptyFieldErr{
			plugin: "regex parser",
			field:  "name",
		}
	}
	if p.Regex == "" {
		return "", emptyFieldErr{
			plugin: "regex parser",
			field:  "regex",
		}
	}
	var b strings.Builder
	if err := parserRegexTemplate.Execute(&b, p); err != nil {
		return "", err
	}
	return b.String(), nil
}

// A Tail represents the configuration data for fluentBit's tail input plugin.
type Tail struct {
	Tag         string
	Path        string
	ExcludePath string
	Parser      string
	DB          string
}

// renderConfig generates a section for configure fluentBit tail parser.
func (t Tail) renderConfig() (string, error) {
	if t.Tag == "" {
		return "", emptyFieldErr{
			plugin: "tail",
			field:  "Tag",
		}
	}
	if t.Path == "" {
		return "", emptyFieldErr{
			plugin: "tail",
			field:  "Path",
		}
	}
	if t.DB == "" {
		return "", emptyFieldErr{
			plugin: "tail",
			field:  "DB",
		}
	}
	var renderedTailConfig strings.Builder
	if err := tailTemplate.Execute(&renderedTailConfig, t); err != nil {
		return "", err
	}
	return renderedTailConfig.String(), nil
}

var tailTemplate = template.Must(template.New("tail").Parse(tailConf))

type invalidValueErr struct {
	plugin                string
	field                 string
	validValueExplanation string
}

func (e invalidValueErr) Error() string {
	return fmt.Sprintf("got invalid value for %q plugin's field %q, should be %s", e.plugin, e.field, e.validValueExplanation)
}

// A Syslog represents the configuration data for fluentBit's syslog input plugin.
type Syslog struct {
	Mode   string
	Listen string
	Port   uint16
	Parser string
	Tag    string
}

var syslogTemplate = template.Must(template.New("syslog").Parse(syslogConf))

// renderConfig generates a section for configure fluentBit syslog input plugin.
func (s Syslog) renderConfig() (string, error) {
	switch m := s.Mode; m {
	case "tcp", "udp":
		if net.ParseIP(s.Listen) == nil {
			return "", invalidValueErr{
				plugin:                "syslog",
				field:                 "Listen",
				validValueExplanation: "a valid IP",
			}
		}
		if s.Port == 0 {
			return "", emptyFieldErr{
				plugin: "syslog",
				field:  "Port",
			}
		}
	case "unix_tcp", "unix_udp":
		// TODO: pending decision on setting up unix_tcp, unix_udp
		fallthrough
	default:
		return "", invalidValueErr{
			plugin:                "syslog",
			field:                 "Mode",
			validValueExplanation: "one of \"tcp\", \"udp\"",
		}
	}
	if s.Parser != "syslog-rfc5424" && s.Parser != "syslog-rfc3164" {
		return "", invalidValueErr{
			plugin:                "syslog",
			field:                 "Parser",
			validValueExplanation: "one of \"syslog-rfc5424\", \"syslog-rfc3164\"",
		}
	}
	if s.Tag == "" {
		return "", emptyFieldErr{
			plugin: "syslog",
			field:  "Tag",
		}
	}

	var renderedSyslogConfig strings.Builder
	if err := syslogTemplate.Execute(&renderedSyslogConfig, s); err != nil {
		return "", err
	}
	return renderedSyslogConfig.String(), nil
}
