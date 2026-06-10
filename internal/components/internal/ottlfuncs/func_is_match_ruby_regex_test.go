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

func Test_isMatchRubyRegex(t *testing.T) {
	tests := []struct {
		name     string
		target   ottl.StringLikeGetter[any]
		pattern  string
		expected bool
	}{
		{
			name: "ruby regex match false",
			target: &ottl.StandardStringLikeGetter[any]{
				Getter: func(context.Context, any) (any, error) {
					return "rustlang", nil
				},
			},
			pattern:  "(?<=go)lang",
			expected: false,
		},
		{
			name: "ruby regex match true",
			target: &ottl.StandardStringLikeGetter[any]{
				Getter: func(context.Context, any) (any, error) {
					return "golang", nil
				},
			},
			pattern:  "(?<=go)lang",
			expected: true,
		},
		{
			name: "replace match true",
			target: &ottl.StandardStringLikeGetter[any]{
				Getter: func(context.Context, any) (any, error) {
					return "hello world", nil
				},
			},
			pattern:  "hello.*",
			expected: true,
		},
		{
			name: "replace match false",
			target: &ottl.StandardStringLikeGetter[any]{
				Getter: func(context.Context, any) (any, error) {
					return "goodbye world", nil
				},
			},
			pattern:  "hello.*",
			expected: false,
		},
		{
			name: "replace match complex",
			target: &ottl.StandardStringLikeGetter[any]{
				Getter: func(context.Context, any) (any, error) {
					return "-12.001", nil
				},
			},
			pattern:  "[-+]?\\d*\\.\\d+([eE][-+]?\\d+)?",
			expected: true,
		},
		{
			name: "target bool",
			target: &ottl.StandardStringLikeGetter[any]{
				Getter: func(context.Context, any) (any, error) {
					return true, nil
				},
			},
			pattern:  "true",
			expected: true,
		},
		{
			name: "target int",
			target: &ottl.StandardStringLikeGetter[any]{
				Getter: func(context.Context, any) (any, error) {
					return int64(1), nil
				},
			},
			pattern:  `\d`,
			expected: true,
		},
		{
			name: "target float",
			target: &ottl.StandardStringLikeGetter[any]{
				Getter: func(context.Context, any) (any, error) {
					return 1.1, nil
				},
			},
			pattern:  `\d\.\d`,
			expected: true,
		},
		{
			name: "target pcommon.Value",
			target: &ottl.StandardStringLikeGetter[any]{
				Getter: func(context.Context, any) (any, error) {
					v := pcommon.NewValueEmpty()
					v.SetStr("test")
					return v, nil
				},
			},
			pattern:  `test`,
			expected: true,
		},
		{
			name: "nil target",
			target: &ottl.StandardStringLikeGetter[any]{
				Getter: func(context.Context, any) (any, error) {
					return nil, nil
				},
			},
			pattern:  "impossible to match",
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exprFunc, err := isMatchRubyRegex(tt.target, tt.pattern)
			assert.NoError(t, err)
			result, err := exprFunc(t.Context(), nil)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_isMatchRubyRegex_validation(t *testing.T) {
	target := &ottl.StandardStringLikeGetter[any]{
		Getter: func(context.Context, any) (any, error) {
			return "anything", nil
		},
	}
	_, err := isMatchRubyRegex[any](target, "[z-a]")
	require.Error(t, err)
}

func Test_isMatchRubyRegex_error(t *testing.T) {
	target := &ottl.StandardStringLikeGetter[any]{
		Getter: func(context.Context, any) (any, error) {
			return make(chan int), nil
		},
	}
	exprFunc, err := isMatchRubyRegex[any](target, "test")
	assert.NoError(t, err)
	_, err = exprFunc(t.Context(), nil)
	require.Error(t, err)
}
