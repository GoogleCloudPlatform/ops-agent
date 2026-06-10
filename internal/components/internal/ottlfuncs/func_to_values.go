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
	"errors"
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
)

type ToValuesArguments[K any] struct {
	Target ottl.PSliceGetter[K]
}

func NewToValuesFactory[K any]() ottl.Factory[K] {
	return ottl.NewFactory("ToValues", &ToValuesArguments[K]{}, createToValuesFunction[K])
}

func createToValuesFunction[K any](_ ottl.FunctionContext, oArgs ottl.Arguments) (ottl.ExprFunc[K], error) {
	args, ok := oArgs.(*ToValuesArguments[K])
	if !ok {
		return nil, errors.New("ToValuesFactory args must be of type *ToValuesArguments[K]")
	}

	return toValues(args.Target)
}

func toValues[K any](target ottl.PSliceGetter[K]) (ottl.ExprFunc[K], error) {
	return func(ctx context.Context, tCtx K) (any, error) {
		res := pcommon.NewSlice()
		target, err := target.Get(ctx, tCtx)
		if err != nil {
			return nil, err
		}

		for i := 0; i < target.Len(); i++ {
			m := target.At(i)
			if m.Type() == pcommon.ValueTypeMap {
				for _, v := range m.Map().All() {
					v.CopyTo(res.AppendEmpty())
				}
			} else {
				return nil, fmt.Errorf("ToValues expects a slice of pcommon.Map, but got %s", m.Type())
			}
		}
		return res, nil
	}, nil
}
