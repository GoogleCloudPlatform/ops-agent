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

	rubex "github.com/GoogleCloudPlatform/opentelemetry-operations-collector/third_party/go-oniguruma"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
)

type IsMatchRubyRegexArguments[K any] struct {
	Target  ottl.StringLikeGetter[K]
	Pattern string
}

func NewIsMatchRubyRegexFactory[K any]() ottl.Factory[K] {
	return ottl.NewFactory("IsMatchRubyRegex", &IsMatchRubyRegexArguments[K]{}, createIsMatchRubyRegexFunction[K])
}

func createIsMatchRubyRegexFunction[K any](_ ottl.FunctionContext, oArgs ottl.Arguments) (ottl.ExprFunc[K], error) {
	args, ok := oArgs.(*IsMatchRubyRegexArguments[K])

	if !ok {
		return nil, errors.New("IsMatchRubyRegexFactory args must be of type *IsMatchRubyRegexArguments[K]")
	}

	return isMatchRubyRegex(args.Target, args.Pattern)
}

func isMatchRubyRegex[K any](target ottl.StringLikeGetter[K], pattern string) (ottl.ExprFunc[K], error) {
	compiledPattern, err := rubex.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("the pattern supplied to IsMatchRubyRegex is not a valid regexp pattern: %w", err)
	}
	return func(ctx context.Context, tCtx K) (any, error) {
		val, err := target.Get(ctx, tCtx)
		if err != nil {
			return nil, err
		}
		if val == nil {
			return false, nil
		}
		return compiledPattern.MatchString(*val), nil
	}, nil
}
