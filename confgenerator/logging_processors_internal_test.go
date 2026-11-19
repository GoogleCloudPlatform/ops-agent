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

package confgenerator

import (
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

func TestParserFilterComponentsQuotesParseKey(t *testing.T) {
	for _, test := range []struct {
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
		{
			// A quote that would end the Lua string, followed by more text and a
			// comment marker that would hide the rest of the line.
			name:     "quote_then_comment_marker",
			field:    `message" trailing("text") --`,
			wantLine: `local parseKey = "message\" trailing(\"text\") --"`,
		},
		{
			// A decimal escape that is followed by a digit has to be three digits long.
			name:     "tab_before_digit",
			field:    "foo\t1",
			wantLine: `local parseKey = "foo\0091"`,
		},
		{
			name:     "newline_before_digit",
			field:    "foo\n1",
			wantLine: `local parseKey = "foo\0101"`,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			components := parserFilterComponents("test_tag", test.field, []string{"test_parser"}, false)
			files, err := fluentbit.ModularConfig{Components: components}.Generate()
			if err != nil {
				t.Fatal(err)
			}
			var luaSrc string
			for _, contents := range files {
				if strings.Contains(contents, "function parser_nest") {
					luaSrc = contents
				}
			}
			if luaSrc == "" {
				t.Fatalf("parser_nest Lua script not found in %d generated files", len(files))
			}
			// Match the whole line, so that nothing can follow the quoted key on it.
			if !strings.Contains(luaSrc, "\n  "+test.wantLine+"\n") {
				t.Errorf("generated Lua is missing the quoted parseKey line.\ngot:\n%s\nwant line:\n  %s", luaSrc, test.wantLine)
			}
		})
	}
}
