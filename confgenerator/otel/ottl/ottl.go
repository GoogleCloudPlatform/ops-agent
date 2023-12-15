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
	"strings"
)

type Statements []Statement

type Statement string

func statementf(format string, args ...any) Statement {
	return Statement(fmt.Sprintf(format, args...))
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

func (a LValue) Set(b Value) Statements {
	return Statements{
		statementf(`set(%s, %s)`, a, b),
	}
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

func (a LValue) Delete() Statements {
	parent := a[:len(a)-1]
	child := a[len(a)-1]
	return Statements{
		statementf(`delete_key(%s, %q)`, parent, child),
	}
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
