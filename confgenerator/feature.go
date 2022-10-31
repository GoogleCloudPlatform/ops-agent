package confgenerator

import (
	"errors"
	"fmt"
	"reflect"
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
	f, err := trackingFeatures(reflect.ValueOf(uc), metadata{}, Feature{})
	if err != nil {
		return nil, err
	}
	allFeatures = append(allFeatures, f...)
	return allFeatures, nil
}

func trackingFeatures(c reflect.Value, m metadata, feature Feature) ([]Feature, error) {
	if m.IsExcluded {
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
		// If struct is inline there is no associated name for key generation
		// By default inline structs of a tracked field are also tracked
		if m.IsInline && m.HasTracking {
			return nil, ErrTrackingInlineStruct
		}

		// If struct has tracking it must have a value
		if m.HasTracking && !m.HasOverride {
			return nil, ErrTrackingOverrideStruct
		}

		// Path for execution is coupled with the UnifiedConfig structure, feature
		// gets populated as we iterate through each level of the struct
		if feature.Module == "" {
			feature.Module = m.FieldTag
		} else if feature.Kind == "" {
			feature.Kind = m.FieldTag
		} else if !m.IsInline {
			feature.Key = append(feature.Key, m.FieldTag)
		}

		// For structs that in a Component
		if m.HasTracking && feature.Module != "" && feature.Kind != "" && feature.Type != "" && feature.Key != nil {
			ftr := feature
			ftr.Value = m.OverrideValue
			features = append(features, ftr)
		}

		// Iterate over all available fields and read the tag value
		for i := 0; i < t.NumField(); i++ {
			// Get the field, returns https://golang.org/pkg/reflect/#StructField
			field := t.Field(i)
			if field.Name == "Type" {
				// Capture special metrics for enabled receiver or processor
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
		if m.HasTracking && !m.HasOverride {
			return nil, ErrTrackingOverrideMap
		}

		keys := feature.Key

		if m.HasTracking {
			ftr := feature
			ftr.Key = append(ftr.Key, m.FieldTag)
			ftr.Value = m.OverrideValue
			features = append(features, ftr)
		}

		// Maps as a field are not supported yet
		if feature.Type != "" {
			return nil, ErrMapAsField
		}

		// Iterate over all values from map. Keys of map are replaced with an index.
		// If the map is a Receiver or Processor its values will all be Components.
		for i, key := range v.MapKeys() {
			obj := v.MapIndex(key)
			md := m
			ftr := feature

			comp, ok := obj.Interface().(Component)
			if !ok {
				return nil, ErrInvalidType
			}
			ftr.Type = comp.Type()
			ftr.Key = append(keys, fmt.Sprintf("[%d]", i))

			f, err := trackingFeatures(reflect.ValueOf(obj.Interface()), md, ftr)
			if err != nil {
				return nil, err
			}
			features = append(features, f...)
		}
	default:
		if skipField(v, m) {
			return nil, nil
		}
		feature.Key = append(feature.Key, m.FieldTag)
		if m.HasOverride {
			feature.Value = m.OverrideValue
		} else {
			feature.Value = fmt.Sprintf("%v", v.Interface())
		}
		features = append(features, feature)
	}

	return features, nil
}

func skipField(value reflect.Value, m metadata) bool {
	if m.HasTracking {
		return false
	}
	if m.IsExcluded {
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
	IsExcluded    bool
	IsInline      bool
	HasTracking   bool
	HasOverride   bool
	FieldTag      string
	OverrideValue string
}

func getMetadata(field reflect.StructField) metadata {
	trackingTag, hasTracking := field.Tag.Lookup("tracking")
	hasOverride := false
	if trackingTag != "" {
		hasOverride = true
	}
	isExcluded := trackingTag == "-"

	yamlTag, _ := field.Tag.Lookup("yaml")
	hasInline := false
	yamlTags := strings.Split(yamlTag, ",")
	for _, tag := range yamlTags {
		if tag == "inline" {
			hasInline = true
		}
	}

	return metadata{
		HasTracking:   hasTracking,
		HasOverride:   hasOverride,
		IsExcluded:    isExcluded,
		IsInline:      hasInline,
		OverrideValue: trackingTag,
		// The first tag is the field identifier
		// See this for more details: https://pkg.go.dev/gopkg.in/yaml.v2#Unmarshal
		FieldTag: yamlTags[0],
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
