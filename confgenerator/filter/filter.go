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

package filter

import (
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter/internal/ast"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter/internal/lexer"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter/internal/parser"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

type Filter struct {
	expr ast.Expression
}

func NewFilter(f string) (*Filter, error) {
	lex := lexer.NewLexer([]byte(f))
	p := parser.NewParser()
	out, err := p.Parse(lex)
	if err != nil {
		return nil, err
	}
	if out, ok := out.(ast.Expression); ok {
		return &Filter{out}, nil
	}
	return nil, fmt.Errorf("not an expression: %+v", out)
}

func (f *Filter) Components(tag string, isExclusionFilter bool) []fluentbit.Component {
	var parity string
	if isExclusionFilter {
		parity = "Exclude"
	} else {
		parity = "Regex"
	}
	c := []fluentbit.Component{{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":       "nest",
			"Match":      tag,
			"Operation":  "nest",
			"Nest_under": "record",
			"Wildcard":   "*",
		},
	}}
	match := fmt.Sprintf("__match_%s", tag)
	c = append(c, f.expr.Components(tag, match)...)
	c = append(c,
		fluentbit.Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":  "grep",
				"Match": tag,
				parity:  fmt.Sprintf("%s 1", match),
			},
		},
		fluentbit.Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":            "modify",
				"Match":           tag,
				"Remove_wildcard": match,
			},
		},
		fluentbit.Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":         "nest",
				"Match":        tag,
				"Operation":    "lift",
				"Nested_under": "record",
			},
		},
	)
	return c
}
