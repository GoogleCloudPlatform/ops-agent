package confgenerator

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

var ErrTrackingInlineStruct = errors.New("cannot have tracking on inline struct")
var ErrInvalidType = errors.New("object in path must be of type Component")

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
		if m.IsInline && m.HasTracking {
			return nil, ErrTrackingInlineStruct
		}
		if feature.Module == "" {
			feature.Module = m.PathName
		} else if feature.Kind == "" {
			feature.Kind = m.PathName
		} else if !m.IsInline {
			feature.Key = append(feature.Key, m.PathName)
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
		if !m.HasTracking {
			return nil, nil
		}
		keys := feature.Key

		// Iterate over all values from map. Keys of map are replaced with an index.
		// If the map is a Component only the index will append to the key.
		// Else both the name of the field and the index of the item is appended to
		// the key respectively.
		for i, key := range v.MapKeys() {
			obj := v.MapIndex(key)
			md := m
			ftr := feature
			index := fmt.Sprintf("%d", i)

			if ftr.Type == "" {
				comp, ok := obj.Interface().(Component)
				if !ok {
					return nil, ErrInvalidType
				}
				ftr.Type = comp.Type()
				ftr.Key = append(keys, index)
			} else {
				ftr.Key = append(keys, m.PathName)
				md.PathName = index
			}

			f, err := trackingFeatures(reflect.ValueOf(obj.Interface()), md, ftr)
			if err != nil {
				return nil, err
			}
			features = append(features, f...)
		}
	default:
		if skipField(v, m.HasTracking) {
			return nil, nil
		}
		feature.Key = append(feature.Key, m.PathName)
		if m.HasOverride {
			feature.Value = m.OverrideValue
		} else {
			feature.Value = fmt.Sprintf("%v", v.Interface())
		}
		features = append(features, feature)
	}

	return features, nil
}

func skipField(value reflect.Value, hasTracking bool) bool {
	if hasTracking {
		return false
	}
	switch value.Type().Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int32:
		return false
	default:
		return true
	}
}

type metadata struct {
	IsInline      bool
	HasTracking   bool
	HasOverride   bool
	PathName      string
	OverrideValue string
}

func getMetadata(field reflect.StructField) metadata {
	trackingTag, hasTracking := field.Tag.Lookup("tracking")
	hasOverride := false
	if trackingTag != "" {
		hasOverride = true
	}

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
		IsInline:      hasInline,
		OverrideValue: trackingTag,
		PathName:      yamlTags[0],
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
