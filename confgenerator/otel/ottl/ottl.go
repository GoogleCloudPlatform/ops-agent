// Copyright 2023 Google LLC
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

package ottl

import (
	"fmt"
	"slices"
	"strings"
)

type Statements []Statement

type Statement string

func statementf(format string, args ...any) Statement {
	return Statement(fmt.Sprintf(format, args...))
}

func statementsf(format string, args ...any) Statements {
	return Statements{statementf(format, args...)}
}

// Value represents something that has a value in OTTL (either LValue or RValue)
type Value interface {
	fmt.Stringer
}

// LValue represents a field (path) that can be written to.
type LValue []string

func (path LValue) String() string {
	parts := []string{path[0]}
	for _, p := range path[1:] {
		parts = append(parts, fmt.Sprintf(`[%q]`, p))
	}
	return strings.Join(parts, "")
}

// RValue represents an arbitrary expression that can be evaluated to a value.
type RValue string

func (v RValue) String() string {
	return string(v)
}

func valuef(format string, args ...any) Value {
	return RValue(fmt.Sprintf(format, args...))
}

func StringLiteral(v string) Value {
	return valuef(`%q`, v)
}

func IntLiteral(v int) Value {
	return valuef(`%d`, v)
}

func False() Value {
	return valuef("false")
}

func True() Value {
	return valuef("true")
}

func Nil() Value {
	return valuef("nil")
}

func (a LValue) Set(b Value) Statements {
	return a.SetIf(b, nil)
}

func (a LValue) SetIfNil(b Value) Statements {
	return a.SetIf(b, valuef(`%s == nil`, a))
}

func (a LValue) SetIf(b, condition Value) Statements {
	var condStr string
	if condition != nil {
		condStr = fmt.Sprintf(" where %s", condition)
	}
	var statements Statements
	if (slices.Equal(a, LValue{"trace_id.string"})) {
		// LogEntry's `trace` field is expected to contain a `projects/$foo/traces/` prefix.
		// Remove it if present.
		// replace_pattern is a statement, not a value, in OTTL, so we have to mutate a cached copy of the value.
		cache := LValue{"cache", "__setif_value"}
		statements = statements.Append(
			cache.Delete(),
			cache.Set(b),
			statementsf(
				`replace_pattern(%s, %q, %q)`,
				cache,
				`^projects/([^/]*)/traces/`,
				"",
			),
		)
		b = cache
	}
	statements = statements.Append(
		statementsf(`set(%s, %s)%s`, a, b, condStr),
	)
	if (slices.Equal(a, LValue{"severity_text"})) {
		// As a special case for severity_text, we need to zero out severity_number to make sure the text takes effect.
		// TODO: Add a unit test for this.
		statements = statements.Append(
			LValue{"severity_number"}.SetIf(IntLiteral(0), condition),
		)
	}
	return statements
}

func (a LValue) MergeMaps(source Value, strategy string) Statements {
	return a.MergeMapsIf(source, strategy, IsNotNil(source))
}

func (a LValue) MergeMapsIf(source Value, strategy string, condition Value) Statements {
	var condStr string
	if condition != nil {
		condStr = fmt.Sprintf(" where %s", condition)
	}
	return Statements{
		statementf(`merge_maps(%s, %s, %q)%s`, a, source, strategy, condStr),
	}
}

// IsPresent returns true if the field is recursively present (with any value)
func (a LValue) IsPresent() Value {
	var conditions []Value
	for i := 1; i <= len(a); i++ {
		conditions = append(conditions, IsNotNil(a[:i]))
	}
	return And(conditions...)
}

func ToString(a Value) Value {
	return valuef(`Concat([%s], "")`, a)
}

func ToInt(a Value) Value {
	return valuef(`Int(%s)`, a)
}

func ToFloat(a Value) Value {
	return valuef(`Double(%s)`, a)
}

func ToTime(a Value, strpformat string) Value {
	return valuef(`Time(%s, %q)`, a, strpformat)
}

func ParseJSON(a Value) Value {
	return valuef(`ParseJSON(%s)`, a)
}

func ConvertCase(a Value, toCase string) Value {
	return valuef(`ConvertCase(%s, %q)`, a, toCase)
}

func IsMatch(target Value, pattern string) Value {
	return valuef(`IsMatch(%s, %q)`, target, pattern)
}

func Equals(a, b Value) Value {
	return valuef(`%s == %s`, a, b)
}

func Not(a Value) Value {
	return valuef(`(not %s)`, a)
}

func And(conditions ...Value) Value {
	var out []string
	for _, c := range conditions {
		out = append(out, c.String())
	}
	return valuef(`(%s)`, strings.Join(out, " and "))
}

func Or(conditions ...Value) Value {
	var out []string
	for _, c := range conditions {
		out = append(out, c.String())
	}
	return valuef(`(%s)`, strings.Join(out, " or "))
}

func IsNotNil(a Value) Value {
	return valuef(`%s != nil`, a)
}

// ExtractCountMetric creates a new metric based on the count value of a Histogram metric
func ExtractCountMetric(monotonic bool, metricName string) Statements {
	monotonicStr := "false"
	if monotonic {
		monotonicStr = "true"
	}
	return Statements{
		statementf(`extract_count_metric(%s) where name == "%s"`, monotonicStr, metricName),
	}
}

func (a LValue) SetToBool(b Value) Statements {
	// https://github.com/fluent/fluent-bit/blob/fd402681ad0ca0427395b07bb8a37c7c1c846cca/src/flb_parser.c#L1261
	// "true" = true, "false" = false, else error
	var out Statements
	if a.String() != b.String() {
		out = append(out, statementf(`set(%s, %s)`, a, b))
	}
	out = append(out,
		statementf(`set(%s, true) where %s == "true"`, a, b),
		statementf(`set(%s, false) where %s == "false"`, a, b),
	)
	return out
}

// Delete removes a (potentially nested) key from its parent maps, if that key exists.
func (a LValue) Delete() Statements {
	parent := a[:len(a)-1]
	child := a[len(a)-1]
	return Statements{
		statementf(`delete_key(%s, %q) where %s`, parent, child, a.IsPresent()),
	}
}

// Delete removes a (potentially nested) key from its parent maps, if that key exists.
func (a LValue) DeleteIf(cond Value) Statements {
	parent := a[:len(a)-1]
	child := a[len(a)-1]
	return Statements{
		statementf(`delete_key(%s, %q) where %s`, parent, child, And(a.IsPresent(), cond)),
	}
}

func (a LValue) KeepKeys(keys ...string) Statements {
	var quotedKeys []string
	for _, k := range keys {
		quotedKeys = append(
			quotedKeys,
			fmt.Sprintf("%q", k),
		)
	}
	return statementsf(
		`keep_keys(%s, [%s])`,
		a,
		strings.Join(quotedKeys, ", "),
	)
}

func NewStatements(a ...Statements) Statements {
	return Statements{}.Append(a...)
}

func (a Statements) Append(b ...Statements) Statements {
	for _, c := range b {
		a = append(a, c...)
	}
	return a
}
