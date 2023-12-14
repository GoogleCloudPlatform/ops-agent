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
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit/modify"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel/ottl"
)

// TODO: Add a validation check that will allow only one unique language exceptions that focus in one specific language.
type ParseMultilineGroup struct {
	Type     string `yaml:"type" validate:"required,oneof=language_exceptions"`
	Language string `yaml:"language" validate:"required,oneof=java python go"`
}

type ParseMultiline struct {
	ConfigComponent `yaml:",inline"`

	// Make this a list so that it's forward compatible to support more `parse_multiline` type other than the build-in language exceptions.
	MultilineGroups []*ParseMultilineGroup `yaml:"match_any" validate:"required,min=1,max=3,unique"`
}

func (r ParseMultiline) Type() string {
	return "parse_multiline"
}

var multilineRulesLanguageMap = map[string][]string{
	// Below is the working java rules provided by fluentbit team: https://github.com/fluent/fluent-bit/issues/4611
	// Move to built-in java support, when upstream fixes the issue
	"java": {`"start_state, java_start_exception"  "/(?:Exception|Error|Throwable|V8 errors stack trace)[:\r\n]/" "java_after_exception"`,
		`"java_nested_exception" "/(?:Exception|Error|Throwable|V8 errors stack trace)[:\r\n]/" "java_after_exception"`,
		`"java_after_exception" "/^[\t ]*nested exception is:[\\t ]*/" "java_nested_exception"`,
		`"java_after_exception" "/^[\r\n]*$/" "java_after_exception"`,
		`"java_after_exception" "/^[\t ]+(?:eval )?at /" "java_after_exception"`,
		`"java_after_exception" "/^[\t ]+--- End of inner exception stack trace ---$/" "java_after_exception"`,
		`"java_after_exception" "/^--- End of stack trace from previous (?x:)location where exception was thrown ---$/" "java_after_exception"`,
		`"java_after_exception" "/^[\t ]*(?:Caused by|Suppressed):/" "java_after_exception"`,
		`"java_after_exception" "/^[\t ]*... \d+ (?:more|common frames omitted)/" "java_after_exception"`},
	"python": {`"start_state, python_start_exception" "/Traceback \(most recent call last\):$/" "python"`,
		`"python" "/^[\t ]+File /" "python_code"`,
		`"python_code" "/[^\t ]/" "python"`,
		`"python" "/^(?:[^\s.():]+\.)*[^\s.():]+:/" "python_start_exception"`},
	"go": {`"start_state" "/\bpanic: /" "go_after_panic"`,
		`"start_state" "/http: panic serving/" "go_goroutine"`,
		`"go_after_panic" "/^$/" "go_goroutine"`,
		`"go_after_panic, go_after_signal, go_frame_1" "/^$/" "go_goroutine"`,
		`"go_after_panic" "/^\[signal /" "go_after_signal"`,
		`"go_goroutine" "/^goroutine \d+ \[[^\]]+\]:$/" "go_frame_1"`,
		`"go_frame_1" "/^(?:[^\s.:]+\.)*[^\s.():]+\(|^created by /" "go_frame_2"`,
		`"go_frame_2" "/^\s/" "go_frame_1"`},
}

func (p ParseMultiline) Components(ctx context.Context, tag, uid string) []fluentbit.Component {
	var components []fluentbit.Component
	// Fluent Bit multiline parser currently can't export using `message` as key.
	// Thus we need to add one renaming component per pipeline
	// Remove below two lines when https://github.com/fluent/fluent-bit/issues/4795 is fixed
	renameLogToMessage := modify.NewRenameOptions("log", "message")
	components = append(components, renameLogToMessage.Component(tag))
	var combinedRules []string
	for _, g := range p.MultilineGroups {
		if g.Type == "language_exceptions" {
			combinedRules = append(combinedRules, multilineRulesLanguageMap[g.Language]...)
		}
	}
	component := fluentbit.ParseMultilineComponent(tag, uid, combinedRules)
	components = append(components, component...)
	return components
}

func init() {
	LoggingProcessorTypes.RegisterType(func() LoggingProcessor { return &ParseMultiline{} })
}

