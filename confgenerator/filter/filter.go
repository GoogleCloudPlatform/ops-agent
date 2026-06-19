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

//go:generate gocc -a -o internal/generated internal/filter.bnf
// To install gocc: go get github.com/goccmack/gocc

package filter

import (
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter/internal/ast"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter/internal/generated/lexer"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter/internal/generated/parser"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel/ottl"
)

type Member struct {
	ast.Target
}

func NewMember(m string) (*Member, error) {
	lex := lexer.NewLexer([]byte(m))
	p := parser.NewParser()
	out, err := p.Parse(lex)
	if err != nil {
		return nil, err
	}
	r, ok := out.(ast.Restriction)
	if !ok || r.Operator != "GLOBAL" {
		return nil, fmt.Errorf("not a field: %#v", out)
	}
	return &Member{r.LHS}, nil
}

// NewMemberLegacy attempts to parse m as a filter member.
// If it fails, it prepends `jsonPayload.` and tries again.
// This is used by legacy config options that allowed bare body field names.
func NewMemberLegacy(m string) (*Member, error) {
	out, err := NewMember(m)
	if err != nil {
		// Bare fields in legacy configs refer to elements of `jsonPayload`; try parsing with a `jsonPayload.` prefix if it fails to parse
		out, err := NewMember(fmt.Sprintf("jsonPayload.%s", m))
		if err == nil {
			return out, nil
		}
	}
	return out, err
}

// Equals checks if two valid members are equal.
// Invalid members are never equal.
func (m Member) Equals(m2 Member) bool {
	return m.Target.Equals(m2.Target)
}

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

func (f Filter) OTTLExpression() (ottl.Value, error) {
	return f.expr.OTTLExpression()
}

func (f Filter) HasRubyRegex() bool {
	return f.expr.HasRubyRegex()
}

func (f Filter) String() string {
	return f.expr.String()
}

// MatchesAny returns a single Filter that matches if any of the child filters match.
func MatchesAny(filters []*Filter) *Filter {
	d := ast.Disjunction{}
	for _, f := range filters {
		d = append(d, f.expr)
	}
	return &Filter{expr: d}
}

func SpecialFields() map[string]string {
	return ast.SpecialFields()
}
