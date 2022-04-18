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
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter/internal/generated/lexer"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter/internal/generated/token"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/google/go-cmp/cmp"
)

var validFilters = []string{
	`severity = "hello"`,
	`jsonPayload."bar.baz" = "hello"`,
	`jsonPayload.b.c=~"b.*c"`,
	`"jsonPayload"."foo" = "bar"`,
	`-severity = 1`,
	`NOT severity = 3`,
	`(jsonPayload.bar = "one" OR jsonPayload.bar = "two") jsonPayload.baz = "three"`,
	`jsonPayload.one = 1 jsonPayload.two = 2 AND jsonPayload.three = 3`,
	`jsonPayload.int_field:0 OR jsonPayload.int_field:0 AND jsonPayload.int_field:0`,
	`jsonPayload.compound.string_field : wal\"rus`,
	`severity =~ "ERROR" AND jsonPayload.message =~ "foo" AND httpRequest.requestMethod =~ "GET"`,
	`severity = "AND"`,
	`severity = AND`,
	`severity = OR`,
	`severity = NOT`,
	`"json\u0050ayload".foo = bar`,
	`jsonPayload.\= = bar`,
	`jsonPayload."\=" = bar`,
}

func TestShouldLex(t *testing.T) {
	for _, test := range validFilters {
		test := test
		t.Run(test, func(t *testing.T) {
			l := lexer.NewLexer([]byte(test))
			for tok := l.Scan(); tok.Type != token.EOF; tok = l.Scan() {
				if tok.Type == token.INVALID {
					t.Errorf("got invalid token: %v", token.TokMap.TokenString(tok))
				}
				t.Logf("tok: %v", token.TokMap.TokenString(tok))
			}
		})
	}
}

func TestShouldParse(t *testing.T) {
	for _, test := range validFilters {
		test := test
		t.Run(test, func(t *testing.T) {
			filter, err := NewFilter(test)
			if err != nil {
				t.Error(err)
			}
			t.Logf("parsed filter = %s", filter)
			if filter == nil {
				t.Fatal("got nil filter")
			}
			components, expr := AllFluentConfig("logname", map[string]*Filter{"filter": filter})
			t.Logf("components = %+v", components)
			t.Logf("expression =\n%s", expr)
			if components != nil {
				files, err := fluentbit.ModularConfig{Components: components}.Generate()
				if err != nil {
					t.Error(err)
				}
				t.Logf("generated config:\n%v", files)
			}
		})
	}
}

func TestFilterRoundTrip(t *testing.T) {
	for _, test := range validFilters {
		test := test
		t.Run(test, func(t *testing.T) {
			filter, err := NewFilter(test)
			if err != nil {
				t.Fatal(err)
			}
			first := filter.String()
			filter2, err := NewFilter(first)
			if err != nil {
				t.Fatalf("failed to re-parse %q: %v", first, err)
			}
			second := filter2.String()
			if diff := cmp.Diff(second, first); diff != "" {
				t.Errorf("filter did not round-trip (second -, first+):\n%s", diff)
			}
		})
	}
}

func TestInvalidFilters(t *testing.T) {
	for _, test := range []string{
		`"missing operator"`,
		`invalid/characters*here`,
		`jsonPayload.foo =~ bareword`,
		`json\u0050ayload.foo = bar`,
	} {
		test := test
		t.Run(test, func(t *testing.T) {
			filter, err := NewFilter(test)
			if err != nil {
				t.Logf("got expected error %v", err)
				return
			}
			t.Errorf("invalid filter %q unexpectedly parsed: %v", test, filter)
		})
	}
}

func TestValidMembers(t *testing.T) {
	for _, test := range []struct {
		in   string
		want []string
	}{
		{`jsonPayload.foo`, []string{"jsonPayload", "foo"}},
		{`labels."logging.googleapis.com/foo"`, []string{"labels", "logging.googleapis.com/foo"}},
		{`severity`, []string{"severity"}},
		{`jsonPayload.\=`, []string{"jsonPayload", `\=`}},
		{`jsonPayload."\="`, []string{"jsonPayload", `=`}},
	} {
		test := test
		t.Run(test.in, func(t *testing.T) {
			member, err := NewMember(test.in)
			if err != nil {
				t.Fatal(err)
			}
			got, err := member.Unquote()
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(got, test.want); diff != "" {
				t.Errorf("incorrect parse (got -/want +):\n%s", diff)
			}
		})
	}
}