// ParserShared holds common parameters that are used by all processors that are implemented with fluentbit's "parser" filter.
type ParserShared struct {
	TimeKey    string `yaml:"time_key,omitempty" validate:"required_with=TimeFormat,omitempty,fieldlegacy"` // by default does not parse timestamp
	TimeFormat string `yaml:"time_format,omitempty" validate:"required_with=TimeKey"`                       // must be provided if time_key is present
	// Types allows parsing the extracted fields.
	// Not exposed to users for now, but can be used by app receivers.
	// Documented at https://docs.fluentbit.io/manual/v/1.3/parser
	// According to docs, this is only supported with `ltsv`, `logfmt`, and `regex` parsers.
	Types map[string]string `yaml:"-" validate:"dive,oneof=string integer bool float hex"`
}

func (p ParserShared) Component(tag, uid string) (fluentbit.Component, string) {
	return fluentbit.ParserComponentBase(p.TimeFormat, p.TimeKey, p.Types, tag, uid)
}

func (p ParserShared) TimestampStatements() (ottl.Statements, error) {
	if p.TimeKey == "" {
		return nil, nil
	}
	from, err := filter.NewMemberLegacy(p.TimeKey)
	if err != nil {
		return nil, err
	}
	fromAccessor, err := from.OTTLAccessor()
	if err != nil {
		return nil, err
	}
	return ottl.NewStatements(
		ottl.PathToValue("time").Set(fromAccessor.ToTime(p.TimeFormat)),
		fromAccessor.Delete(),
	), nil
}

func (p ParserShared) TimestampConfig() map[string]any {
	if p.TimeKey == "" {
		return nil
	}
	// TODO: Support arbitrary fields using filter.Member
	from := fmt.Sprintf("body.%s", p.TimeKey)
	return map[string]any{
		"parse_from":  from,
		"layout_type": "strptime",
		"layout":      p.TimeFormat,
	}
}

func (p ParserShared) TypesStatements() (ottl.Statements, error) {
	var out ottl.Statements
	for field, fieldType := range p.Types {
		m, err := filter.NewMemberLegacy(field)
		if err != nil {
			return nil, err
		}
		a, err := m.OTTLAccessor()
		if err != nil {
			return nil, err
		}
		// See OTTL docs at https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/ottl/ottlfuncs
		switch fieldType {
		case "string":
			out = out.Append(a.Set(a.ToString()))
		case "integer":
			out = out.Append(a.Set(a.ToInt()))
		case "bool":
			out = out.Append(a.SetToBool(a))
		case "float":
			out = out.Append(a.Set(a.ToFloat()))
		case "hex":
			// TODO: Not exposed in OTTL
			fallthrough
		default:
			return nil, fmt.Errorf("type %q not supported for field %s", fieldType, m)
		}
	}
	return out, nil
}

// A LoggingProcessorParseJson parses the specified field as JSON.
type LoggingProcessorParseJson struct {
	ConfigComponent `yaml:",inline"`
	ParserShared    `yaml:",inline"`
	Field           string `yaml:"field,omitempty" validate:"omitempty,fieldlegacy"`
}

func (r LoggingProcessorParseJson) Type() string {
	return "parse_json"
}

func (p LoggingProcessorParseJson) Components(ctx context.Context, tag, uid string) []fluentbit.Component {
	parser, parserName := p.ParserShared.Component(tag, uid)
	parser.Config["Format"] = "json"

	parserFilters := []fluentbit.Component{}
	parserFilters = append(parserFilters, fluentbit.ParserFilterComponents(tag, p.Field, []string{parserName}, false)...)
	parserFilters = append(parserFilters, parser)
	return parserFilters
}

func (p LoggingProcessorParseJson) Processors() []otel.Component {
	out, err := p.processors()
	if err != nil {
		// It shouldn't be possible to get here if the input validation is working
		panic(err)
	}
	return out
}

func (p LoggingProcessorParseJson) processors() ([]otel.Component, error) {
	from := p.Field
	// TODO: Parse field using filter.Member (but somehow also continue to support bare fields as currently allowed)
	if from == "" {
		from = "body.message"
	}
	m, err := filter.NewMemberLegacy(from)
	if err != nil {
		return nil, err
	}

	fromAccessor, err := m.OTTLAccessor()
	if err != nil {
		return nil, err
	}

	statements := ottl.NewStatements(
		ottl.PathToValue("body").Set(fromAccessor.ParseJSON()),
	)

	ts, err := p.TimestampStatements()
	if err != nil {
		return nil, err
	}
	statements = statements.Append(ts)
	ts, err = p.TypesStatements()
	if err != nil {
		return nil, err
	}
	statements = statements.Append(ts)

	// TODO: Handle special fields documented at https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration#special-fields

	// TODO: Support merging instead of replacing.
	return []otel.Component{otel.Transform(
		"log", "log",
		statements,
	)}, nil
}

