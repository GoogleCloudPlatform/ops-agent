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

package ast

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter/internal/token"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

type Attrib interface{}

type Member []string

// RecordAccessor returns a string that can be used as a key in fluentbit config
func (m Member) RecordAccessor() string {
	s := `$record`
	for _, part := range m {
		// TODO: Confirm this is the right escape
		s = s + fmt.Sprintf(`["%s"]`, strings.ReplaceAll(part, `"`, `\"`))
	}
	return s
}

type Restriction struct {
	Operator string
	LHS      Member
	RHS      string
}

func NewRestriction(lhs, operator, rhs Attrib) (*Restriction, error) {
	var r Restriction
	switch operator := operator.(type) {
	case string:
		r.Operator = operator
	case *token.Token:
		r.Operator = string(operator.Lit)
	default:
		return nil, fmt.Errorf("unknown operator: %v", operator)
	}
	switch lhs := lhs.(type) {
	case Member:
		r.LHS = lhs
	default:
		return nil, fmt.Errorf("unknown lhs: %v", lhs)
	}
	switch rhs := rhs.(type) {
	case nil:
	case Member:
		// BNF parses values as Member, even if they are singular
		if len(rhs) != 1 {
			return nil, fmt.Errorf("unexpected rhs: %v", rhs)
		}
		r.RHS = rhs[0]
	default:
		return nil, fmt.Errorf("unknown rhs: %v", rhs)
	}
	return &r, nil
}

func (r Restriction) Simplify() Expression {
	return r
}

func modify(tag, key string) fluentbit.Component {
	return fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":  "modify",
			"Match": tag,
			"Set":   fmt.Sprintf("%s 1", key),
		},
	}
}
func (r Restriction) Components(tag, key string) []fluentbit.Component {
	c := modify(tag, key)
	lhs := r.LHS.RecordAccessor()
	rhs := r.RHS
	lhsrhs := fmt.Sprintf(`%s %s`, lhs, rhs)
	switch r.Operator {
	case "GLOBAL":
		// Key exists
		c.Config["Key_exists"] = lhs
	case "<", "<=", ">", ">=":
		panic("unimplemented")
	case ":":
		// substring match
		// FIXME: Escape the regex
		c.Config["Key_value_matches"] = fmt.Sprintf(`%s .*%s.*`, lhs, rhs)
	case "=~":
		// regex match
		// FIXME: Escape
		c.Config["Key_value_matches"] = lhsrhs
	case "!~":
		// FIXME: Escape
		c.Config["Key_value_does_not_match"] = lhsrhs
	case "=":
		// equality
		// FIXME: Escape
		// FIXME: Non-string values
		c.Config["Key_value_equals"] = lhsrhs
	case "!=":
		// FIXME
		c.Config["Key_value_does_not_equal"] = lhsrhs
	}
	return []fluentbit.Component{c}
}

type Expression interface {
	// Simplify returns a logically equivalent Expression.
	Simplify() Expression

	// Components returns a sequence of fluentbit operations that
	// will set key if tagged records match this expression.
	Components(tag, key string) []fluentbit.Component
}

func Simplify(a Attrib) (Expression, error) {
	switch a := a.(type) {
	case Expression:
		return a.Simplify(), nil
	}
	return nil, fmt.Errorf("expected expression: %v", a)
}

// Conjunction represents an AND expression
type Conjunction []Expression

// Disjunction represents an OR expression
type Disjunction []Expression

func NewConjunction(a Attrib) (Conjunction, error) {
	switch a := a.(type) {
	case Conjunction:
		return a, nil
	case Expression:
		return Conjunction{a.Simplify()}, nil
	}
	return nil, fmt.Errorf("expected expression: %v", a)
}

func (c Conjunction) Simplify() Expression {
	if len(c) == 1 {
		return c[0]
	}
	return c
}

func (c Conjunction) Append(a Attrib) (Conjunction, error) {
	switch a := a.(type) {
	case Conjunction:
		return append(c, a...), nil
	case Expression:
		return append(c, a.Simplify()), nil
	}
	return nil, fmt.Errorf("expected expression: %v", a)
}

func (c Conjunction) Components(tag, key string) []fluentbit.Component {
	var components []fluentbit.Component
	m := modify(tag, key)
	for i, e := range c {
		subkey := fmt.Sprintf("%s_%d", key, i)
		components = append(components, e.Components(tag, subkey)...)
		m.OrderedConfig = append(m.OrderedConfig, [2]string{
			"Key_exists", subkey,
		})
	}
	components = append(components, m)
	return components
}

func NewDisjunction(a Attrib) (Disjunction, error) {
	switch a := a.(type) {
	case Disjunction:
		return a, nil
	case Expression:
		return Disjunction{a.Simplify()}, nil
	}
	return nil, fmt.Errorf("expected expression: %v", a)
}

func (d Disjunction) Simplify() Expression {
	if len(d) == 1 {
		return d[0]
	}
	return d
}

func (d Disjunction) Append(a Attrib) (Disjunction, error) {
	switch a := a.(type) {
	case Disjunction:
		return append(d, a...), nil
	case Expression:
		return append(d, a.Simplify()), nil
	}
	return nil, fmt.Errorf("expected expression: %v", a)
}

func (d Disjunction) Components(tag, key string) []fluentbit.Component {
	var components []fluentbit.Component
	var subkeys []string
	for i, e := range d {
		subkey := fmt.Sprintf("%s_%d", key, i)
		components = append(components, e.Components(tag, subkey)...)
		subkeys = append(subkeys, subkey)
	}
	// NB: We can't just pass key to e.Components because nested expressions might collide.
	for _, subkey := range subkeys {
		m := modify(tag, key)
		m.Config["Key_exists"] = subkey
		components = append(components, m)
	}
	return components
}

type Negation struct {
	Expression
}

func (n Negation) Simplify() Expression {
	return n
}

func ParseText(a Attrib) (string, error) {
	str := string(a.(*token.Token).Lit)
	// Add quotes
	str = `"` + str + `"`
	// TODO: Support all escape sequences
	return strconv.Unquote(str)
}
func ParseString(a Attrib) (string, error) {
	str := string(a.(*token.Token).Lit)
	// TODO: Support all escape sequences
	return strconv.Unquote(str)
}
