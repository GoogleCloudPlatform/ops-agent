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

type Pipeline struct {
	Components []Component
	LogName    string
	Internal   bool
}

type ModularConfig struct {
	Pipelines      []Pipeline
	InternalOutput Component
	Output         Component
}
