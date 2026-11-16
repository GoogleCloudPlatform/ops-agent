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
	"strings"
	"testing"
)

func TestParserFilterComponentsLuaQuoting(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		wantLine string
	}{
		{
			name:     "default_field",
			field:    "",
			wantLine: `local parseKey = "message"`,
		},
		{
			name:     "quoted_dotted_field",
			field:    `"my.field"`,
			wantLine: `local parseKey = "\"my.field\""`,
		},
		{
			name:     "quoted_backslash_field",
			field:    `"foo\\"`,
			wantLine: `local parseKey = "\"foo\\\\\""`,
		},
		{
			name:     "tab_character",
			field:    "\"foo\tbar\"",
			wantLine: `local parseKey = "\"foo\9bar\""`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			components := ParserFilterComponents("test_tag", tc.field, []string{"test_parser"}, false)
			var luaSrc string
			for _, c := range components {
				if c.Kind == outputFileKind && strings.Contains(c.Config[outputFileContents], "function parser_nest") {
					luaSrc = c.Config[outputFileContents]
					break
				}
			}
			if luaSrc == "" {
				t.Fatalf("parser_nest Lua output component not found")
			}
			if !strings.Contains(luaSrc, tc.wantLine) {
				t.Errorf("generated Lua missing expected quoted parseKey.\ngot:\n%s\nwant substring:\n%s", luaSrc, tc.wantLine)
			}
		})
	}
}
