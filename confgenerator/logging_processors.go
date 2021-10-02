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
		fluentbit.FilterComponent(tag, p.Field, []string{parserName}),
		parser,
	}
}

func init() {
	LoggingProcessorTypes.RegisterType(func() Component { return &LoggingProcessorParseJson{} })
}

// A LoggingProcessorParseRegexSimple applies a regex to the specified field, storing the named capture groups as keys in the log record.
// This was maintained in addition to the parse_regex_multiple_formats to ensure backward compatibility with any existing configurations
type LoggingProcessorParseRegexSimple struct {
	ConfigComponent        `yaml:",inline"`
	fluentbit.ParserShared `yaml:",inline"`
	Field                  string `yaml:"field,omitempty"`

	Regex string `yaml:"regex,omitempty" validate:"required"`
}

func (r LoggingProcessorParseRegexSimple) Type() string {
	return "parse_regex"
}

func (p LoggingProcessorParseRegexSimple) Components(tag, uid string) []fluentbit.Component {
	parser, parserName := p.ParserShared.Component(tag, uid)
	parser.Config["Format"] = "regex"
	parser.Config["Regex"] = p.Regex

	return []fluentbit.Component{
		parser,
		fluentbit.FilterComponent(tag, p.Field, []string{parserName}),
	}
}

type RegexParser struct {
	Regex  string                 `yaml:"regex,omitempty" validate:"required"`
	Parser fluentbit.ParserShared `yaml:",inline"`
}

// A LoggingProcessorParseRegex applies a set of regexes to the specified field, storing the named capture groups as keys in the log record.
type LoggingProcessorParseRegex struct {
	ConfigComponent `yaml:",inline"`
	Field           string `yaml:"field,omitempty"`

	Parsers []RegexParser `yaml:"parsers,omitempty" validate:"required"`
}

func (r LoggingProcessorParseRegex) Type() string {
	return "parse_regex_multiple_formats"
}

func (p LoggingProcessorParseRegex) Components(tag, uid string) []fluentbit.Component {
	components := []fluentbit.Component{}
	parserNames := []string{}

	for idx, parserConfig := range p.Parsers {
		parser, parserName := parserConfig.Parser.Component(tag, fmt.Sprintf("%s.%d", uid, idx))
		parser.Config["Format"] = "regex"
		parser.Config["Regex"] = parserConfig.Regex
		components = append(components, parser)
		parserNames = append(parserNames, parserName)
	}

	components = append(components, fluentbit.FilterComponent(tag, p.Field, parserNames))
	return components
}

type MultilineRule struct {
	StateName string `yaml:"state_name" validate:"required"`
	Regex     string `yaml:"regex,omitempty" validate:"required"`
	NextState string `yaml:"next_state" validate:"required"`
}

func (r MultilineRule) AsString() string {
	return fmt.Sprintf(`"%s"    "%s"    "%s"`, r.StateName, r.Regex, r.NextState)
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
	LoggingProcessorParseRegex `yaml:",inline"`

	Rules []MultilineRule `yaml:"rules" validate:"required"`
}

func (r LoggingProcessorParseMultilineRegex) Type() string {
	return "parse_multiline_regex"
}

func (p LoggingProcessorParseMultilineRegex) Components(tag, uid string) []fluentbit.Component {
	multilineParserName := fmt.Sprintf("%s.%s.multiline", tag, uid)
	rules := []string{}
	for _, rule := range p.Rules {
		rules = append(rules, rule.AsString())
	}

	multilineComponents := []fluentbit.Component{
		fluentbit.Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":                  "multiline",
				"Match":                 tag,
				"Multiline.Key_Content": p.Field,
				"Multiline.Parser":      multilineParserName,
			},
		},
		fluentbit.Component{
			Kind: "MULTILINE_PARSER",
			Config: map[string]string{
				"Name": multilineParserName,
				"Type": "regex",
			},
			RepeatedConfig: map[string][]string{
				"rule": rules,
			},
		},
	}

	return append(multilineComponents, p.LoggingProcessorParseRegex.Components(tag, uid)...)
}

func init() {
	LoggingProcessorTypes.RegisterType(func() Component { return &LoggingProcessorParseRegexSimple{} })
	LoggingProcessorTypes.RegisterType(func() Component { return &LoggingProcessorParseRegex{} })
	LoggingProcessorTypes.RegisterType(func() Component { return &LoggingProcessorParseMultilineRegex{} })
}

var LegacyBuiltinProcessors = map[string]LoggingProcessor{
	"lib:default_message_parser": &LoggingProcessorParseRegexSimple{
		Regex: `^(?<message>.*)$`,
	},
	"lib:apache": &LoggingProcessorParseRegexSimple{
		Regex: `^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")?$`,
		ParserShared: fluentbit.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
		},
	},
	"lib:apache2": &LoggingProcessorParseRegexSimple{
		Regex: `^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^ ]*) +\S*)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>.*)")?$`,
		ParserShared: fluentbit.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
		},
	},
	"lib:apache_error": &LoggingProcessorParseRegexSimple{
		Regex: `^\[[^ ]* (?<time>[^\]]*)\] \[(?<level>[^\]]*)\](?: \[pid (?<pid>[^\]]*)\])?( \[client (?<client>[^\]]*)\])? (?<message>.*)$`,
	},
	"lib:mongodb": &LoggingProcessorParseRegexSimple{
		Regex: `^(?<time>[^ ]*)\s+(?<severity>\w)\s+(?<component>[^ ]+)\s+\[(?<context>[^\]]+)]\s+(?<message>.*?) *(?<ms>(\d+))?(:?ms)?$`,
		ParserShared: fluentbit.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L",
		},
	},
	"lib:nginx": &LoggingProcessorParseRegexSimple{
		Regex: `^(?<remote>[^ ]*) (?<host>[^ ]*) (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")`,
		ParserShared: fluentbit.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
		},
	},
	"lib:syslog-rfc5424": &LoggingProcessorParseRegexSimple{
		Regex: `^\<(?<pri>[0-9]{1,5})\>1 (?<time>[^ ]+) (?<host>[^ ]+) (?<ident>[^ ]+) (?<pid>[-0-9]+) (?<msgid>[^ ]+) (?<extradata>(\[(.*?)\]|-)) (?<message>.+)$`,
		ParserShared: fluentbit.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%Z",
		},
	},
	"lib:syslog-rfc3164": &LoggingProcessorParseRegexSimple{
		Regex: `/^\<(?<pri>[0-9]+)\>(?<time>[^ ]* {1,2}[^ ]* [^ ]*) (?<host>[^ ]*) (?<ident>[a-zA-Z0-9_\/\.\-]*)(?:\[(?<pid>[0-9]+)\])?(?:[^\:]*\:)? *(?<message>.*)$/`,
		ParserShared: fluentbit.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%b %d %H:%M:%S",
		},
	},
}
