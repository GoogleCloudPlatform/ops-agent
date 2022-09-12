package confgenerator

import "fmt"

type Feature struct {
	Module, Kind, Type, Key, Value string
}

type featureExtractor struct {
	Module, Kind, Type string
	values             map[string]string
}

type ValueWrapper interface {
	AsString() string
	IsZero() bool
}

func newFeatureExtractor(module, kind, typ string) featureExtractor {
	return featureExtractor{
		Module: module,
		Kind:   kind,
		Type:   typ,
		values: make(map[string]string),
	}
}

func (f *featureExtractor) Add(key string, value ValueWrapper) {
	if value.IsZero() {
		return
	}
	f.values[key] = value.AsString()
}

func (f featureExtractor) ToFeatures() []Feature {
	var features []Feature
	for k, v := range f.values {
		features = append(features, Feature{
			Module: f.Module,
			Kind:   f.Kind,
			Type:   f.Type,
			Key:    k,
			Value:  v,
		})
	}

	return features
}

type box[T bool | int | string] struct {
	Value T
}

func (v box[T]) IsZero() bool {
	switch x := interface{}(v.Value).(type) {
	case int:
		return x == 0
	case string:
		return x == ""
	}
	return false
}

func (v box[T]) AsString() string {
	var s string
	switch x := interface{}(v.Value).(type) {
	case bool:
		s = fmt.Sprintf("%t", x)
	case int:
		s = fmt.Sprintf("%d", x)
	case string:
		s = x
	}
	return s
}

func newBox[T bool | int | string](t T) box[T] {
	return box[T]{
		Value: t,
	}
}

func ExtractFeatures(uc *UnifiedConfig) []Feature {
	allFeatures := getOverriddenDefaultPipelines(uc)

	//if uc.Metrics != nil {
	//	for _, r := range uc.Metrics.Receivers {
	//		allFeatures = append(allFeatures, r.Features()...)
	//	}
	//	for _, p := range uc.Metrics.Processors {
	//		allFeatures = append(allFeatures, p.Features()...)
	//	}
	//}
	//if uc.Logging != nil {
	//	for _, r := range uc.Logging.Receivers {
	//		allFeatures = append(allFeatures, r.Features()...)
	//	}
	//	for _, p := range uc.Logging.Processors {
	//		allFeatures = append(allFeatures, p.Features()...)
	//	}
	//}

	return allFeatures
}

func getOverriddenDefaultPipelines(uc *UnifiedConfig) []Feature {
	features := []Feature{
		{
			Module: "logging",
			Kind:   "service",
			Type:   "pipelines",
			Key:    "default_pipeline_overridden",
			Value:  "false",
		},
		{
			Module: "metrics",
			Kind:   "service",
			Type:   "pipelines",
			Key:    "default_pipeline_overridden",
			Value:  "false",
		},
	}
	if uc.Logging != nil && uc.Logging.Service != nil && uc.Logging.Service.Pipelines["default_pipeline"] != nil {
		features[0].Value = "true"
	}

	if uc.Metrics != nil && uc.Metrics.Service != nil && uc.Metrics.Service.Pipelines["default_pipeline"] != nil {
		features[1].Value = "true"
	}

	return features
}
