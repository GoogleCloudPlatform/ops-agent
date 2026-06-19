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

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter/internal/generated/token"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel/ottl"
	"go.uber.org/multierr"
)

type Attrib interface{}

// Cloud Logging's filter syntax has extremely weird rules around quoting/escaping
//
// A "text" is a literal that is not surrounded by double quotes. A
// text may contain a small number of special characters, must start
// with a non-digit, and can contain some but not all escape
// sequences. However, if it does contain escape sequences, they are
// NOT unescaped before matching.
//
// A "string" is a literal that IS surrounded by double quotes. A
// string can contain a larger number of special characters and escape
// sequences, and the escape sequences ARE unescaped. However, if it
// is used for a regex, the escape sequences are unescaped by the
// regex engine, not separately as part of filter parsing.

// Target represents member from the filter BNF, and represents either a value or a dotted field path.
// Each element of the slice is not yet unescaped (if needed).
type Target []string

var logEntryRootValueMapToOTel = map[string][]string{
	"severity": {"severity_text"},
	"logName":  {"attributes", "gcp.log_name"},
	// The "trace" field has the format `projects/$project/traces/$trace`. The `trace_id.string` field has the format `$trace` and does not support a project prefix.
	// N.B. While these look like they're map elements, `trace_id["string"]` does not work. They must be referred to as `trace_id.string`.
	// N.B. See special handling of trace_id.string in ottl.go
	"trace":       {"trace_id.string"},
	"spanId":      {"span_id.string"},
	"textPayload": {"body"},
}

var logEntryRootStructMapToOTel = map[string][]string{
	"jsonPayload": {"body"},
	"labels":      {"attributes"},
	// "operation": {}, // TODO: Missing in OTel exporter
	"sourceLocation": {"attributes", "gcp.source_location"},
	"httpRequest":    {"attributes", "gcp.http_request"},
}

var specialFieldsMap = map[string]string{
	"logging.googleapis.com/severity":       "severity",
	"logging.googleapis.com/logName":        "logName",
	"logging.googleapis.com/trace":          "trace",
	"logging.googleapis.com/spanId":         "spanId",
	"logging.googleapis.com/labels":         "labels",
	"logging.googleapis.com/operation":      "operation",
	"logging.googleapis.com/sourceLocation": "sourceLocation",
	"logging.googleapis.com/httpRequest":    "httpRequest",
}

func SpecialFields() map[string]string {
	out := map[string]string{}
	for k, v := range specialFieldsMap {
		if _, ok := logEntryRootValueMapToOTel[v]; ok {
			out[k] = v
		} else if _, ok := logEntryRootStructMapToOTel[v]; ok {
			out[k] = v
		}
	}
	return out
}

func (m Target) ottlPath() ([]string, error) {
	unquoted, err := m.Unquote()
	if err != nil {
		return nil, err
	}
	var otel []string
	if len(unquoted) == 1 {
		if v, ok := logEntryRootValueMapToOTel[unquoted[0]]; ok {
			otel = v
		}
	}
	if len(unquoted) >= 1 {
		if unquoted[0] == "sourceLocation" && len(unquoted) > 1 && unquoted[1] == "function" {
			unquoted[1] = "func"
		}
		if v, ok := logEntryRootStructMapToOTel[unquoted[0]]; ok {
			otel = append(v, unquoted[1:]...)
		}
	}
	if otel == nil {
		return nil, fmt.Errorf("field %q not found", strings.Join(m, "."))
	}
	return otel, nil
}

// Equals checks if two valid targets are equal.
// Invalid targets are never equal.
func (m Target) Equals(m2 Target) bool {
	s1, err := m.ottlPath()
	if err != nil {
		return false
	}
	s2, err := m2.ottlPath()
	if err != nil {
		return false
	}
	if len(s1) != len(s2) {
		return false
	}
	for i := range s1 {
		if s1[i] != s2[i] {
			return false
		}
	}
	return true
}

// OTTLAccessor returns a string that can be used to refer to the field in OTTL
func (m Target) OTTLAccessor() (ottl.LValue, error) {
	otel, err := m.ottlPath()
	if err != nil {
		return nil, err
	}
	return ottl.LValue(otel), nil
}

const (
	filterStartChar  = `#$%&'*/;?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[]^_` + "`" + `abcdefghijklmnopqrstuvwxyz{|}`
	filterMidChar    = filterStartChar + `0123456789+-`
	filterStringChar = filterMidChar + `!(),.:<=>~`
)

