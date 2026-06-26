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
	"regexp"
	"sort"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel/ottl"
)

// ParserShared holds common parameters that are used by all processors that are implemented with parser filter.
type ParserShared struct {
	TimeKey    string `yaml:"time_key,omitempty" validate:"required_with=TimeFormat,omitempty,fieldlegacy"` // by default does not parse timestamp
	TimeFormat string `yaml:"time_format,omitempty" validate:"required_with=TimeKey"`                       // must be provided if time_key is present
	// Types allows parsing the extracted fields.
	// Not exposed to users for now, but can be used by app receivers.
	Types map[string]string `yaml:"-" validate:"dive,oneof=string integer bool float hex"`
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
	for _, field := range GetSortedKeys(p.Types) {
		fieldType := p.Types[field]
		m, err := filter.NewMemberLegacy(field)
		if err != nil {
			return nil, err
		}
		a, err := m.OTTLAccessor()
		if err != nil {
			return nil, err
		}
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
			out = out.Append(a.Set(ottl.ParseInt(a, 16)))
		default:
			return nil, fmt.Errorf("type %q not supported for field %s", fieldType, m)
		}
	}
	return out, nil
}

// Handle special fields documented at https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration#special-fields
func (p ParserShared) SpecialFieldsStatements(ctx context.Context) ottl.Statements {
	fields := filter.SpecialFields()
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
			panic(err)
		}
		statements = statements.Append(s)
	}

	funcOld := ottl.LValue{"attributes", "gcp.source_location", "function"}
	funcNew := ottl.LValue{"attributes", "gcp.source_location", "func"}

	statements = statements.Append(ottl.NewStatements(
		funcNew.SetIf(funcOld, funcOld.IsPresent()),
		funcOld.DeleteIf(funcNew.IsPresent()),
	))

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

	statements = statements.Append(p.SpecialFieldsStatements(ctx))

	return []otel.Component{otel.Transform(
		"log", "log",
		statements,
	)}, nil
}

func init() {
	LoggingProcessorTypes.RegisterType(func() LoggingProcessor { return &LoggingProcessorParseJson{} })
}

// A LoggingProcessorParseRegex applies a regex to the specified field, storing the named capture groups as keys in the log record.
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

func (p LoggingProcessorParseRegex) ListAllFeatures() ([]string, bool) {
	return []string{
		"has_ruby_regex",
	}, false
}

func (p LoggingProcessorParseRegex) ExtractFeatures() ([]CustomFeature, bool, error) {
	_, err := regexp.Compile(p.Regex)
	goCompatible := err == nil
	return []CustomFeature{
		{
			Key:   []string{"has_ruby_regex"},
			Value: fmt.Sprintf("%v", !goCompatible),
		},
	}, false, nil
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

	statements = statements.Append(p.SpecialFieldsStatements(ctx))

	return []otel.Component{otel.Transform(
		"log", "log",
		statements,
	)}, nil
}

func init() {
	LoggingProcessorTypes.RegisterType(func() LoggingProcessor { return &LoggingProcessorParseRegex{} })
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

func (p LoggingProcessorExcludeLogs) ListAllFeatures() ([]string, bool) {
	return []string{
		"has_ruby_regex",
	}, false
}

func (p LoggingProcessorExcludeLogs) ExtractFeatures() ([]CustomFeature, bool, error) {
	filters, err := p.filters()
	if err != nil {
		return nil, false, nil
	}
	hasRubyRegex := false
	for _, f := range filters {
		if f.HasRubyRegex() {
			hasRubyRegex = true
			break
		}
	}
	return []CustomFeature{
		{
			Key:   []string{"has_ruby_regex"},
			Value: fmt.Sprintf("%v", hasRubyRegex),
		},
	}, false, nil
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
