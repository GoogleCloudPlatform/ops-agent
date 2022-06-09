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
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// ParseMultilineComponent constitutes the mulltiline_parser components.
func ParseMultilineComponent(tag string, uid string, languageRules []string) []Component {
	var components []Component
	multilineParserName := fmt.Sprintf("multiline.%s.%s", tag, uid)
	rules := [][2]string{}
	for _, rule := range languageRules {
		rules = append(rules, [2]string{"rule", rule})
	}

	multilineParser := Component{
		Kind: "MULTILINE_PARSER",
		Config: map[string]string{
			"name":          multilineParserName,
			"type":          "regex",
			"flush_timeout": "1000",
		},
		OrderedConfig: rules,
	}
	components = append(components, multilineParser)
	return components
}

// TranslationComponents translates SrcVal on key src to DestVal on key dest, if the dest key does not exist.
// If removeSrc is true, the original key is removed when translated.
func TranslationComponents(tag, src, dest string, removeSrc bool, translations []struct{ SrcVal, DestVal string }) []Component {
	c := []Component{}
	for _, t := range translations {
		comp := Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":      "modify",
				"Match":     tag,
				"Condition": fmt.Sprintf("Key_Value_Equals %s %s", src, t.SrcVal),
				"Add":       fmt.Sprintf("%s %s", dest, t.DestVal),
			},
		}

		if removeSrc {
			comp.Config["Remove"] = src
		}

		c = append(c, comp)
	}

	return c
}

// LuaFilterComponents returns components that execute the Lua script given in src on records that match tag.
// TODO(ridwanmsharif): Replace this with in-config script when
//   fluent/fluent-bit#4634 is supported.
func LuaFilterComponents(tag, function, src string) []Component {
	hasher := md5.New()
	hasher.Write([]byte(src))
	hash := hex.EncodeToString(hasher.Sum(nil))

	filename := fmt.Sprintf("%s.lua", hash)

	return []Component{
		{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":   "lua",
				"Match":  tag,
				"script": filename,
				"call":   function,
			},
		},
		outputFileComponent(filename, src),
	}
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

func ParserFilterComponents(tag string, field string, parserNames []string, preserveKey bool) []Component {
	parsers := [][2]string{}
	for _, name := range parserNames {
		parsers = append(parsers, [2]string{"Parser", name})
	}

	parseKey := "message"
	if field != "" {
		parseKey = field
	}

	nestFilters := LuaFilterComponents(tag, ParserNestLuaFunction, fmt.Sprintf(ParserNestLuaScriptContents, parseKey))
	filter := Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Match":    tag,
			"Name":     "parser",
			"Key_Name": parseKey, // Required
			// We need to preserve existing fields (like LogName) that are present
			// before parsing.
			"Reserve_Data": "True",
		},
		OrderedConfig: parsers,
	}

	if preserveKey {
		filter.Config["Preserve_Key"] = "True"
	}

	mergeFilters := LuaFilterComponents(tag, ParserMergeLuaFunction, ParserMergeLuaScriptContents)
	parseFilters := []Component{}
	parseFilters = append(parseFilters, nestFilters...)
	parseFilters = append(parseFilters, filter)
	parseFilters = append(parseFilters, mergeFilters...)

	return parseFilters
}
