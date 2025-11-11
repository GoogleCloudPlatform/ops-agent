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
	"sort"
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

var multilineRulesLanguageMap = map[string][]MultilineRule{
	// Below is the working java rules provided by fluentbit team: https://github.com/fluent/fluent-bit/issues/4611
	// Move to built-in java support, when upstream fixes the issue
	"java": []MultilineRule{
		{"start_state, java_start_exception", `/(?:Exception|Error|Throwable|V8 errors stack trace)[:\r\n]/`, "java_after_exception"},
		{"java_nested_exception", `/(?:Exception|Error|Throwable|V8 errors stack trace)[:\r\n]/`, "java_after_exception"},
		{"java_after_exception", `/^[\t ]*nested exception is:[\\t ]*/`, "java_nested_exception"},
		{"java_after_exception", `/^[\r\n]*$/`, "java_after_exception"},
		{"java_after_exception", `/^[\t ]+(?:eval )?at /`, "java_after_exception"},
		{"java_after_exception", `/^[\t ]+--- End of inner exception stack trace ---$/`, "java_after_exception"},
		{"java_after_exception", `/^--- End of stack trace from previous (?x:)location where exception was thrown ---$/`, "java_after_exception"},
		{"java_after_exception", `/^[\t ]*(?:Caused by|Suppressed):/`, "java_after_exception"},
		{"java_after_exception", `/^[\t ]*... \d+ (?:more|common frames omitted)/`, "java_after_exception"},
	},
	"python": []MultilineRule{
		{"start_state, python_start_exception", `/Traceback \(most recent call last\):$/`, "python"},
		{"python", `/^[\t ]+File /`, "python_code"},
		{"python_code", `/[^\t ]/`, "python"},
		{"python", `/^(?:[^\s.():]+\.)*[^\s.():]+:/`, "python_start_exception"},
	},
	"go": []MultilineRule{
		{"start_state", `/\bpanic: /`, "go_after_panic"},
		{"start_state", `/http: panic serving/`, "go_goroutine"},
		{"go_after_panic", `/^$/`, "go_goroutine"},
		{"go_after_panic, go_after_signal, go_frame_1", `/^$/`, "go_goroutine"},
		{"go_after_panic", `/^\[signal /`, "go_after_signal"},
		{"go_goroutine", `/^goroutine \d+ \[[^\]]+\]:$/`, "go_frame_1"},
		{"go_frame_1", `/^(?:[^\s.:]+\.)*[^\s.():]+\(|^created by /`, "go_frame_2"},
		{"go_frame_2", `/^\s/`, "go_frame_1"},
	},
}

func (p ParseMultiline) CombinedRules() []MultilineRule {
	var combinedRules []MultilineRule
	for _, g := range p.MultilineGroups {
		if g.Type == "language_exceptions" {
			combinedRules = append(combinedRules, multilineRulesLanguageMap[g.Language]...)
		}
	}
	return combinedRules
}

func (p ParseMultiline) Components(ctx context.Context, tag, uid string) []fluentbit.Component {
	// Fluent Bit multiline parser currently can't export using `message` as key.
	// Thus we need to add one renaming component per pipeline
	// Remove the rename component when https://github.com/fluent/fluent-bit/issues/4795 is fixed
	parserName := fmt.Sprintf("multiline.%s.%s", tag, uid)
	parserComponent := fluentbit.ParseMultilineComponent(parserName, p.CombinedRules())
	filter := fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":                  "multiline",
			"Match":                 tag,
			"Multiline.Key_Content": "message",
			"Multiline.Parser":      parserName,
		},
	}
	// TODO: Refactor to share an implementation with LoggingReceiverFilesMixin.Components
	return []fluentbit.Component{
		filter,
		parserComponent,
		modify.NewRenameOptions("log", "message").Component(tag),
	}
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
	flag := ottl.LValue{"cache", "__time_valid"}
	// Replicate fluent-bit behavior of preserving the existing field if the time is unparsable.
	// The result of `ToTime` cannot be stored in `cache`, so instead we store a boolean flag.
	return ottl.NewStatements(
		flag.Set(ottl.False()),
		flag.SetIf(ottl.True(), ottl.And(
			fromAccessor.IsPresent(),
			ottl.IsNotNil(ottl.ToTime(fromAccessor, p.TimeFormat)),
		)),
		ottl.LValue{"time"}.SetIf(ottl.ToTime(fromAccessor, p.TimeFormat), ottl.Equals(flag, ottl.True())),
		fromAccessor.DeleteIf(ottl.Equals(flag, ottl.True())),
	), nil
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
			out = out.Append(a.Set(ottl.ToString(a)))
		case "integer":
			out = out.Append(a.Set(ottl.ToInt(a)))
		case "bool":
			out = out.Append(a.SetToBool(a))
		case "float":
			out = out.Append(a.Set(ottl.ToFloat(a)))
		case "hex":
			// The strtoull C function is used in Fluent Bit to parse hexadecimal strings: https://github.com/fluent/fluent-bit/blob/a20a127f1b5ae70706bac0dac45fbc6abde4ad27/src/flb_parser.c#L1319
			// strtoull parses strings into an unsigned long long integer, which is equivalent to an uint64 in Go. It also accepts hexadecimal strings with a leading "0x" prefix and trailing whitespace. Additionally, it accepts a leading "+" or "-" sign.
			// However, ottl.ParseInt(a, 16) only parses hexadecimal strings without a leading "0x" prefix (e.g., "AF111", "-123F") and does not allow trailing whitespace. It does accept a leading "+" or "-" sign.
			// ottl.ParseInt parses the string to an int64.
			out = out.Append(a.Set(ottl.ParseInt(a, 16)))
		default:
			return nil, fmt.Errorf("type %q not supported for field %s", fieldType, m)
		}
	}
	return out, nil
}

