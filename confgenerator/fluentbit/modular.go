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
	// OrderedConfig is used for configuration pairs where the
	// key can appear in the output fluent bit config multiple times
	// and/or the order of the configuration provided is important
	OrderedConfig [][2]string
}

func (c Component) generateSection() string {
	var lines []string
	var maxLen int
	for k := range c.Config {
		if len(k) > maxLen {
			maxLen = len(k)
		}
	}
	for _, line := range c.OrderedConfig {
		if len(line[0]) > maxLen {
			maxLen = len(line[0])
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
	for _, line := range c.OrderedConfig {
		addLine(line[0], line[1])
	}

	return fmt.Sprintf("[%s]\n%s\n", c.Kind, strings.Join(lines, "\n"))
}

type ModularConfig struct {
	Variables  map[string]string
	Components []Component
}

const (
	outputFileKind     = "OPSAGENTOUTPUTFILE"
	outputFileName     = "filename"
	outputFileContents = "contents"
)

const (
	MainConfigFileName   = "fluent_bit_main.conf"
	ParserConfigFileName = "fluent_bit_parser.conf"
)

func outputFileComponent(name, contents string) Component {
	return Component{
		Kind: outputFileKind,
		Config: map[string]string{
			outputFileName:     name,
			outputFileContents: contents,
		},
	}
}

func (c ModularConfig) Generate() (map[string]string, error) {
	files := make(map[string]string)

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
		if c.Kind == outputFileKind {
			files[c.Config[outputFileName]] = c.Config[outputFileContents]
			continue
		}
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
		return nil, fmt.Errorf("unknown fluentbit config sections %+v", sectionsByKind)
	}
	files[MainConfigFileName] = strings.Join(parts, "\n")
	files[ParserConfigFileName] = strings.Join(parserParts, "\n")
	return files, nil
}
