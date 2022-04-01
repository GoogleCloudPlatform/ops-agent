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
	"log"
	"regexp"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

// ParserShared holds common parameters that are used by all processors that are implemented with fluentbit's "parser" filter.
type ParserShared struct {
	TimeKey    string `yaml:"time_key,omitempty" validate:"required_with=TimeFormat"` // by default does not parse timestamp
	TimeFormat string `yaml:"time_format,omitempty" validate:"required_with=TimeKey"` // must be provided if time_key is present
	// Types allows parsing the extracted fields.
	// Not exposed to users for now, but can be used by app receivers.
	// Documented at https://docs.fluentbit.io/manual/v/1.3/parser
	// According to docs, this is only supported with `ltsv`, `logfmt`, and `regex` parsers.
	Types map[string]string `yaml:"-" validate:"dive,oneof=string integer bool float hex"`
}

func (p ParserShared) Component(tag, uid string) (fluentbit.Component, string) {
	return fluentbit.ParserComponentBase(p.TimeFormat, p.TimeKey, p.Types, tag, uid)
}

// A LoggingProcessorParseJson parses the specified field as JSON.
type LoggingProcessorParseJson struct {
	ConfigComponent `yaml:",inline"`
	ParserShared    `yaml:",inline"`
	Field           string `yaml:"field,omitempty"`
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

// rewriteComplexCaptures translates disallowed capture group names into placeholders and a transformation to rename them back in the log record.
func rewriteComplexCaptures(regex, tag string) (string, []fluentbit.Component) {
	// Short-circuit regexes that don't have disallowed capture group names
	disallowed := regexp.MustCompile(`\(\?P<(?:[A-Za-z0-9_]*[^A-Za-z0-9_>])+(?:[A-Za-z0-9_]*)>`)
	if !disallowed.MatchString(regex) {
		return regex, nil
	}
	// Maintain a list of rewritten capture group names
	var rewrites []string
	captureGroup := regexp.MustCompile(`\(\?P<((?:[^>\\]|\\.)*)>`)
	// Can't use ReplaceAllStringFunc, since it doesn't support replacing only captured values
	groupIndexes := captureGroup.FindAllStringSubmatchIndex(regex, -1)
	l := 0
	var r []string
	for _, i := range groupIndexes {
		g := regex[i[0]:i[1]] // Full match
		s := regex[i[2]:i[3]] // First capture group
		r = append(r, regex[l:i[2]])
		// Also replace any capture group whose name starts with "__"
		if !disallowed.MatchString(g) && !strings.HasPrefix(s, "__") {
			r = append(r, s)
		} else {
			rewrites = append(rewrites, s)
			r = append(r, fmt.Sprintf("__%d", len(rewrites)))
		}
		l = i[3]
	}
	r = append(r, regex[l:])
	// Reconstruct the regex
	regex = strings.Join(r, "")
	// Rename all captured fields
	oc := make([][2]string, len(rewrites))
	for i, g := range rewrites {
		oc = append(oc, [2]string{"Rename", fmt.Sprintf("__%d %q", i+1, g)})
	}
	rename := fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Match": tag,
			"Name":  "modify",
		},
		OrderedConfig: oc,
	}
	return regex, []fluentbit.Component{rename}
}

// A LoggingProcessorParseRegex applies a regex to the specified field, storing the named capture groups as keys in the log record.
// This was maintained in addition to the parse_regex_complex to ensure backward compatibility with any existing configurations
type LoggingProcessorParseRegex struct {
	ConfigComponent `yaml:",inline"`
	ParserShared    `yaml:",inline"`
	Field           string `yaml:"field,omitempty"`

	Regex string `yaml:"regex,omitempty" validate:"required"`
}

func (r LoggingProcessorParseRegex) Type() string {
	return "parse_regex"
}

func (p LoggingProcessorParseRegex) Components(tag, uid string) []fluentbit.Component {
	regex, transforms := rewriteComplexCaptures(p.Regex, tag)

	parser, parserName := p.ParserShared.Component(tag, uid)
	parser.Config["Format"] = "regex"
	parser.Config["Regex"] = regex

	return append([]fluentbit.Component{
		parser,
		fluentbit.ParserFilterComponent(tag, p.Field, []string{parserName}),
	}, transforms...)
}

type RegexParser struct {
	Regex  string
	Parser ParserShared
}

// A LoggingProcessorParseRegexComplex applies a set of regexes to the specified field, storing the named capture groups as keys in the log record.
type LoggingProcessorParseRegexComplex struct {
	Field   string
	Parsers []RegexParser
}