func init() {
	LoggingProcessorTypes.RegisterType(func() LoggingProcessor { return &LoggingProcessorParseJson{} })
}

// A LoggingProcessorParseRegex applies a regex to the specified field, storing the named capture groups as keys in the log record.
// This was maintained in addition to the parse_regex_complex to ensure backward compatibility with any existing configurations
type LoggingProcessorParseRegex struct {
	ConfigComponent `yaml:",inline"`
	ParserShared    `yaml:",inline"`
	Field           string `yaml:"field,omitempty" validate:"omitempty,fieldlegacy"`
	PreserveKey     bool   `yaml:"-"`

	Regex string `yaml:"regex,omitempty" validate:"required"`
}

func (r LoggingProcessorParseRegex) Type() string {
	return "parse_regex"
}

func (p LoggingProcessorParseRegex) Components(ctx context.Context, tag, uid string) []fluentbit.Component {
	parser, parserName := p.ParserShared.Component(tag, uid)
	parser.Config["Format"] = "regex"
	parser.Config["Regex"] = p.Regex

	parserFilters := []fluentbit.Component{}
	parserFilters = append(parserFilters, parser)
	parserFilters = append(parserFilters, fluentbit.ParserFilterComponents(tag, p.Field, []string{parserName}, p.PreserveKey)...)
	return parserFilters
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

func (p LoggingProcessorParseRegexComplex) Components(ctx context.Context, tag, uid string) []fluentbit.Component {
	components := []fluentbit.Component{}
	parserNames := []string{}

	for idx, parserConfig := range p.Parsers {
		parser, parserName := parserConfig.Parser.Component(tag, fmt.Sprintf("%s.%d", uid, idx))
		parser.Config["Format"] = "regex"
		parser.Config["Regex"] = parserConfig.Regex
		components = append(components, parser)
		parserNames = append(parserNames, parserName)
	}

	components = append(components, fluentbit.ParserFilterComponents(tag, p.Field, parserNames, false)...)
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
//
//	#
//	# Regex rules for multiline parsing
//	# ---------------------------------
//	#
//	# configuration hints:
//	#
//	#  - first state always has the name: start_state
//	#  - every field in the rule must be inside double quotes
//	#
//	# rules |   state name  | regex pattern                  | next state
//	# ------|---------------|--------------------------------------------
//	rule      "start_state"   "/(Dec \d+ \d+\:\d+\:\d+)(.*)/"  "cont"
//	rule      "cont"          "/^\s+at.*/"                     "cont"
type LoggingProcessorParseMultilineRegex struct {
	LoggingProcessorParseRegexComplex
	Rules []MultilineRule
}

func (p LoggingProcessorParseMultilineRegex) Components(ctx context.Context, tag, uid string) []fluentbit.Component {
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

	return append([]fluentbit.Component{filter, multilineParser}, p.LoggingProcessorParseRegexComplex.Components(ctx, tag, uid)...)
}

func init() {
	LoggingProcessorTypes.RegisterType(func() LoggingProcessor { return &LoggingProcessorParseRegex{} })
}

type LoggingProcessorNestWildcard struct {
	Wildcard     string
	NestUnder    string
	RemovePrefix string
}

func (p LoggingProcessorNestWildcard) Components(ctx context.Context, tag, uid string) []fluentbit.Component {
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

func (p LoggingProcessorExcludeLogs) Components(ctx context.Context, tag, uid string) []fluentbit.Component {
	filters := make([]*filter.Filter, 0, len(p.MatchAny))
	for _, condition := range p.MatchAny {
		filter, err := filter.NewFilter(condition)
		if err != nil {
			log.Printf("error parsing condition '%s': %v", condition, err)
			return nil
		}
		filters = append(filters, filter)
	}
	components, lua := filter.AllFluentConfig(tag, map[string]*filter.Filter{
		"match": filter.MatchesAny(filters),
	})
	components = append(components, fluentbit.LuaFilterComponents(
		tag, "process", fmt.Sprintf(`
function process(tag, timestamp, record)
%s
  if match then
    return -1, 0, 0
  end
  return 2, 0, record
end`, lua))...)
	return components
}

func init() {
	LoggingProcessorTypes.RegisterType(func() LoggingProcessor { return &LoggingProcessorExcludeLogs{} })
}
