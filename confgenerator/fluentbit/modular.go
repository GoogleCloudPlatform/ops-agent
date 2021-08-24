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
		if v != "" {
			lines = append(lines, fmt.Sprintf("    %-*s %s", maxLen, k, v))
		}
	}
	sort.Strings(lines)
	return fmt.Sprintf("[%s]\n%s", c.Kind, strings.Join(lines, "\n"))
}

type Pipeline struct {
	Components []Component
	LogName    string
	Internal   bool
}

type ModularConfig struct {
	Components []Component
	StateDir   string
	LogsDir    string
}

// Generate the FluentBit main and parser config files for c.
func (c ModularConfig) Generate() (mainConfig string, parserConfig string, err error) {
	parserComponents := []Component{}
	for _, p := range DefaultParsers {
		parserComponents = append(parserComponents, p.Component())
	}
	components := append(parserComponents, c.Components...)
	sectionMap := map[string][]string{}
	for _, o := range components {
		s := o.generateSection()
		sectionMap[o.Kind] = append(sectionMap[o.Kind], s)
	}
	mainConfigSections := []string{
		fmt.Sprintf(`@SET buffers_dir=%s/buffers
@SET logs_dir=%s`, c.StateDir, c.LogsDir),
	}
	mainConfigSections = append(mainConfigSections, sectionMap["SERVICE"]...)
	mainConfigSections = append(mainConfigSections, sectionMap["INPUT"]...)
	mainConfigSections = append(mainConfigSections, sectionMap["FILTER"]...)
	mainConfigSections = append(mainConfigSections, sectionMap["OUTPUT"]...)
	parserConfigSections := append(sectionMap["PARSER"], "")
	return strings.Join(mainConfigSections, "\n\n"), strings.Join(parserConfigSections, "\n\n"), nil
}