func escapeFilterString(in string) string {
	var needQuotes bool
	var b strings.Builder
	for i, c := range in {
		if i == 0 {
			if strings.ContainsRune(filterStartChar, c) {
				b.WriteRune(c)
				continue
			}
			needQuotes = true
		}
		if strings.ContainsRune(filterMidChar, c) {
			b.WriteRune(c)
			continue
		}
		needQuotes = true
		if strings.ContainsRune(filterStringChar, c) {
			b.WriteRune(c)
		} else if c == '\a' {
			b.WriteString(`\a`)
		} else if c == '\b' {
			b.WriteString(`\b`)
		} else if c == '\f' {
			b.WriteString(`\f`)
		} else if c == '\n' {
			b.WriteString(`\n`)
		} else if c == '\r' {
			b.WriteString(`\r`)
		} else if c == '\t' {
			b.WriteString(`\t`)
		} else if c == '\v' {
			b.WriteString(`\v`)
		} else {
			fmt.Fprintf(&b, `\u%04X`, c)
		}
	}
	if needQuotes {
		return fmt.Sprintf(`"%s"`, b.String())
	}
	return b.String()
}

func (m Target) Unquote() ([]string, error) {
	var unquoted []string
	for _, part := range m {
		p, err := UnquoteTextOrString(part)
		if err != nil {
			return nil, err
		}
		unquoted = append(unquoted, p)
	}
	return unquoted, nil
}

// String formats a target as a valid expression
func (m Target) String() string {
	var out []string
	unquoted, err := m.Unquote()
	if err != nil {
		return fmt.Sprintf("UNPARSABLE TARGET %#v", m)
	}
	for _, s := range unquoted {
		out = append(out, escapeFilterString(s))
	}
	return strings.Join(out, ".")
}

func prepend(value string, slice []string) []string {
	return append([]string{value}, slice...)
}

type Restriction struct {
	Operator string
	// LHS contains the field being matched
	LHS Target
	// RHS contains the string to match against; for regexes, this is a raw string including escape sequences (but always without double quotes).
	RHS string
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
	case Target:
		// Eager validation for OTel OTTL compatibility
		if _, err := lhs.OTTLAccessor(); err != nil {
			return nil, err
		}
		r.LHS = lhs
	default:
		return nil, fmt.Errorf("unknown lhs: %v", lhs)
	}
	switch rhs := rhs.(type) {
	case nil:
	case Target:
		// BNF parses values as Target, even if they are singular
		if len(rhs) != 1 {
			return nil, fmt.Errorf("unexpected rhs: %v", rhs)
		}
		// Perform the appropriate unquoting depending on what operator is being used.
		switch r.Operator {
		case "=~", "!~":
			rhs := rhs[0]
			// Regular expressions must be string, not text, and we need to preserve the original escaped text for the regex engine.
			if rhs[0] != byte('"') || rhs[len(rhs)-1] != byte('"') {
				return nil, fmt.Errorf("regular expressions must begin and end with '\"', token %q", rhs)
			}
			r.RHS = rhs[1 : len(rhs)-1]
		default:
			rhs, err := UnquoteTextOrString(rhs[0])
			if err != nil {
				return nil, err
			}
			r.RHS = rhs
		}
	default:
		return nil, fmt.Errorf("unknown rhs: %v", rhs)
	}
	return &r, nil
}

func (r Restriction) Simplify() Expression {
	return r
}

func (r Restriction) String() string {
	if r.Operator == "GLOBAL" {
		return escapeFilterString(r.RHS)
	}
	switch r.Operator {
	case "=~", "!~":
		return fmt.Sprintf(`%s %s "%s"`, r.LHS, r.Operator, r.RHS)
	}
	return fmt.Sprintf(`%s %s %s`, r.LHS, r.Operator, escapeFilterString(r.RHS))
}

