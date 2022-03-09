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
)

var validFilters = []string{
	`severity = "hello"`,
	`jsonPayload."bar.baz" = "hello"`,
	`jsonPayload.b.c=~"b.*c"`,
	`-severity = 1`,
	`NOT severity = 3`,
	`(jsonPayload.bar = "one" OR jsonPayload.bar = "two") jsonPayload.baz = "three"`,
	`jsonPayload.one = 1 jsonPayload.two = 2 AND jsonPayload.three = 3`,
	`jsonPayload.int_field:0 OR jsonPayload.int_field:0 AND jsonPayload.int_field:0`,
	`jsonPayload.compound.string_field : wal\"rus`,
	`severity =~ ERROR AND jsonPayload.message =~ foo AND httpRequest.requestMethod =~ GET`,
	`severity = "AND"`,
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
			components, expr := AllFluentConfig("logname", map[string]*Filter{"filter": filter})
			t.Logf("components = %+v", components)
			t.Logf("expression =\n%s", expr)
			files, err := fluentbit.ModularConfig{Components: components}.Generate()
			if err != nil {
				t.Error(err)
			}
			t.Logf("generated config:\n%v", files)
		})
	}
}