// Handle special fields documented at https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration#special-fields
func (p ParserShared) FluentBitSpecialFieldsStatements(ctx context.Context) ottl.Statements {
	fields := filter.FluentBitSpecialFields()
	var names []string
	for f := range fields {
		if fields[f] == "labels" {
			continue
		}
		names = append(names, f)
	}
	sort.Strings(names)
	labels := ottl.LValue{"body", "logging.googleapis.com/labels"}
	statements := ottl.NewStatements(
		// Do labels first so other fields can override it.
		ottl.LValue{"attributes"}.MergeMaps(labels, "upsert"),
		labels.Delete(),
	)
	for _, f := range names {
		s, err := LoggingProcessorModifyFields{Fields: map[string]*ModifyField{
			fields[f]: &ModifyField{
				MoveFrom: fmt.Sprintf(`jsonPayload.%q`, f),
			},
		}}.statements(ctx)
		if err != nil {
			// Should be impossible
			panic(err)
		}
		statements = statements.Append(s)
	}
	return statements
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

func (p LoggingProcessorParseJson) Processors(ctx context.Context) ([]otel.Component, error) {
	from := p.Field
	if from == "" {
		from = "jsonPayload.message"
	}
	m, err := filter.NewMemberLegacy(from)
	if err != nil {
		return nil, err
	}

	fromAccessor, err := m.OTTLAccessor()
	if err != nil {
		return nil, err
	}

	cachedJSON := ottl.LValue{"cache", "__parsed_json"}
	statements := ottl.NewStatements(
		cachedJSON.SetIf(ottl.ParseJSON(fromAccessor), fromAccessor.IsPresent()),
		fromAccessor.DeleteIf(cachedJSON.IsPresent()),
		ottl.LValue{"body"}.MergeMapsIf(cachedJSON, "upsert", cachedJSON.IsPresent()),
		cachedJSON.Delete(),
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

	statements = statements.Append(p.FluentBitSpecialFieldsStatements(ctx))

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

func (p LoggingProcessorParseRegex) Processors(ctx context.Context) ([]otel.Component, error) {
	from := p.Field
	if from == "" {
		from = "jsonPayload.message"
	}
	m, err := filter.NewMemberLegacy(from)
	if err != nil {
		return nil, err
	}

	fromAccessor, err := m.OTTLAccessor()
	if err != nil {
		return nil, err
	}

	cachedParsedRegex := ottl.LValue{"cache", "__parsed_regex"}
	statements := ottl.NewStatements(
		// Set `OmitEmptyValues : true` to have the same behaviour as fluent-bit `parse_regex` with `Skip_Empty_Values: true`.
		cachedParsedRegex.SetIf(ottl.ExtractPatternsRubyRegex(fromAccessor, p.Regex, true), ottl.And(
			fromAccessor.IsPresent(),
			ottl.IsMatchRubyRegex(fromAccessor, p.Regex),
		)),
		fromAccessor.DeleteIf(cachedParsedRegex.IsPresent()),
		ottl.LValue{"body"}.MergeMapsIf(cachedParsedRegex, "upsert", cachedParsedRegex.IsPresent()),
		cachedParsedRegex.Delete(),
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

	statements = statements.Append(p.FluentBitSpecialFieldsStatements(ctx))

	return []otel.Component{otel.Transform(
		"log", "log",
		statements,
	)}, nil
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
	if len(p.Parsers) == 0 {
		return []fluentbit.Component{}
	}

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

func (p LoggingProcessorParseRegexComplex) Processors(ctx context.Context) ([]otel.Component, error) {
	if len(p.Parsers) == 0 {
		return []otel.Component{}, nil
	}

	processors := []otel.Component{}
	for _, parserConfig := range p.Parsers {
		parseRegex := LoggingProcessorParseRegex{
			ParserShared: parserConfig.Parser,
			Regex:        parserConfig.Regex,
			Field:        p.Field,
		}
		parseRegexProcessors, err := parseRegex.Processors(ctx)
		if err != nil {
			return nil, err
		}
		processors = append(processors, parseRegexProcessors...)
	}
	return processors, nil
}

type MultilineRule = fluentbit.MultilineRule

// A LoggingProcessorParseMultilineRegex applies a set of regex rules to the specified lines, storing the named capture groups as keys in the log record.
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

	return append(
		[]fluentbit.Component{
			filter,
			fluentbit.ParseMultilineComponent(multilineParserName, p.Rules),
		},
		p.LoggingProcessorParseRegexComplex.Components(ctx, tag, uid)...,
	)
}

func (p LoggingProcessorParseMultilineRegex) Processors(ctx context.Context) ([]otel.Component, error) {
	var firstLines []string
	for _, r := range p.Rules {
		if r.StateName == "start_state" {
			firstLines = append(firstLines, r.Regex)
		}
	}

	var exprParts []string
	for _, r := range firstLines {
		exprParts = append(exprParts, fmt.Sprintf("body.message matches %q", r))
	}
	expr := strings.Join(exprParts, " or ")

	logsTransform := []otel.Component{
		{
			Type: "logstransform",
			Config: map[string]any{
				"operators": []map[string]any{
					{
						"type":  "add",
						"field": "attributes.__source_identifier",
						"value": `EXPR(attributes["agent.googleapis.com/log_file_path"] ?? "")`,
					},
					{
						"type":           "recombine",
						"combine_field":  "body.message",
						"is_first_entry": expr,
						// Take the timestamp and other attributes from the first entry.
						"overwrite_with": "oldest",
						// Use the log file path to disambiguate if present.
						"source_identifier": `attributes.__source_identifier`,
						// Set to half of the filelogreceiver default "poll_interval" (200ms) to guarantee it is flushed every poll.
						"force_flush_period": "100ms",
					},
					{
						"type":  "remove",
						"field": "attributes.__source_identifier",
					},
				},
			},
		},
	}

	parseRegexComplexComponents, err := p.LoggingProcessorParseRegexComplex.Processors(ctx)
	if err != nil {
		return nil, err
	}

	// return logsTransform, nil

	return append(logsTransform, parseRegexComplexComponents...), nil
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

func (p LoggingProcessorExcludeLogs) Type() string {
	return "exclude_logs"
}

func (p LoggingProcessorExcludeLogs) filters() ([]*filter.Filter, error) {
	filters := make([]*filter.Filter, 0, len(p.MatchAny))
	for _, condition := range p.MatchAny {
		filter, err := filter.NewFilter(condition)
		if err != nil {
			return nil, fmt.Errorf("error parsing condition '%s': %v", condition, err)
		}
		filters = append(filters, filter)
	}
	return filters, nil
}

func (p LoggingProcessorExcludeLogs) Components(ctx context.Context, tag, uid string) []fluentbit.Component {
	filters, err := p.filters()
	if err != nil {
		panic(err)
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

func (p LoggingProcessorExcludeLogs) Processors(ctx context.Context) ([]otel.Component, error) {
	filters, err := p.filters()
	if err != nil {
		return nil, err
	}
	var expressions []ottl.Value
	for _, f := range filters {
		expr, err := f.OTTLExpression()
		if err != nil {
			return nil, fmt.Errorf("failed to process condition %q: %w", f, err)
		}
		expressions = append(expressions, expr)
	}
	return []otel.Component{otel.Filter(
		"logs", "log_record",
		expressions,
	)}, nil
}

func init() {
	LoggingProcessorTypes.RegisterType(func() LoggingProcessor { return &LoggingProcessorExcludeLogs{} })
}
