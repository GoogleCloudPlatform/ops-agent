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

package confgenerator

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/go-playground/validator/v10"
)

// requiredFeatureForType maps a component type to a feature that must
// be enabled (via EXPERIMENTAL_FEATURES) in order to use that component
// in an Ops Agent configuration.
// For example, the following would require the user to define the
// "otlp_receiver" feature flag inside EXPERIMENTAL_FEATURES in order to
// be able to use the "otlp" combined receiver:
//
//	"otlp": "otlp_receiver"
//
// N.B. There are no enforced feature flags today, so this map is
// intentionally left empty.
var requiredFeatureForType = map[string]string{}

var enabledExperimentalFeatures map[string]bool

func ParseExperimentalFeatures(features string) map[string]bool {
	out := map[string]bool{}
	for _, f := range strings.Split(features, ",") {
		out[strings.TrimSpace(f)] = true
	}
	return out
}

func init() {
	enabledExperimentalFeatures = ParseExperimentalFeatures(os.Getenv("EXPERIMENTAL_FEATURES"))
}

type experimentsKeyType struct{}

var experimentsKey = experimentsKeyType{}

func ContextWithExperiments(ctx context.Context, experiments map[string]bool) context.Context {
	return context.WithValue(ctx, experimentsKey, experiments)
}

func experimentsFromContext(ctx context.Context) map[string]bool {
	if features := ctx.Value(experimentsKey); features != nil {
		return features.(map[string]bool)
	}
	return enabledExperimentalFeatures
}

func registerExperimentalValidations(v *validator.Validate) {
	v.RegisterValidationCtx("experimental", func(ctx context.Context, fl validator.FieldLevel) bool {
		return fl.Field().IsZero() || experimentsFromContext(ctx)[fl.Param()]
	})
	v.RegisterStructValidationCtx(componentValidator, ConfigComponent{})
}

func componentValidator(ctx context.Context, sl validator.StructLevel) {
	comp, ok := sl.Current().Interface().(ConfigComponent)
	if !ok {
		return
	}
	feature, ok := requiredFeatureForType[comp.Type]
	if !ok || experimentsFromContext(ctx)[feature] {
		return
	}
	sl.ReportError(comp, "type", "Type", "experimental", comp.Type)
}

func experimentalValidationErrorString(ve validationError) string {
	return fmt.Sprintf("Experimental feature %q cannot be used with the current version of the Ops Agent", ve.Param())
}
