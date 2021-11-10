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

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter/internal/lexer"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter/internal/token"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

var validFilters = []string{
	`"this is a simple quoted string"`,
	`foo."bar"`,
	`foo = "hello"`,
	`foo."bar.baz" = "hello"`,
	`a.b.c=~"b.*c"`,
	`-a = 1`,
	`NOT a = 3`,
	`(foo.bar = "one" OR foo.bar = "two") foo.baz = "three"`,
	`foo.one = 1 foo.two = 2 AND foo.three = 3`,
	`int_field:0 OR int_field:0 AND int_field:0`,
	`compound.string_field : wal\"rus`,
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
			components := filter.Components("logname", false)
			t.Logf("components = %+v", components)
			main, _, err := fluentbit.ModularConfig{Components: components}.Generate()
			if err != nil {
				t.Error(err)
			}
			t.Logf("generated config:\n%s", main)
		})
	}
}
