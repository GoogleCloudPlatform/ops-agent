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
	"fmt"
	"os"
	"strings"

	"github.com/go-playground/validator/v10"
)

var requiredFeatureForType = map[string]string{
	"otlp": "otlp_receiver",
}

func IsExperimentalFeatureEnabled(feature string) bool {
	enabledList := strings.Split(os.Getenv("EXPERIMENTAL_FEATURES"), ",")
	for _, e := range enabledList {
		if e == feature {
			return true
		}
	}
	return false
}

func registerExperimentalValidations(v *validator.Validate) {
	v.RegisterStructValidation(componentValidator, ConfigComponent{})
}

func componentValidator(sl validator.StructLevel) {
	comp, ok := sl.Current().Interface().(ConfigComponent)
	if !ok {
		return
	}
	feature, ok := requiredFeatureForType[comp.Type]
	if !ok || IsExperimentalFeatureEnabled(feature) {
		return
	}
	sl.ReportError(comp, "type", "Type", "experimental", comp.Type)
}

func experimentalValidationErrorString(ve validationError) string {
	return fmt.Sprintf("Component of type %q cannot be used with the current version of the Ops Agent", ve.Param())
}
