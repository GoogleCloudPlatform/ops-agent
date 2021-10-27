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

func TranslationComponents(tag, src, dest string, translations []struct{ SrcVal, DestVal string }) []Component {
	c := []Component{}
	for _, t := range translations {
		c = append(c, Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":      "modify",
				"Match":     tag,
				"Condition": fmt.Sprintf("Key_Value_Equals %s %s", src, t.SrcVal),
				"Add":       fmt.Sprintf("%s %s", dest, t.DestVal),
			},
		})
	}

	return c
}

// The parser component is incomplete and needs (at a minimum) the "Format" key to be set.
func ParserComponentBase(TimeFormat string, TimeKey string, Types map[string]string, tag string, uid string) (Component, string) {
	parserName := fmt.Sprintf("%s.%s", tag, uid)
	parser := Component{
		Kind: "PARSER",
		Config: map[string]string{
			"Name": parserName,
		},
	}

	if TimeFormat != "" {
		parser.Config["Time_Format"] = TimeFormat
	}
	if TimeKey != "" {
		parser.Config["Time_Key"] = TimeKey
	}
	if len(Types) > 0 {
		var types []string
		for k, v := range Types {
			types = append(types, fmt.Sprintf("%s:%s", k, v))
		}
		sort.Strings(types)
		parser.Config["Types"] = strings.Join(types, " ")
	}

	return parser, parserName
}

func ParserFilterComponent(tag string, field string, parserNames []string) Component {
	parsers := [][2]string{}
	for _, name := range parserNames {
		parsers = append(parsers, [2]string{"Parser", name})
	}
	filter := Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Match":    tag,
			"Name":     "parser",
			"Key_Name": "message", // Required
		},
		OrderedConfig: parsers,
	}
	if field != "" {
		filter.Config["Key_Name"] = field
	}

	return filter
}