func (p LoggingProcessorParseRegexComplex) Components(tag, uid string) []fluentbit.Component {
	components := []fluentbit.Component{}
	parserNames := []string{}

	for idx, parserConfig := range p.Parsers {
		regex, transforms := rewriteComplexCaptures(parserConfig.Regex, tag)

		parser, parserName := parserConfig.Parser.Component(tag, fmt.Sprintf("%s.%d", uid, idx))
		parser.Config["Format"] = "regex"
		parser.Config["Regex"] = regex
		components = append(components, parser)
		components = append(components, transforms...)
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
	rules := [][2]string{}
	for _, rule := range p.Rules {
		rules = append(rules, [2]string{"rule", rule.AsString()})
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
		OrderedConfig: rules,
	}

	return append([]fluentbit.Component{filter, multilineParser}, p.LoggingProcessorParseRegexComplex.Components(tag, uid)...)
}

func init() {
	LoggingProcessorTypes.RegisterType(func() Component { return &LoggingProcessorParseRegex{} })
}

type LoggingProcessorNestWildcard struct {
	Wildcard     string
	NestUnder    string
	RemovePrefix string
}

func (p LoggingProcessorNestWildcard) Components(tag, uid string) []fluentbit.Component {
	filter := fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":          "nest",
			"Match":         tag,
			"Operation":     "nest",
			"Wildcard":      p.Wildcard,
			"Nest_under":    p.NestUnder,
			"Remove_prefix": p.RemovePrefix,
		},
	}

	return []fluentbit.Component{
		filter,
	}
}

var LegacyBuiltinProcessors = map[string]LoggingProcessor{
	"lib:default_message_parser": &LoggingProcessorParseRegex{
		Regex: `^(?<message>.*)$`,
	},
	"lib:apache": &LoggingProcessorParseRegex{
		Regex: `^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")?$`,
		ParserShared: ParserShared{
			TimeKey:    "time",
			TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
		},
	},
	"lib:apache2": &LoggingProcessorParseRegex{
		Regex: `^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^ ]*) +\S*)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>.*)")?$`,
		ParserShared: ParserShared{
			TimeKey:    "time",
			TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
		},
	},
	"lib:apache_error": &LoggingProcessorParseRegex{
		Regex: `^\[[^ ]* (?<time>[^\]]*)\] \[(?<level>[^\]]*)\](?: \[pid (?<pid>[^\]]*)\])?( \[client (?<client>[^\]]*)\])? (?<message>.*)$`,
	},
	"lib:mongodb": &LoggingProcessorParseRegex{
		Regex: `^(?<time>[^ ]*)\s+(?<severity>\w)\s+(?<component>[^ ]+)\s+\[(?<context>[^\]]+)]\s+(?<message>.*?) *(?<ms>(\d+))?(:?ms)?$`,
		ParserShared: ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L",
		},
	},
	"lib:nginx": &LoggingProcessorParseRegex{
		Regex: `^(?<remote>[^ ]*) (?<host>[^ ]*) (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")`,
		ParserShared: ParserShared{
			TimeKey:    "time",
			TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
		},
	},
	"lib:syslog-rfc5424": &LoggingProcessorParseRegex{
		Regex: `^\<(?<pri>[0-9]{1,5})\>1 (?<time>[^ ]+) (?<host>[^ ]+) (?<ident>[^ ]+) (?<pid>[-0-9]+) (?<msgid>[^ ]+) (?<extradata>(\[(.*?)\]|-)) (?<message>.+)$`,
		ParserShared: ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%Z",
		},
	},
	"lib:syslog-rfc3164": &LoggingProcessorParseRegex{
		Regex: `/^\<(?<pri>[0-9]+)\>(?<time>[^ ]* {1,2}[^ ]* [^ ]*) (?<host>[^ ]*) (?<ident>[a-zA-Z0-9_\/\.\-]*)(?:\[(?<pid>[0-9]+)\])?(?:[^\:]*\:)? *(?<message>.*)$/`,
		ParserShared: ParserShared{
			TimeKey:    "time",
			TimeFormat: "%b %d %H:%M:%S",
		},
	},
}

// A LoggingProcessorExcludeLogs filters out logs according to a pattern.
type LoggingProcessorExcludeLogs struct {
	ConfigComponent `yaml:",inline"`
	MatchAny        []string `yaml:"match_any" validate:"required,dive,filter"`
}

func (r LoggingProcessorExcludeLogs) Type() string {
	return "exclude_logs"
}

func (p LoggingProcessorExcludeLogs) Components(tag, uid string) []fluentbit.Component {
	filters := make([]*filter.Filter, 0, len(p.MatchAny))
	for _, condition := range p.MatchAny {
		filter, err := filter.NewFilter(condition)
		if err != nil {
			log.Printf("error parsing condition '%s': %v", condition, err)
			return nil
		}
		filters = append(filters, filter)
	}
	return filter.AllComponents(tag, filters, true)
}

func init() {
	LoggingProcessorTypes.RegisterType(func() Component { return &LoggingProcessorExcludeLogs{} })
}
