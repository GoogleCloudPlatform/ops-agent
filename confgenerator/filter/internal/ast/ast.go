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
	"regexp"
	"strconv"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter/internal/token"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

type Attrib interface{}

type Member []string

var logEntryRootValueMapToFluentBit = map[string]string{
	"severity": "logging.googleapis.com/severity",
}

var logEntryRootStructMapToFluentBit = map[string]string{
	"labels":         "logging.googleapis.com/labels",
	"operation":      "logging.googleapis.com/operation",
	"sourceLocation": "logging.googleapis.com/sourceLocation",
	"httpRequest":    "logging.googleapis.com/http_request",
}

// RecordAccessor returns a string that can be used as a key in fluentbit config
func (m Member) RecordAccessor() string {
	s := `$record`
	for _, part := range m {
		unquoted, _ := Unquote(part)
		s = s + fmt.Sprintf(`['%s']`, strings.ReplaceAll(unquoted, `'`, `''`))
	}
	return s
}

// logEntryToFluentBit translates a Member from a LogEntry model to a FluentBit model
func (m Member) logEntryToFluentBit() (Member, error) {
	var fluentbit Member
	if len(m) == 1 {
		if v, ok := logEntryRootValueMapToFluentBit[m[0]]; ok {
			fluentbit = Member{v}
		}
	} else if len(m) > 1 {
		if v, ok := logEntryRootStructMapToFluentBit[m[0]]; ok {
			fluentbit = prepend(v, m[1:])
		} else if m[0] == "jsonPayload" {
			// Special case for jsonPayload, where the root "jsonPayload" must be omitted
			fluentbit = m[1:]
		}
	}
	if fluentbit == nil {
		return nil, fmt.Errorf("unrecognized LogEntry path: %v", strings.Join(m, "."))
	}
	for _, part := range fluentbit {
		unquoted, err := Unquote(part)
		if err != nil {
			return Member{}, err
		}
		// Disallowed characters because they cannot be encoded in a Record Accessor.
		// \r is allowed in a Record Accessor, but we disallow it to avoid issues on Windows.
		// (interestingly, \f and \v work fine...)
		if strings.ContainsAny(unquoted, "\n\r\", ") {
			return nil, fmt.Errorf(`path may not contain line breaks, spaces, commas, or double-quotes: %s`, part)
		}
	}
	return fluentbit, nil
}

