// Copyright 2026 Google LLC
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

package fluentbit

import (
	"fmt"
	"strings"
	"testing"
)

func TestLuaQuote(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "simple_identifier",
			in:   "message",
			want: `"message"`,
		},
		{
			name: "double_quote_injection",
			in:   `message" os.execute("id") --`,
			want: `"message\" os.execute(\"id\") --"`,
		},
		{
			name: "backslash_and_newline_injection",
			in:   "message\"\\\nos.execute(\"id\")--",
			want: `"message\"\\\10os.execute(\"id\")--"`,
		},
		{
			name: "escape_followed_by_digit",
			in:   "line\n1_and_\x196",
			want: `"line\0101_and_\0256"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := LuaQuote(tc.in)
			if got != tc.want {
				t.Errorf("LuaQuote(%q) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

func TestParserFilterComponentsSanitizesParseKey(t *testing.T) {
	maliciousField := "message\"\nos.execute(\"touch /tmp/pwned\")--"
	components := ParserFilterComponents("test_tag", maliciousField, []string{"test_parser"}, false)

	var luaScript string
	for _, c := range components {
		if c.Kind == outputFileKind && strings.Contains(c.Config[outputFileContents], "function parser_nest") {
			luaScript = c.Config[outputFileContents]
			break
		}
	}
	if luaScript == "" {
		t.Fatalf("expected parser_nest Lua script output component, got none in %+v", components)
	}

	wantLine := fmt.Sprintf("local parseKey = %s", `"message\"\10os.execute(\"touch /tmp/pwned\")--"`)
	if !strings.Contains(luaScript, wantLine) {
		t.Errorf("expected Lua script to contain sanitized line %q, got:\n%s", wantLine, luaScript)
	}
	if strings.Contains(luaScript, "\nos.execute(") {
		t.Errorf("Lua script contains unescaped newline injection:\n%s", luaScript)
	}
}
