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
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"

	rubex "github.com/GoogleCloudPlatform/opentelemetry-operations-collector/third_party/go-oniguruma"
)

type ExtractPatternsRubyRegexArguments[K any] struct {
	Target          ottl.StringGetter[K]
	Pattern         string
	OmitEmptyValues ottl.Optional[bool]
}

func NewExtractPatternsRubyRegexFactory[K any]() ottl.Factory[K] {
	return ottl.NewFactory("ExtractPatternsRubyRegex", &ExtractPatternsRubyRegexArguments[K]{}, createExtractPatternsRubyRegexFunction[K])
}

func createExtractPatternsRubyRegexFunction[K any](_ ottl.FunctionContext, oArgs ottl.Arguments) (ottl.ExprFunc[K], error) {
	args, ok := oArgs.(*ExtractPatternsRubyRegexArguments[K])

	if !ok {
		return nil, fmt.Errorf("ExtractPatternsRubyRegexFactory args must be of type *ExtractPatternsRubyRegexArguments[K]")
	}

	omitEmptyValues := false
	if !args.OmitEmptyValues.IsEmpty() {
		omitEmptyValues = args.OmitEmptyValues.Get()
	}

	return extractPatternsRubyRegex(args.Target, args.Pattern, omitEmptyValues)
}

func extractPatternsRubyRegex[K any](target ottl.StringGetter[K], pattern string, omitEmtpyValues bool) (ottl.ExprFunc[K], error) {
	r, err := rubex.NewRegexp(pattern, rubex.ONIG_OPTION_DEFAULT)
	if err != nil {
		return nil, fmt.Errorf("the pattern supplied to ExtractPatternsRubyRegex is not a valid pattern: %w", err)
	}

	namedCaptureGroups := 0
	subExpNames := r.SubexpNames()
	for _, groupName := range subExpNames {
		if groupName != "" {
			namedCaptureGroups++
		}
	}

	if namedCaptureGroups == 0 {
		return nil, fmt.Errorf("at least 1 named capture group must be supplied in the given regex")
	}

	return func(ctx context.Context, tCtx K) (any, error) {
		val, err := target.Get(ctx, tCtx)
		if err != nil {
			return nil, err
		}

		matches := r.FindStringSubmatch(val)
		if matches == nil {
			return pcommon.NewMap(), nil
		}

		result := pcommon.NewMap()
		for i, subexp := range r.SubexpNames() {
			if subexp != "" {
				if omitEmtpyValues && matches[i+1] == "" {
					continue
				}
				result.PutStr(subexp, matches[i+1])
			}
		}
		return result, err
	}, nil
}