func (r Restriction) OTTLExpression() (ottl.Value, error) {
	lhs, _ := r.LHS.OTTLAccessor()

	// TODO: Add support for numeric comparisons

	var expr ottl.Value

	switch r.Operator {
	case "GLOBAL", "<", "<=", ">", ">=":
		return nil, fmt.Errorf("unimplemented operator: %s", r.Operator)
	case ":":
		// substring match, case insensitive
		expr = ottl.IsMatch(lhs, fmt.Sprintf(`(?i)%s`, regexp.QuoteMeta(r.RHS)))
	case "=~", "!~":
		// regex match, case sensitive

		// TODO: b/436898109 - Enable regex validity config checks when Ruby Regex library
		// is added to the Ops Agent build. This requires "CGO_ENABLED=1".

		expr = ottl.IsMatch(lhs, r.RHS)
		if _, err := regexp.Compile(r.RHS); err != nil {
			expr = ottl.IsMatchRubyRegex(lhs, r.RHS)
		}

		if r.Operator == "!~" {
			expr = ottl.Not(expr)
		}
	case "=", "!=":
		// equality, case insensitive
		expr = ottl.IsMatch(lhs, fmt.Sprintf(`(?i)^%s$`, regexp.QuoteMeta(r.RHS)))
		if r.Operator == "!=" {
			expr = ottl.Not(expr)
		}
	}
	if expr != nil {
		// All comparisons involving a missing field are false
		return ottl.And(lhs.IsPresent(), expr), nil
	}
	// This is all the supported operators.
	return nil, fmt.Errorf("unknown operator: %s", r.Operator)
}

func (r Restriction) HasRubyRegex() bool {
	switch r.Operator {
	case "=~", "!~":
		_, err := regexp.Compile(r.RHS)
		return err != nil
	}
	return false
}

type Expression interface {
	// Simplify returns a logically equivalent Expression.
	Simplify() Expression

	// OTTLExpression returns an OTTL value that can be used to evaluate the expression.
	OTTLExpression() (ottl.Value, error)

	HasRubyRegex() bool

	fmt.Stringer
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

type exprSlice []Expression

func (s exprSlice) OTTLExpression(operator func(...ottl.Value) ottl.Value) (ottl.Value, error) {
	var values []ottl.Value
	var err error
	for _, e := range s {
		value, eerr := e.OTTLExpression()
		values = append(values, value)
		multierr.AppendInto(&err, eerr)
	}
	return operator(values...), err
}

func (s exprSlice) HasRubyRegex() bool {
	for _, e := range s {
		if e.HasRubyRegex() {
			return true
		}
	}
	return false
}

func (s exprSlice) String(operator string) string {
	var out []string
	for _, e := range s {
		out = append(out, e.String())
	}
	return fmt.Sprintf("(%s)", strings.Join(out, ") "+operator+" ("))
}

func (c Conjunction) OTTLExpression() (ottl.Value, error) {
	return exprSlice(c).OTTLExpression(ottl.And)
}

func (c Conjunction) HasRubyRegex() bool {
	return exprSlice(c).HasRubyRegex()
}

func (c Conjunction) String() string {
	return exprSlice(c).String("AND")
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

func (d Disjunction) OTTLExpression() (ottl.Value, error) {
	return exprSlice(d).OTTLExpression(ottl.Or)
}

func (d Disjunction) HasRubyRegex() bool {
	return exprSlice(d).HasRubyRegex()
}

func (d Disjunction) String() string {
	return exprSlice(d).String("OR")
}

type Negation struct {
	Expression
}

func (n Negation) Simplify() Expression {
	return Negation{n.Expression.Simplify()}
}

func (n Negation) OTTLExpression() (ottl.Value, error) {
	value, err := n.Expression.OTTLExpression()
	if err != nil {
		return nil, err
	}
	return ottl.Not(value), nil
}

func (n Negation) String() string {
	return fmt.Sprintf("NOT %s", n.Expression.String())
}

// UnquoteTextOrString returns text literals as-is and unquotes string literals.
func UnquoteTextOrString(in string) (string, error) {
	if in[0] == byte('"') {
		return UnquoteString(in[1 : len(in)-1])
	}
	return in, nil
}

// UnquoteString replaces all escape sequences with their respective characters that they represent.
//
// It assumes the leading and trailing double-quotes have been removed.
//
// Escape sequences are replaced if and only if they are defined in our grammar: confgenerator/filter/internal/filter.bnf.
// An error is returned if an unrecognized escape sequence is encountered.
//
// This is a compatibility layer to maintain parity with Cloud Logging query strings. strconv.Unquote cannot be used here
// because it follows escape rules for Go strings, and Cloud Logging strings are not Go strings.
func UnquoteString(in string) (string, error) {
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

func ParseTextOrString(a Attrib) (string, error) {
	str := string(a.(*token.Token).Lit)
	return str, nil
}
