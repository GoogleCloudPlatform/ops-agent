// Copyright 2025 Google LLC
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

package ottlfuncs

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
)

func Test_extractPatternsRubyRegex(t *testing.T) {
	tests := []struct {
		name            string
		target          ottl.StringGetter[any]
		pattern         string
		omitEmptyValues bool
		want            func(pcommon.Map)
	}{
		{
			name: "extract patterns",
			target: &ottl.StandardStringGetter[any]{
				Getter: func(_ context.Context, _ any) (any, error) {
					return `a=b c=d`, nil
				},
			},
			pattern: `^a=(?<a>\w+)\s+c=(?<c>\w+)$`,
			want: func(expectedMap pcommon.Map) {
				expectedMap.PutStr("a", "b")
				expectedMap.PutStr("c", "d")
			},
		},
		{
			name: "no pattern found",
			target: &ottl.StandardStringGetter[any]{
				Getter: func(_ context.Context, _ any) (any, error) {
					return `a=b c=d`, nil
				},
			},
			pattern: `^a=(?<a>\w+)$`,
			want:    func(_ pcommon.Map) {},
		},
		{
			name: "complex pattern",
			target: &ottl.StandardStringGetter[any]{
				Getter: func(_ context.Context, _ any) (any, error) {
					return `<13>1 2006-01-02T15:04:05+0700 vm_name_1 my_app_id n n n qqqqrrrr`, nil
				},
			},
			pattern: `^\<(?<pri>[0-9]{1,5})\>1 (?<time>[^ ]+) (?<host>[^ ]+) (?<ident>[^ ]+) (?<pid>[n0-9]+) (?<msgid>[^ ]+) (?<extradata>(\[(.*?)\]|n)) (?<message>.+)$`,
			want: func(expectedMap pcommon.Map) {
				expectedMap.PutStr("pri", "13")
				expectedMap.PutStr("time", "2006-01-02T15:04:05+0700")
				expectedMap.PutStr("host", "vm_name_1")
				expectedMap.PutStr("ident", "my_app_id")
				expectedMap.PutStr("pid", "n")
				expectedMap.PutStr("msgid", "n")
				expectedMap.PutStr("extradata", "n")
				expectedMap.PutStr("message", "qqqqrrrr")
			},
		},
		{
			name: "keep empty values",
			target: &ottl.StandardStringGetter[any]{
				Getter: func(_ context.Context, _ any) (any, error) {
					return `<13>1 2006-01-02T15:04:05+0700  my_app_id n n n `, nil
				},
			},
			pattern:         `^\<(?<pri>[0-9]{1,5})\>1 (?<time>[^ ]+) (?<host>.*) (?<ident>[^ ]+) (?<pid>[n0-9]+) (?<msgid>[^ ]+) (?<extradata>(\[(.*?)\]|n)) (?<message>.*)$`,
			omitEmptyValues: false,
			want: func(expectedMap pcommon.Map) {
				expectedMap.PutStr("pri", "13")
				expectedMap.PutStr("time", "2006-01-02T15:04:05+0700")
				expectedMap.PutStr("host", "")
				expectedMap.PutStr("ident", "my_app_id")
				expectedMap.PutStr("pid", "n")
				expectedMap.PutStr("msgid", "n")
				expectedMap.PutStr("extradata", "n")
				expectedMap.PutStr("message", "")
			},
		},
		{
			name: "omit empty values",
			target: &ottl.StandardStringGetter[any]{
				Getter: func(_ context.Context, _ any) (any, error) {
					return `<13>1 2006-01-02T15:04:05+0700  my_app_id n n n `, nil
				},
			},
			pattern:         `^\<(?<pri>[0-9]{1,5})\>1 (?<time>[^ ]+) (?<host>.*) (?<ident>[^ ]+) (?<pid>[n0-9]+) (?<msgid>[^ ]+) (?<extradata>(\[(.*?)\]|n)) (?<message>.*)$`,
			omitEmptyValues: true,
			want: func(expectedMap pcommon.Map) {
				expectedMap.PutStr("pri", "13")
				expectedMap.PutStr("time", "2006-01-02T15:04:05+0700")
				expectedMap.PutStr("ident", "my_app_id")
				expectedMap.PutStr("pid", "n")
				expectedMap.PutStr("msgid", "n")
				expectedMap.PutStr("extradata", "n")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exprFunc, err := extractPatternsRubyRegex(tt.target, tt.pattern, tt.omitEmptyValues)
			assert.NoError(t, err)

			result, err := exprFunc(context.Background(), nil)
			assert.NoError(t, err)

			resultMap, ok := result.(pcommon.Map)
			require.True(t, ok)

			expected := pcommon.NewMap()
			tt.want(expected)

			assert.Equal(t, expected.Len(), resultMap.Len())
			for k := range expected.All() {
				ev, _ := expected.Get(k)
				av, _ := resultMap.Get(k)
				assert.Equal(t, ev, av)
			}
		})
	}
}

func Test_extractPatternsRubyRegex_validation(t *testing.T) {
	tests := []struct {
		name            string
		target          ottl.StringGetter[any]
		pattern         string
		omitEmptyValues bool
	}{
		{
			name: "bad regex",
			target: &ottl.StandardStringGetter[any]{
				Getter: func(_ context.Context, _ any) (any, error) {
					return "foobar", nil
				},
			},
			pattern: "(",
		},
		{
			name: "no named capture group",
			target: &ottl.StandardStringGetter[any]{
				Getter: func(_ context.Context, _ any) (any, error) {
					return "foobar", nil
				},
			},
			pattern: "(.*)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exprFunc, err := extractPatternsRubyRegex[any](tt.target, tt.pattern, tt.omitEmptyValues)
			assert.Error(t, err)
			assert.Nil(t, exprFunc)
		})
	}
}

func Test_extractPatternsRubyRegex_bad_input(t *testing.T) {
	tests := []struct {
		name            string
		target          ottl.StringGetter[any]
		pattern         string
		omitEmptyValues bool
	}{
		{
			name: "target is non-string",
			target: &ottl.StandardStringGetter[any]{
				Getter: func(_ context.Context, _ any) (any, error) {
					return 123, nil
				},
			},
			pattern: "(?<line>.*)",
		},
		{
			name: "target is nil",
			target: &ottl.StandardStringGetter[any]{
				Getter: func(_ context.Context, _ any) (any, error) {
					return nil, nil
				},
			},
			pattern: "(?<line>.*)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exprFunc, err := extractPatternsRubyRegex[any](tt.target, tt.pattern, tt.omitEmptyValues)
			assert.NoError(t, err)

			result, err := exprFunc(nil, nil)
			assert.Error(t, err)
			assert.Nil(t, result)
		})
	}
}
