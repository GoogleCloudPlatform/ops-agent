// Copyright 2021 Google LLC
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

package confgenerator

import (
	"fmt"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

// A LoggingProcessorParseJson parses the specified field as JSON.
type LoggingProcessorParseJson struct {
	ConfigComponent        `yaml:",inline"`
	fluentbit.ParserShared `yaml:",inline"`
	Field                  string `yaml:"field,omitempty"`
}

func (r LoggingProcessorParseJson) Type() string {
	return "parse_json"
}

func (p LoggingProcessorParseJson) Components(tag, uid string) []fluentbit.Component {
	parser, parserName := p.ParserShared.Component(tag, uid)
	parser.Config["Format"] = "json"
	return []fluentbit.Component{
		fluentbit.ParserFilterComponent(tag, p.Field, []string{parserName}),
		parser,
	}
}

func init() {
	LoggingProcessorTypes.RegisterType(func() Component { return &LoggingProcessorParseJson{} })
}

// A LoggingProcessorParseRegex applies a regex to the specified field, storing the named capture groups as keys in the log record.
// This was maintained in addition to the parse_regex_complex to ensure backward compatibility with any existing configurations
type LoggingProcessorParseRegex struct {
	ConfigComponent        `yaml:",inline"`
	fluentbit.ParserShared `yaml:",inline"`
	Field                  string `yaml:"field,omitempty"`

	Regex string `yaml:"regex,omitempty" validate:"required"`
}

func (r LoggingProcessorParseRegex) Type() string {
	return "parse_regex"
}

func (p LoggingProcessorParseRegex) Components(tag, uid string) []fluentbit.Component {
	parser, parserName := p.ParserShared.Component(tag, uid)
	parser.Config["Format"] = "regex"
	parser.Config["Regex"] = p.Regex

	return []fluentbit.Component{
		parser,
		fluentbit.ParserFilterComponent(tag, p.Field, []string{parserName}),
	}
}

type RegexParser struct {
	Regex  string
	Parser fluentbit.ParserShared
}

// A LoggingProcessorParseRegexComplex applies a set of regexes to the specified field, storing the named capture groups as keys in the log record.
type LoggingProcessorParseRegexComplex struct {
	ConfigComponent
	Field string

	Parsers []RegexParser
}

func (p LoggingProcessorParseRegexComplex) Components(tag, uid string) []fluentbit.Component {
	components := []fluentbit.Component{}
	parserNames := []string{}

	for idx, parserConfig := range p.Parsers {
		parser, parserName := parserConfig.Parser.Component(tag, fmt.Sprintf("%s.%d", uid, idx))
		parser.Config["Format"] = "regex"
		parser.Config["Regex"] = parserConfig.Regex
		components = append(components, parser)
		parserNames = append(parserNames, parserName)
	}

	components = append(components, fluentbit.ParserFilterComponent(tag, p.Field, parserNames))
	return components
}

type MultilineRule struct {
	StateName string
	Regex     string
	NextState string
}

func (r MultilineRule) AsString() string {
	escapedRegex := strings.ReplaceAll(r.Regex, `"`, `\"`)
	return fmt.Sprintf(`"%s"    "%s"    "%s"`, r.StateName, escapedRegex, r.NextState)
}

// A LoggingProcessorParseMultiline applies a set of regex rules to the specified lines, storing the named capture groups as keys in the log record.
//     #
//     # Regex rules for multiline parsing
//     # ---------------------------------
//     #
//     # configuration hints:
//     #
//     #  - first state always has the name: start_state
//     #  - every field in the rule must be inside double quotes
//     #
//     # rules |   state name  | regex pattern                  | next state
//     # ------|---------------|--------------------------------------------
//     rule      "start_state"   "/(Dec \d+ \d+\:\d+\:\d+)(.*)/"  "cont"
//     rule      "cont"          "/^\s+at.*/"                     "cont"
type LoggingProcessorParseMultilineRegex struct {
	LoggingProcessorParseRegexComplex

	Rules []MultilineRule
}

func (p LoggingProcessorParseMultilineRegex) Components(tag, uid string) []fluentbit.Component {
	multilineParserName := fmt.Sprintf("%s.%s.multiline", tag, uid)
	rules := []string{}
	for _, rule := range p.Rules {
		rules = append(rules, rule.AsString())
	}

	filter := fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":                  "multiline",
			"Match":                 tag,
			"Multiline.Key_Content": "message",
			"Multiline.Parser":      multilineParserName,
		},
	}

	if p.Field != "" {
		filter.Config["Multiline.Key_Content"] = p.Field
	}

	multilineParser := fluentbit.Component{
		Kind: "MULTILINE_PARSER",
		Config: map[string]string{
			"Name": multilineParserName,
			"Type": "regex",
		},
		RepeatedConfig: map[string][]string{
			"rule": rules,
		},
	}

	return append([]fluentbit.Component{filter, multilineParser}, p.LoggingProcessorParseRegexComplex.Components(tag, uid)...)
}

func init() {
	LoggingProcessorTypes.RegisterType(func() Component { return &LoggingProcessorParseRegex{} })
}

var LegacyBuiltinProcessors = map[string]LoggingProcessor{
	"lib:default_message_parser": &LoggingProcessorParseRegex{
		Regex: `^(?<message>.*)$`,
	},
	"lib:apache": &LoggingProcessorParseRegex{
		Regex: `^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")?$`,
		ParserShared: fluentbit.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
		},
	},
	"lib:apache2": &LoggingProcessorParseRegex{
		Regex: `^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^ ]*) +\S*)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>.*)")?$`,
		ParserShared: fluentbit.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
		},
	},
	"lib:apache_error": &LoggingProcessorParseRegex{
		Regex: `^\[[^ ]* (?<time>[^\]]*)\] \[(?<level>[^\]]*)\](?: \[pid (?<pid>[^\]]*)\])?( \[client (?<client>[^\]]*)\])? (?<message>.*)$`,
	},
	"lib:mongodb": &LoggingProcessorParseRegex{
		Regex: `^(?<time>[^ ]*)\s+(?<severity>\w)\s+(?<component>[^ ]+)\s+\[(?<context>[^\]]+)]\s+(?<message>.*?) *(?<ms>(\d+))?(:?ms)?$`,
		ParserShared: fluentbit.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L",
		},
	},
	"lib:nginx": &LoggingProcessorParseRegex{
		Regex: `^(?<remote>[^ ]*) (?<host>[^ ]*) (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")`,
		ParserShared: fluentbit.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
		},
	},
	"lib:syslog-rfc5424": &LoggingProcessorParseRegex{
		Regex: `^\<(?<pri>[0-9]{1,5})\>1 (?<time>[^ ]+) (?<host>[^ ]+) (?<ident>[^ ]+) (?<pid>[-0-9]+) (?<msgid>[^ ]+) (?<extradata>(\[(.*?)\]|-)) (?<message>.+)$`,
		ParserShared: fluentbit.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%Z",
		},
	},
	"lib:syslog-rfc3164": &LoggingProcessorParseRegex{
		Regex: `/^\<(?<pri>[0-9]+)\>(?<time>[^ ]* {1,2}[^ ]* [^ ]*) (?<host>[^ ]*) (?<ident>[a-zA-Z0-9_\/\.\-]*)(?:\[(?<pid>[0-9]+)\])?(?:[^\:]*\:)? *(?<message>.*)$/`,
		ParserShared: fluentbit.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%b %d %H:%M:%S",
		},
	},
}
