// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
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
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
)

func Test_toValues(t *testing.T) {

	tests := []struct {
		name    string
		target  ottl.PSliceGetter[any]
		wantRaw []any
	}{
		{
			name: "a slice of maps with string values",
			target: ottl.StandardPSliceGetter[any]{
				Getter: func(context.Context, any) (any, error) {
					s := pcommon.NewSlice()

					m1 := s.AppendEmpty().SetEmptyMap()
					m1.PutStr("param1", "Software Protection")

					m2 := s.AppendEmpty().SetEmptyMap()
					m2.PutStr("param2", "stopped")
					return s, nil

				},
			},

			wantRaw: []any{"Software Protection", "stopped"},
		},
		{
			name: "a slice of maps, with entries in different order, to ensure order is preserved in the result",
			target: ottl.StandardPSliceGetter[any]{
				Getter: func(context.Context, any) (any, error) {
					s := pcommon.NewSlice()

					m1 := s.AppendEmpty().SetEmptyMap()
					m1.PutStr("param2", "stopped")

					m2 := s.AppendEmpty().SetEmptyMap()
					m2.PutStr("param1", "Software Protection")
					return s, nil

				},
			},

			wantRaw: []any{"stopped", "Software Protection"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exprFunc, err := toValues(tc.target)
			assert.NoError(t, err)
			gotSlice, err := exprFunc(nil, nil)
			assert.NoError(t, err)
			gotRaw := gotSlice.(pcommon.Slice).AsRaw()
			assert.Equal(t, gotRaw, tc.wantRaw)
		})
	}
}
