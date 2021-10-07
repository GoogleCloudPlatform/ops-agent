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
			Config: [][2]string{
				{"Add", fmt.Sprintf("%s %s", dest, t.DestVal)},
				{"Condition", fmt.Sprintf("Key_Value_Equals %s %s", src, t.SrcVal)},
				{"Match", tag},
				{"Name", "modify"},
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
		Config: [][2]string{
			{"Name", parserName},
		},
	}

	if TimeFormat != "" {
		parser.Config = append(parser.Config, [2]string{"Time_Format", TimeFormat})
	}
	if TimeKey != "" {
		parser.Config = append(parser.Config, [2]string{"Time_Key", TimeKey})
	}
	if len(Types) > 0 {
		var types []string
		for k, v := range Types {
			types = append(types, fmt.Sprintf("%s:%s", k, v))
		}
		sort.Strings(types)
		parser.Config = append(parser.Config, [2]string{"Types", strings.Join(types, " ")})
	}

	return parser, parserName
}

func ParserFilterComponent(tag string, field string, parserNames []string) Component {
	filter := Component{
		Kind: "FILTER",
		Config: [][2]string{
			{"Name", "parser"},
			{"Match", tag},
		},
	}

	if field != "" {
		filter.Config = append(filter.Config, [2]string{"Key_Name", field})
	} else {
		filter.Config = append(filter.Config, [2]string{"Key_Name", "message"})
	}

	for _, parser := range parserNames {
		filter.Config = append(filter.Config, [2]string{"Parser", parser})
	}

	return filter
}
