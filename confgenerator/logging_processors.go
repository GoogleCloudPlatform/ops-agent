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
	ConfigComponent             `yaml:",inline"`
	LoggingProcessorParseShared `yaml:",inline"`
}

func (r LoggingProcessorParseJson) Type() string {
	return "parse_json"
}

func (p LoggingProcessorParseJson) Components(tag, uid string) []fluentbit.Component {
	filter, parser := p.LoggingProcessorParseShared.Components(tag, uid)
	parser.Config["Format"] = "json"
	return []fluentbit.Component{
		filter,
		parser,
	}
}

func init() {
	LoggingProcessorTypes.RegisterType(func() Component { return &LoggingProcessorParseJson{} })
}

// A LoggingProcessorParseRegex applies a regex to the specified field, storing the named capture groups as keys in the log record.
type LoggingProcessorParseRegex struct {
	ConfigComponent             `yaml:",inline"`
	LoggingProcessorParseShared `yaml:",inline"`

	Regex string `yaml:"regex,omitempty" validate:"required"`
}

func (r LoggingProcessorParseRegex) Type() string {
	return "parse_regex"
}

func (p LoggingProcessorParseRegex) Components(tag, uid string) []fluentbit.Component {
	filter, parser := p.LoggingProcessorParseShared.Components(tag, uid)
	parser.Config["Format"] = "regex"
	parser.Config["Regex"] = p.Regex
	return []fluentbit.Component{
		filter,
		parser,
	}
}

type MultilineRule struct {
	StateName string `yaml:"state_name" validate:"required"`
	Regex     string `yaml:"regex,omitempty" validate:"required"`
	NextState string `yaml:"next_state" validate:"required"`
}

func (r MultilineRule) AsString() string {
	return fmt.Sprintf(`rule    "%s"    "%s"    "%s"`, r.StateName, r.Regex, r.NextState)
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
type LoggingProcessorParseMultiline struct {
	ConfigComponent             `yaml:",inline"`
	LoggingProcessorParseShared `yaml:",inline"`

	Rules []MultilineRule `yaml:"rules" validate:"required"`
	Regex string          `yaml:"regex,omitempty" validate:"required"`
}

func (r LoggingProcessorParseMultiline) Type() string {
	return "parse_regex"
}

func (p LoggingProcessorParseMultiline) Components(tag, uid string) []fluentbit.Component {
	multilineParserName := fmt.Sprintf("%s.%s.multiline", tag, uid)
	rulesText := []string{}
	for _, rule := range p.Rules {
		rulesText = append(rulesText, rule.AsString())
	}

	multilineComponents := []fluentbit.Component{
		fluentbit.Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":                  "multiline",
				"Match":                 tag,
				"Multiline.Key_Content": "message",
				"Multiline.Parser":      multilineParserName,
			},
		},
		fluentbit.Component{
			Kind: "MULTILINE_PARSER",
			Config: map[string]string{
				"Name": multilineParserName,
				"Type": "regex",
			},
			AdditionalConfig: rulesText,
		},
	}

	filter, parser := p.LoggingProcessorParseShared.Components(tag, uid)
	parser.Config["Format"] = "regex"
	parser.Config["Regex"] = p.Regex
	return append(multilineComponents, filter, parser)
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
		LoggingProcessorParseShared: LoggingProcessorParseShared{
			TimeKey:    "time",
			TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
		},
	},
	"lib:apache2": &LoggingProcessorParseRegex{
		Regex: `^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^ ]*) +\S*)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>.*)")?$`,
		LoggingProcessorParseShared: LoggingProcessorParseShared{
			TimeKey:    "time",
			TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
		},
	},
	"lib:apache_error": &LoggingProcessorParseRegex{
		Regex: `^\[[^ ]* (?<time>[^\]]*)\] \[(?<level>[^\]]*)\](?: \[pid (?<pid>[^\]]*)\])?( \[client (?<client>[^\]]*)\])? (?<message>.*)$`,
	},
	"lib:mongodb": &LoggingProcessorParseRegex{
		Regex: `^(?<time>[^ ]*)\s+(?<severity>\w)\s+(?<component>[^ ]+)\s+\[(?<context>[^\]]+)]\s+(?<message>.*?) *(?<ms>(\d+))?(:?ms)?$`,
		LoggingProcessorParseShared: LoggingProcessorParseShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L",
		},
	},
	"lib:nginx": &LoggingProcessorParseRegex{
		Regex: `^(?<remote>[^ ]*) (?<host>[^ ]*) (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")`,
		LoggingProcessorParseShared: LoggingProcessorParseShared{
			TimeKey:    "time",
			TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
		},
	},
	"lib:syslog-rfc5424": &LoggingProcessorParseRegex{
		Regex: `^\<(?<pri>[0-9]{1,5})\>1 (?<time>[^ ]+) (?<host>[^ ]+) (?<ident>[^ ]+) (?<pid>[-0-9]+) (?<msgid>[^ ]+) (?<extradata>(\[(.*?)\]|-)) (?<message>.+)$`,
		LoggingProcessorParseShared: LoggingProcessorParseShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%Z",
		},
	},
	"lib:syslog-rfc3164": &LoggingProcessorParseRegex{
		Regex: `/^\<(?<pri>[0-9]+)\>(?<time>[^ ]* {1,2}[^ ]* [^ ]*) (?<host>[^ ]*) (?<ident>[a-zA-Z0-9_\/\.\-]*)(?:\[(?<pid>[0-9]+)\])?(?:[^\:]*\:)? *(?<message>.*)$/`,
		LoggingProcessorParseShared: LoggingProcessorParseShared{
			TimeKey:    "time",
			TimeFormat: "%b %d %H:%M:%S",
		},
	},
}
