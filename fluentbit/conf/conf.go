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
    # https://docs.fluentbit.io/manual/administration/configuring-fluent-bit/configuration-file#config_section
    # Flush logs every 1 second, even if the buffer is not full to minimize log entry arrival delay.
    Flush      1
    # Waits 120 seconds after receiving a SIGTERM before it shuts down to minimize log loss.
    Grace      120
    # We use systemd to manage Fluent Bit instead.
    Daemon     off
    # Log_File is set by Fluent Bit systemd unit (e.g. /var/log/google-cloud-ops-agent/subagents/fluent-bit.log).
    Log_Level  info

    # https://docs.fluentbit.io/manual/administration/monitoring
    # Enable a built-in HTTP server that can be used to query internal information and monitor metrics of each running plugin.
    HTTP_Server  On
    HTTP_Listen  0.0.0.0
    HTTP_PORT    2020

    # https://docs.fluentbit.io/manual/administration/buffering-and-storage#service-section-configuration
    # storage.path is set by Fluent Bit systemd unit (e.g. /var/lib/google-cloud-ops-agent/buffers/fluent-bit).
    storage.sync               normal
    # Enable the data integrity check when writing and reading data from the filesystem.
    storage.checksum           on
    # The maximum amount of data to load into the memory when processing old chunks from the backlog that is from previous Fluent Bit processes (e.g. Fluent Bit may have crashed or restarted).
    storage.backlog.mem_limit  50M
    # Enable storage metrics in the built-in HTTP server.
    storage.metrics            on

{{range .TailConfigSections -}}
{{.}}

{{end}}
{{- range .SyslogConfigSections -}}
{{.}}

{{end}}
{{- range .FilterParserConfigSections -}}
{{.}}

{{end}}

{{- range .StackdriverConfigSections -}}
{{.}}

{{end}}`

	parserConfTemplate = `[PARSER]
    Name        default_message_parser
    Format      regex
    Regex       ^(?<message>.*)$

[PARSER]
    Name   apache
    Format regex
    Regex  ^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")?$
    Time_Key time
    Time_Format %d/%b/%Y:%H:%M:%S %z

[PARSER]
    Name   apache2
    Format regex
    Regex  ^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^ ]*) +\S*)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>.*)")?$
    Time_Key time
    Time_Format %d/%b/%Y:%H:%M:%S %z

[PARSER]
    Name   apache_error
    Format regex
    Regex  ^\[[^ ]* (?<time>[^\]]*)\] \[(?<level>[^\]]*)\](?: \[pid (?<pid>[^\]]*)\])?( \[client (?<client>[^\]]*)\])? (?<message>.*)$

[PARSER]
    Name    mongodb
    Format  regex
    Regex   ^(?<time>[^ ]*)\s+(?<severity>\w)\s+(?<component>[^ ]+)\s+\[(?<context>[^\]]+)]\s+(?<message>.*?) *(?<ms>(\d+))?(:?ms)?$
    Time_Key time
    Time_Format %Y-%m-%dT%H:%M:%S.%L

[PARSER]
    Name   nginx
    Format regex
    Regex ^(?<remote>[^ ]*) (?<host>[^ ]*) (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")
    Time_Key time
    Time_Format %d/%b/%Y:%H:%M:%S %z

[PARSER]
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

	filterParserConf = `[FILTER]
    Name parser
    Match {{.Match}}
    Key_Name {{.KeyName}}
    Parser {{.Parser}}`

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
    Key message
{{- if (ne .ExcludePath "")}}
    Exclude_Path {{.ExcludePath}}
{{- end}}`

	syslogConf = `[INPUT]
    Name syslog
    Mode {{.Mode}}
    Listen {{.Listen}}
    Tag {{.Tag}}
    Port {{.Port}}
    Parser default_message_parser`

	stackdriverConf = `[OUTPUT]
    Name stackdriver
    resource gce_instance
    Match {{.Match}}`
)

type mainConfigSections struct {
	TailConfigSections         []string
	SyslogConfigSections       []string
	FilterParserConfigSections []string
	StackdriverConfigSections  []string
}

type parserConfigSections struct {
	JSONParserConfigSections  []string
	RegexParserConfigSections []string
}

// GenerateFluentBitMainConfig generates a FluentBit main configuration.
func GenerateFluentBitMainConfig(tails []*Tail, syslogs []*Syslog, filterParsers []*FilterParser, stackdrivers []*Stackdriver) (string, error) {
	tailConfigSections := []string{}
	syslogConfigSections := []string{}
	filterParserConfigSections := []string{}
	stackdriverConfigSections := []string{}
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
	for _, f := range filterParsers {
		configSection, err := f.renderConfig()
		if err != nil {
			return "", err
		}
		filterParserConfigSections = append(filterParserConfigSections, configSection)
	}
	for _, s := range stackdrivers {
		configSection, err := s.renderConfig()
		if err != nil {
			return "", err
		}
		stackdriverConfigSections = append(stackdriverConfigSections, configSection)
	}
	configSections := mainConfigSections{
		TailConfigSections:         tailConfigSections,
		SyslogConfigSections:       syslogConfigSections,
		FilterParserConfigSections: filterParserConfigSections,
		StackdriverConfigSections:  stackdriverConfigSections,
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

type FilterParser struct {
	Match   string
	KeyName string
	Parser  string
}

var filterParserTemplate = template.Must(template.New("filter_parser").Parse(filterParserConf))

func (f FilterParser) renderConfig() (string, error) {
	if f.Match == "" {
		return "", emptyFieldErr{
			plugin: "filter parser",
			field:  "Match",
		}
	}
	if f.KeyName == "" {
		return "", emptyFieldErr{
			plugin: "filter parser",
			field:  "KeyName",
		}
	}
	if f.Parser == "" {
		return "", emptyFieldErr{
			plugin: "filter parser",
			field:  "Parser",
		}
	}
	var renderedFilterParserConfig strings.Builder
	if err := filterParserTemplate.Execute(&renderedFilterParserConfig, f); err != nil {
		return "", err
	}
	return renderedFilterParserConfig.String(), nil
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

// A Stackdriver represents the configurable data for fluentBit's stackdriver output plugin.
type Stackdriver struct {
	Match string
}

var stackdriverTemplate = template.Must(template.New("stackdriver").Parse(stackdriverConf))

// renderConfig generates a section for configure fluentBit syslog input plugin.
func (s Stackdriver) renderConfig() (string, error) {
	if s.Match == "" {
		return "", emptyFieldErr{
			plugin: "stackdriver",
			field:  "Match",
		}
	}
	var renderedStackdriverConfig strings.Builder
	if err := stackdriverTemplate.Execute(&renderedStackdriverConfig, s); err != nil {
		return "", err
	}
	return renderedStackdriverConfig.String(), nil
}
