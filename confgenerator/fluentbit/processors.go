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
