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
}

func (c Component) generateSection() string {
	var lines []string
	var maxLen int
	for k := range c.Config {
		if len(k) > maxLen {
			maxLen = len(k)
		}
	}
	for k, v := range c.Config {
		lines = append(lines, fmt.Sprintf("    %-*s %s", maxLen, k, v))
	}
	sort.Strings(lines)
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
	parserParts := sectionsByKind["PARSER"]
	delete(sectionsByKind, "PARSER")
	for _, k := range []string{"SERVICE", "INPUT", "FILTER", "OUTPUT"} {
		parts = append(parts, sectionsByKind[k]...)
		delete(sectionsByKind, k)
	}
	if len(sectionsByKind) > 0 {
		return "", "", fmt.Errorf("unknown fluentbit config sections %+v", sectionsByKind)
	}
	return strings.Join(parts, "\n"), strings.Join(parserParts, "\n"), nil
}
