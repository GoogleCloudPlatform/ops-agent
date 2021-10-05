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

// Package fluentbit provides data structures to represent and generate fluentBit configuration.
package fluentbit

import (
	"fmt"
	"sort"
	"strings"
)

type Component struct {
	// Kind is "INPUT", "FILTER", "PARSER", etc.
	Kind string
	// Config is a set of key-value configuration pairs
	Config map[string]string
	// RepeatedConfig is used for configuration pairs where the
	// key can appear in the output fluent bit config multiple times
	RepeatedConfig map[string][]string
}

// ParserShared holds common parameters that are used by all processors that are implemented with fluentbit's "parser" filter.
type ParserShared struct {
	TimeKey    string `yaml:"time_key,omitempty"`    // by default does not parse timestamp
	TimeFormat string `yaml:"time_format,omitempty"` // must be provided if time_key is present
	// Types allows parsing the extracted fields.
	// Not exposed to users for now, but can be used by app receivers.
	// Documented at https://docs.fluentbit.io/manual/v/1.3/parser
	// According to docs, this is only supported with `ltsv`, `logfmt`, and `regex` parsers.
	Types map[string]string `yaml:"-" validate:"dive,oneof=string integer bool float hex"`
}

// The parser component is incomplete and needs (at a minimum) the "Format" key to be set.
func (p ParserShared) Component(tag string, uid string) (Component, string) {
	parserName := fmt.Sprintf("%s.%s", tag, uid)
	parser := Component{
		Kind: "PARSER",
		Config: map[string]string{
			"Name": parserName,
		},
	}

	if p.TimeFormat != "" {
		parser.Config["Time_Format"] = p.TimeFormat
	}
	if p.TimeKey != "" {
		parser.Config["Time_Key"] = p.TimeKey
	}
	if len(p.Types) > 0 {
		var types []string
		for k, v := range p.Types {
			types = append(types, fmt.Sprintf("%s:%s", k, v))
		}
		sort.Strings(types)
		parser.Config["Types"] = strings.Join(types, " ")
	}

	return parser, parserName
}

func ParserFilterComponent(tag string, field string, parserNames []string) Component {
	filter := Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Match":    tag,
			"Name":     "parser",
			"Key_Name": "message", // Required
		},
		RepeatedConfig: map[string][]string{
			"Parser": parserNames,
		},
	}
	if field != "" {
		filter.Config["Key_Name"] = field
	}

	return filter
}

func (c Component) generateSection() string {
	var lines []string
	var maxLen int
	for k := range c.Config {
		if len(k) > maxLen {
			maxLen = len(k)
		}
	}
	for k := range c.RepeatedConfig {
		if len(k) > maxLen {
			maxLen = len(k)
		}
	}

	addLine := func(k, v string) { lines = append(lines, fmt.Sprintf("    %-*s %s", maxLen, k, v)) }

	for k, v := range c.Config {
		addLine(k, v)
	}
	sort.Strings(lines)

	// Used for Multiline config where several "rule" lines
	// must be placed at the end of a parser config, and when multiple "Parser"
	// are provided to one parser filter
	for k, list := range c.RepeatedConfig {
		for _, v := range list {
			addLine(k, v)
		}
	}

	return fmt.Sprintf("[%s]\n%s\n", c.Kind, strings.Join(lines, "\n"))
}

type ModularConfig struct {
	Variables  map[string]string
	Components []Component
}

func (c ModularConfig) Generate() (string, string, error) {
	var parts []string
	for k, v := range c.Variables {
		parts = append(parts, fmt.Sprintf("@SET %s=%s", k, v))
	}
	sort.Strings(parts)
	// Blank line
	parts = append(parts, "")

	// TODO: Consider removing this sorting and just outputting the components in native order.
	sectionsByKind := map[string][]string{}
	for _, c := range c.Components {
		out := c.generateSection()
		sectionsByKind[c.Kind] = append(sectionsByKind[c.Kind], out)
	}
	parserParts := append(sectionsByKind["PARSER"], sectionsByKind["MULTILINE_PARSER"]...)
	delete(sectionsByKind, "PARSER")
	delete(sectionsByKind, "MULTILINE_PARSER")
	for _, k := range []string{"SERVICE", "INPUT", "FILTER", "OUTPUT"} {
		parts = append(parts, sectionsByKind[k]...)
		delete(sectionsByKind, k)
	}
	if len(sectionsByKind) > 0 {
		return "", "", fmt.Errorf("unknown fluentbit config sections %+v", sectionsByKind)
	}
	return strings.Join(parts, "\n"), strings.Join(parserParts, "\n"), nil
}
