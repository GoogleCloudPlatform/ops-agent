// Copyright 2022 Google LLC
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
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestUnquoteString(t *testing.T) {
	for in, out := range map[string]string{
		`\150\145\154\154\157`:     "\150\145\154\154\157",
		`\x48\x65\x6C\x6C\x6F`:     "Hello",
		`\150\145\u013E\u013E\157`: "\150\145\u013E\u013E\157",
		`sl\\as\\\\h`:              `sl\as\\h`,
		`\777`:                     `?7`,
		`\377`:                     "\u00FF",
		`\`:                        `\`,
		`☃`:                        `☃`,
	} {
		in, out := in, out
		t.Run(in, func(t *testing.T) {
			got, err := UnquoteString(in)
			if got != out {
				t.Errorf("got %q, want %q", got, out)
			}
			if err != nil {
				t.Error(err)
			}
		})
	}
}

func TestValidPath(t *testing.T) {
	for _, test := range []struct {
		in            Target
		want          string
		fluentBitPath []string
		ottlPath      []string
		ottlAccessor  string
	}{
		{
			Target{"jsonPayload", "hello"},
			"jsonPayload.hello",
			[]string{"hello"},
			[]string{"body", "hello"},
			`body["hello"]`,
		},
		{
			Target{`"json\u0050ayload"`, "hello"},
			"jsonPayload.hello",
			[]string{"hello"},
			[]string{"body", "hello"},
			`body["hello"]`,
		},
		{
			Target{"severity"},
			"severity",
			[]string{"logging.googleapis.com/severity"},
			[]string{"severity_text"},
			`severity_text`,
		},
		{
			Target{"httpRequest", "status"},
			"httpRequest.status",
			[]string{"logging.googleapis.com/httpRequest", "status"},
			[]string{"attributes", "gcp.http_request", "status"},
			`attributes["gcp.http_request"]["status"]`,
		},
		{
			Target{"sourceLocation", "line"},
			"sourceLocation.line",
			[]string{"logging.googleapis.com/sourceLocation", "line"},
			[]string{"attributes", "gcp.source_location", "line"},
			`attributes["gcp.source_location"]["line"]`,
		},
		{
			Target{"labels", "custom"},
			"labels.custom",
			[]string{"logging.googleapis.com/labels", "custom"},
			[]string{"attributes", "custom"},
			`attributes["custom"]`,
		},
		{
			Target{`jsonPayload`, `"escaped fields \a\b\f\n\r\t\v"`},
			`jsonPayload."escaped\u0020fields\u0020\a\b\f\n\r\t\v"`,
			[]string{"escaped fields \a\b\f\n\r\t\v"},
			[]string{"body", "escaped fields \a\b\f\n\r\t\v"},
			`body["escaped fields \a\b\f\n\r\t\v"]`,
		},
	} {
		test := test
		t.Run(test.in.String(), func(t *testing.T) {
			got := test.in.String()
			if diff := cmp.Diff(got, test.want); diff != "" {
				t.Errorf("unexpected target string (got -/want +):\n%s", diff)
			}
			gotPath, err := test.in.fluentBitPath()
			if err != nil {
				t.Errorf("got unexpected error: %v", err)
			}
			if diff := cmp.Diff(gotPath, test.fluentBitPath); diff != "" {
				t.Errorf("unexpected fluent-bit path (got -/want +):\n%s", diff)
			}
			gotPath, err = test.in.ottlPath()
			if err != nil {
				t.Errorf("got unexpected error: %v", err)
			}
			if diff := cmp.Diff(gotPath, test.ottlPath); diff != "" {
				t.Errorf("unexpected OTTL path (got -/want +):\n%s", diff)
			}
			gotAccessor, err := test.in.OTTLAccessor()
			if err != nil {
				t.Errorf("got unexpected error: %v", err)
			}
			if diff := cmp.Diff(gotAccessor, test.ottlAccessor); diff != "" {
				t.Errorf("unexpected OTTL accessor (got -/want +):\n%s", diff)
			}
		})
	}
}

func TestInvalidFluentBitPath(t *testing.T) {
	for _, test := range []Target{
		{"notJsonPayload"},
		{"jsonPayload"},
		{"sourceLocation"},
		{"jsonPayload", `"broken\descape"`},
	} {
		test := test
		t.Run(test.String(), func(t *testing.T) {
			got, err := test.fluentBitPath()
			if err == nil {
				t.Errorf("got unexpected success for %v: %+v", test, got)
			}
		})
	}
}