func prepend(value string, slice []string) []string {
	return append([]string{value}, slice...)
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
		// Eager validation
		_, err := lhs.logEntryToFluentBit()
		if err != nil {
			return nil, err
		}
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
		// Eager validation
		switch r.Operator {
		case ":", "=", "!=":
			_, err := Unquote(rhs[0])
			if err != nil {
				return nil, err
			}
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

func cond(ctype string, values ...string) string {
	return fmt.Sprintf("%s %s", ctype, strings.Join(values, " "))
}

func escapeWhitespace(s string) string {
	s = strings.ReplaceAll(s, "\a", `\a`)
	s = strings.ReplaceAll(s, "\b", `\x08`)
	s = strings.ReplaceAll(s, "\f", `\f`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	s = strings.ReplaceAll(s, "\v", `\v`)
	s = strings.ReplaceAll(s, " ", `\x20`)
	return s
}

func (r Restriction) Components(tag, key string) []fluentbit.Component {
	c := modify(tag, key)
	lhsMember, _ := r.LHS.logEntryToFluentBit()
	lhs := lhsMember.RecordAccessor()
	rhs := r.RHS
	rhsLiteral, _ := Unquote(rhs)
	rhsLiteral = escapeWhitespace(regexp.QuoteMeta(rhsLiteral))
	rhsRegex := escapeWhitespace(rhs)
	switch r.Operator {
	case "GLOBAL", "<", "<=", ">", ">=":
		panic(fmt.Errorf("unimplemented operator: %s", r.Operator))
	case ":":
		// substring match
		c.Config["Condition"] = cond("Key_value_matches", lhs, fmt.Sprintf(`.*%s.*`, rhsLiteral))
	case "=~":
		// regex match
		c.Config["Condition"] = cond("Key_value_matches", lhs, rhsRegex)
	case "!~":
		c.OrderedConfig = append(c.OrderedConfig, [2]string{"Condition", cond("Key_value_does_not_match", lhs, rhsRegex)})
		// TODO: Remove when https://github.com/fluent/fluent-bit/issues/4319 is resolved.
		c.OrderedConfig = append(c.OrderedConfig, [2]string{"Condition", cond("Key_exists", lhs)})
	case "=":
		// equality
		c.Config["Condition"] = cond("Key_value_matches", lhs, fmt.Sprintf(`(?i)^%s$`, rhsLiteral))
	case "!=":
		c.OrderedConfig = append(c.OrderedConfig, [2]string{"Condition", cond("Key_value_does_not_match", lhs, fmt.Sprintf(`(?i)^%s$`, rhsLiteral))})
		// TODO: Remove when https://github.com/fluent/fluent-bit/issues/4319 is resolved.
		c.OrderedConfig = append(c.OrderedConfig, [2]string{"Condition", cond("Key_exists", lhs)})
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
			"Condition", cond("Key_exists", subkey),
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
		m.Config["Condition"] = cond("Key_exists", subkey)
		components = append(components, m)
	}
	return components
}

type Negation struct {
	Expression
}

func (n Negation) Simplify() Expression {
	return Negation{n.Expression.Simplify()}
}

func (n Negation) Components(tag, key string) []fluentbit.Component {
	subkey := fmt.Sprintf("%s_0", key)
	components := n.Expression.Components(tag, subkey)
	m := modify(tag, key)
	m.Config["Condition"] = cond("Key_does_not_exist", subkey)
	components = append(components, m)
	return components
}

func Unquote(in string) (string, error) {
	var buf strings.Builder
	buf.Grow(3 * len(in) / 2)

	r := strings.NewReader(in)
	for {
		c, _, err := r.ReadRune()
		if err != nil {
			// EOF is only possible error
			break
		}
		if c != '\\' {
			buf.WriteRune(c)
			continue
		}
		c, _, err = r.ReadRune()
		if err != nil {
			buf.WriteRune('\\')
			break
		}
		switch c {
		case ',', ':', '=', '<', '>', '+', '~', '"', '\\', '.', '*':
			// escaped_char
			buf.WriteRune(c)
		case 'u':
			// unicode_esc
			// [0-9a-f]{4}
			digits := make([]byte, 4)
			n, _ := r.Read(digits)
			digits = digits[:n]
			codepoint, err := strconv.ParseUint(string(digits), 16, 16)
			if n < 4 || err != nil {
				buf.WriteRune('\\')
				buf.WriteRune('u')
				buf.Write(digits)
				break
			}
			buf.WriteRune(rune(codepoint))
		case '0', '1', '2', '3', '4', '5', '6', '7':
			// octal_esc
			// [0-7]{1,2} or [0-3],[0-7]{2}
			digits := []byte{byte(c)}
			for len(digits) < 3 {
				c, err := r.ReadByte()
				if err != nil {
					break
				}
				if c < '0' || c > '7' {
					r.UnreadByte()
					break
				}
				digits = append(digits, c)
				if digits[0] > '3' && len(digits) == 2 {
					break
				}
			}
			codepoint, err := strconv.ParseUint(string(digits), 8, 8)
			if err != nil {
				buf.WriteRune('\\')
				buf.Write(digits)
				break
			}
			buf.WriteRune(rune(codepoint))
		case 'x':
			// hex_esc:
			// 2*hex_digit
			digits := make([]byte, 2)
			n, _ := r.Read(digits)
			digits = digits[:n]
			codepoint, err := strconv.ParseUint(string(digits), 16, 8)
			if n < 2 || err != nil {
				buf.WriteRune('\\')
				buf.WriteRune('x')
				buf.Write(digits)
				break
			}
			buf.WriteRune(rune(codepoint))
		case 'a':
			buf.WriteRune('\a')
		case 'b':
			buf.WriteRune('\b')
		case 'f':
			buf.WriteRune('\f')
		case 'n':
			buf.WriteRune('\n')
		case 'r':
			buf.WriteRune('\r')
		case 't':
			buf.WriteRune('\t')
		case 'v':
			buf.WriteRune('\v')
		default:
			return "", fmt.Errorf(`invalid escape sequence: \%s`, string(c))
		}
	}
	return buf.String(), nil
}

func ParseText(a Attrib) (string, error) {
	str := string(a.(*token.Token).Lit)
	return str, nil
}
func ParseString(a Attrib) (string, error) {
	str := string(a.(*token.Token).Lit)
	// TODO: Support all escape sequences
	return str[1 : len(str)-1], nil
}
