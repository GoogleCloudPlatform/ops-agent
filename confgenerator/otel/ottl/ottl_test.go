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
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestLValueString(t *testing.T) {
	for _, test := range []struct {
		in   LValue
		want string
	}{
		{
			LValue{"body", "hello"},
			`body["hello"]`,
		},
		{
			LValue{"severity_text"},
			`severity_text`,
		},
		{
			LValue{"attributes", "gcp.http_request", "status"},
			`attributes["gcp.http_request"]["status"]`,
		},
		{
			LValue{"attributes", "gcp.source_location", "line"},
			`attributes["gcp.source_location"]["line"]`,
		},
		{
			LValue{"attributes", "custom"},
			`attributes["custom"]`,
		},
		{
			LValue{"body", "escaped fields \a\b\f\n\r\t\v"},
			`body["escaped fields \a\b\f\n\r\t\v"]`,
		},
	} {
		t.Run(fmt.Sprintf("%+v", []string(test.in)), func(t *testing.T) {
			got := test.in.String()
			if diff := cmp.Diff(got, test.want); diff != "" {
				t.Errorf("unexpected OTTL accessor (got -/want +):\n%s", diff)
			}
		})
	}
}
