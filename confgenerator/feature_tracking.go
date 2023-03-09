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
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

var (
	ErrTrackingInlineStruct   = errors.New("cannot have tracking on inline struct")
	ErrTrackingOverrideStruct = errors.New("struct that has tracking tag must not be empty")
)

type TrackingOverrideMapError struct {
	FieldName string
}

func (e *TrackingOverrideMapError) Error() string {
	return fmt.Sprintf("map type for a field is not supported: %s", e.FieldName)
}

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

type CustomFeature struct {
	// Key: set of keys that will be joined together for feature tracking metrics
	Key []string
	// Value defined from fields of UnifiedConfig.
	Value string
}

// CustomFeatures is the interface that components must implement to be able to
// track features not captured by the `tracking` struct tag.
type CustomFeatures interface {
	// ExtractFeatures returns a list of features that will be tracked for this component.
	ExtractFeatures() ([]CustomFeature, error)

	// ListAllFeatures returns a list of all features that could be tracked for this component.
	// This lists all the possible features that could be tracked for this component, but some of these
	// features may not be tracked when not used by the component.
	ListAllFeatures() []string
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

	if uc.HasCombined() {
		tempTrackedFeatures, err = trackedMappedComponents("combined", "receivers", uc.Combined.Receivers)
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

func reflectSortedKeys(m reflect.Value) []string {
	keys := make([]string, 0)
	for _, k := range m.MapKeys() {
		keys = append(keys, k.String())
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

// TODO: add time.Duration to auto tracking
func trackingFeatures(c reflect.Value, m metadata, feature Feature) ([]Feature, error) {
	if customFeatures, ok := c.Interface().(CustomFeatures); ok {
		cfs, err := customFeatures.ExtractFeatures()
		if err != nil {
			return nil, err
		}
		var features []Feature
		for _, cf := range cfs {
			features = append(features, Feature{
				Module: feature.Module,
				Kind:   feature.Kind,
				Type:   feature.Type,
				Key:    append(feature.Key, cf.Key...),
				Value:  cf.Value,
			})
		}
		return features, nil
	}

	if m.isExcluded {
		return nil, nil
	}
	t := c.Type()

	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if c.IsZero() {
		return nil, nil
	}

	v := reflect.Indirect(c)
	if v.Kind() == reflect.Invalid {
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

			f := Feature{
				Module: feature.Module,
				Kind:   feature.Kind,
				Type:   feature.Type,
				Key:    append([]string{}, feature.Key...),
				Value:  feature.Value,
			}

			tf, err := trackingFeatures(v.Field(i), getMetadata(field), f)
			if err != nil {
				return nil, err
			}
			features = append(features, tf...)
		}
	case kind == reflect.Map:

		// Create map length metric
		features = append(features, Feature{
			Module: feature.Module,
			Kind:   feature.Kind,
			Type:   feature.Type,
			Key:    append(feature.Key, m.yamlTag, "__length"),
			Value:  fmt.Sprintf("%d", v.Len()),
		})

		for i, key := range reflectSortedKeys(v) {
			f := Feature{
				Module: feature.Module,
				Kind:   feature.Kind,
				Type:   feature.Type,
				Key:    append(feature.Key, m.yamlTag),
			}
			v := v.MapIndex(reflect.ValueOf(key))
			t := v.Type()
			fs := make([]Feature, 0)

			k := fmt.Sprintf("[%d]", i)
			if m.keepKeys {
				features = append(features, Feature{
					Module: feature.Module,
					Kind:   feature.Kind,
					Type:   feature.Type,
					Key:    append(feature.Key, m.yamlTag, k, "__key"),
					Value:  key,
				})
			}

			m2 := m.deepCopy()

			var err error
			if t.Kind() == reflect.Struct {
				f.Key = append(f.Key, k)
				m2.yamlTag = ""
				fs, err = trackingFeatures(v, m2, f)
			} else {
				m2.yamlTag = k
				fs, err = trackingFeatures(v, m2, f)
			}

			if err != nil {
				return nil, err
			}
			features = append(features, fs...)
		}

	case kind == reflect.Slice || kind == reflect.Array:

		// Create array length metric
		features = append(features, Feature{
			Module: feature.Module,
			Kind:   feature.Kind,
			Type:   feature.Type,
			Key:    append(feature.Key, m.yamlTag, "__length"),
			Value:  fmt.Sprintf("%d", v.Len()),
		})

		for i := 0; i < v.Len(); i++ {
			f := Feature{
				Module: feature.Module,
				Kind:   feature.Kind,
				Type:   feature.Type,
				Key:    append(feature.Key, m.yamlTag),
			}

			v := v.Index(i)
			t := v.Type()
			fs := make([]Feature, 0)
			m2 := m.deepCopy()

			var err error
			if t.Kind() == reflect.Struct {
				f.Key = append(f.Key, fmt.Sprintf("[%d]", i))
				m2.yamlTag = ""
				fs, err = trackingFeatures(v, m2, f)
			} else {
				m2.yamlTag = fmt.Sprintf("[%d]", i)
				fs, err = trackingFeatures(v, m2, f)
			}

			if err != nil {
				return nil, err
			}
			features = append(features, fs...)
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
	keepKeys       bool
	yamlTag        string
	overrideValue  string
	componentIndex int
}

func (m metadata) deepCopy() metadata {
	return metadata{
		isExcluded:     m.isExcluded,
		isInline:       m.isInline,
		hasTracking:    m.hasTracking,
		hasOverride:    m.hasOverride,
		keepKeys:       m.keepKeys,
		yamlTag:        m.yamlTag,
		overrideValue:  m.overrideValue,
		componentIndex: m.componentIndex,
	}
}

func getMetadata(field reflect.StructField) metadata {
	trackingTag, hasTracking := field.Tag.Lookup("tracking")
	hasOverride := false
	hasKeepKeys := false

	trackingTags := strings.Split(trackingTag, ",")

	if trackingTags[0] != "" {
		hasOverride = true
	}
	if len(trackingTags) > 1 && trackingTags[1] == "keys" {
		hasKeepKeys = true
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
		keepKeys:      hasKeepKeys,
		isExcluded:    isExcluded,
		isInline:      hasInline,
		overrideValue: trackingTags[0],
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
