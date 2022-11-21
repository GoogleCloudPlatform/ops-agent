package confgenerator

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
)

// Errors returned by ExtractFeatures can be tested against these errors using
// errors.Is
var (
	ErrTrackingInlineStruct   = errors.New("cannot have tracking on inline struct")
	ErrTrackingOverrideStruct = errors.New("struct that has tracking tag must not be empty")
	ErrMapAsField             = errors.New("map type for a field is not supported")
	ErrTrackingOverrideMap    = errors.New("map that has tracking tag must not be empty")
	ErrInvalidType            = errors.New("object in path must be of type Component")
)

type Feature struct {
	// Module defines the sub-agent: metrics or logging
	Module string
	// Kind defines the kind: receivers or processors
	Kind string
	// Type from Component.Type()
	Type string
	// Key: set of keys that will be joined together for feature tracking metrics
	Key []string
	// Value defined from fields of UnifiedConfig.
	Value string
}

// ExtractFeatures fields that containing a tracking tag will be tracked.
// Automatic collection of bool or int fields. Any value that exists on tracking
// tag will be used instead of value from UnifiedConfig.
func ExtractFeatures(uc *UnifiedConfig) ([]Feature, error) {
	allFeatures := getOverriddenDefaultPipelines(uc)

	var err error
	var tempTrackedFeatures []Feature
	if uc.HasMetrics() {
		tempTrackedFeatures, err = trackedMappedComponents("metrics", "receivers", uc.Metrics.Receivers)
		if err != nil {
			return nil, err
		}
		allFeatures = append(allFeatures, tempTrackedFeatures...)

		tempTrackedFeatures, err = trackedMappedComponents("metrics", "processors", uc.Metrics.Processors)
		if err != nil {
			return nil, err
		}
		allFeatures = append(allFeatures, tempTrackedFeatures...)
	}

	if uc.HasLogging() {
		tempTrackedFeatures, err = trackedMappedComponents("logging", "receivers", uc.Logging.Receivers)
		if err != nil {
			return nil, err
		}
		allFeatures = append(allFeatures, tempTrackedFeatures...)

		tempTrackedFeatures, err = trackedMappedComponents("logging", "processors", uc.Logging.Processors)
		if err != nil {
			return nil, err
		}
		allFeatures = append(allFeatures, tempTrackedFeatures...)
	}
	return allFeatures, nil
}

func getSortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func trackedMappedComponents[C Component](module string, kind string, m map[string]C) ([]Feature, error) {
	if m == nil {
		return nil, nil
	}
	var features []Feature
	for i, k := range getSortedKeys(m) {
		c := m[k]
		feature := Feature{
			Module: module,
			Kind:   kind,
			Type:   c.Type(),
			Key:    []string{fmt.Sprintf("[%d]", i)},
		}
		trackedFeatures, err := trackingFeatures(reflect.ValueOf(c), metadata{}, feature)
		if err != nil {
			return nil, err
		}
		features = append(features, trackedFeatures...)
	}

	return features, nil
}

func trackingFeatures(c reflect.Value, m metadata, feature Feature) ([]Feature, error) {
	if m.isExcluded {
		return nil, nil
	}
	t := c.Type()

	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	v := reflect.Indirect(c)
	if v.Kind() == reflect.Invalid || v.IsZero() {
		return nil, nil
	}

	var features []Feature

	switch kind := t.Kind(); {
	case kind == reflect.Struct:
		// If struct has tracking it must have a value
		if m.hasTracking && !m.hasOverride {
			return nil, ErrTrackingOverrideStruct
		}

		if m.yamlTag != "" {
			feature.Key = append(feature.Key, m.yamlTag)
		}

		if m.hasTracking {
			// If struct is inline there is no associated name for key generation
			// By default inline structs of a tracked field are also tracked
			if m.isInline {
				return nil, ErrTrackingInlineStruct
			} else {
				// For structs that are in a Component. An extra metric is added with
				// the value being the override value from the yaml tag
				ftr := feature
				ftr.Value = m.overrideValue
				features = append(features, ftr)
			}
		}

		// Iterate over all available fields and read the tag value
		for i := 0; i < t.NumField(); i++ {
			// Get the field, returns https://golang.org/pkg/reflect/#StructField
			field := t.Field(i)
			// Type field name is part of the ConfigComponent definition.
			// All user visible component inlines that component, this field can help
			// us assert that a certain component is enabled.
			// Capture special metrics for enabled receiver or processor
			if field.Name == "Type" {
				f := feature
				f.Key = append(f.Key, "enabled")
				f.Value = "true"
				features = append(features, f)
				continue
			}

			tf, err := trackingFeatures(v.Field(i), getMetadata(field), feature)
			if err != nil {
				return nil, err
			}
			features = append(features, tf...)
		}
	case kind == reflect.Map:

		// TODO(b/258211839): Add support for tracking maps using feature tracking
		if feature.Type != "" {
			return nil, ErrMapAsField
		}

	default:
		if skipField(v, m) {
			return nil, nil
		}
		feature.Key = append(feature.Key, m.yamlTag)
		if m.hasOverride {
			feature.Value = m.overrideValue
		} else {
			feature.Value = fmt.Sprintf("%v", v.Interface())
		}
		features = append(features, feature)
	}

	return features, nil
}

func skipField(value reflect.Value, m metadata) bool {
	if m.hasTracking {
		return false
	}
	if m.isExcluded {
		return true
	}
	switch value.Type().Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int32:
		return false
	default:
		return true
	}
}

type metadata struct {
	isExcluded     bool
	isInline       bool
	hasTracking    bool
	hasOverride    bool
	yamlTag        string
	overrideValue  string
	componentIndex int
}

func getMetadata(field reflect.StructField) metadata {
	trackingTag, hasTracking := field.Tag.Lookup("tracking")
	hasOverride := false
	if trackingTag != "" {
		hasOverride = true
	}
	isExcluded := trackingTag == "-"

	yamlTag, ok := field.Tag.Lookup("yaml")
	if !ok {
		panic("field must have a yaml tag")
	}
	hasInline := false
	yamlTags := strings.Split(yamlTag, ",")
	for _, tag := range yamlTags {
		if tag == "inline" {
			hasInline = true
		}
	}

	return metadata{
		hasTracking:   hasTracking,
		hasOverride:   hasOverride,
		isExcluded:    isExcluded,
		isInline:      hasInline,
		overrideValue: trackingTag,
		// The first tag is the field identifier
		// See this for more details: https://pkg.go.dev/gopkg.in/yaml.v2#Unmarshal
		yamlTag: yamlTags[0],
	}
}

func getOverriddenDefaultPipelines(uc *UnifiedConfig) []Feature {
	features := []Feature{
		{
			Module: "logging",
			Kind:   "service",
			Type:   "pipelines",
			Key:    []string{"default_pipeline_overridden"},
			Value:  "false",
		},
		{
			Module: "metrics",
			Kind:   "service",
			Type:   "pipelines",
			Key:    []string{"default_pipeline_overridden"},
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

func IsExperimentalReceiverEnabled(receiver string) bool {
	enabledList := strings.Split(os.Getenv("EXPERIMENTAL_RECEIVERS"), ",")
	for _, e := range enabledList {
		if e == receiver {
			return true
		}
	}
	return false
}
