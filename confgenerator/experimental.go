package confgenerator

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-playground/validator/v10"
)

var requiredFeatureForType = map[string]string{
	"prometheus": "prometheus_receiver",
}

func IsExperimentalFeatureEnabled(receiver string) bool {
	enabledList := strings.Split(os.Getenv("EXPERIMENTAL_FEATURES"), ",")
	for _, e := range enabledList {
		if e == receiver {
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
	sl.ReportError(nil, "", "", "experimental", fmt.Sprintf("Component of type %q cannot be used with the current version of the Ops Agent", comp.Type))
}

func experimentalValidationErrorString(ve validationError) string {
	return ve.Param()
}
