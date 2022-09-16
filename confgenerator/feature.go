package confgenerator

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

var ErrTrackingInlineStruct = errors.New("cannot have tracking on inline struct")

type Feature struct {
	Module, Kind, Type, Key, Value string
}

func ExtractFeatures(uc *UnifiedConfig) ([]Feature, error) {
	allFeatures := getOverriddenDefaultPipelines(uc)

	if uc.Metrics != nil {
		for _, r := range uc.Metrics.Receivers {
			features, err := trackingFeatures(r, MetricsReceiverTypes.Subagent, MetricsReceiverTypes.Kind, r.Type())
			if err != nil {
				return nil, err
			}
			allFeatures = append(allFeatures, features...)
		}
		for _, p := range uc.Metrics.Processors {
			features, err := trackingFeatures(p, MetricsProcessorTypes.Subagent, MetricsProcessorTypes.Kind, p.Type())
			if err != nil {
				return nil, err
			}
			allFeatures = append(allFeatures, features...)
		}
	}
	if uc.Logging != nil {
		for _, r := range uc.Logging.Receivers {
			features, err := trackingFeatures(r, LoggingReceiverTypes.Subagent, LoggingReceiverTypes.Kind, r.Type())
			if err != nil {
				return nil, err
			}
			allFeatures = append(allFeatures, features...)
		}
		for _, p := range uc.Logging.Processors {
			features, err := trackingFeatures(p, LoggingProcessorTypes.Subagent, LoggingProcessorTypes.Kind, p.Type())
			if err != nil {
				return nil, err
			}
			allFeatures = append(allFeatures, features...)
		}
	}

	return allFeatures, nil
}

func trackingFeatures(component any, module, kind, typ string) ([]Feature, error) {
	return trackingFeaturesHelper(component, module, kind, typ, "")
}

func trackingFeaturesHelper(component any, module, kind, typ, prefix string) ([]Feature, error) {
	if component == nil {
		return []Feature{}, nil
	}
	c := reflect.ValueOf(component)
	t := c.Type()

	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	var features []Feature
	// Iterate over all available fields and read the tag value
	for i := 0; i < t.NumField(); i++ {
		// Get the field, returns https://golang.org/pkg/reflect/#StructField
		field := t.Field(i)

		// Grab actual value instead of pointer
		v := reflect.Indirect(reflect.Indirect(c).FieldByName(field.Name))
		if v.IsZero() {
			continue
		}

		// Get the field tag value
		// field.Tag.Lookup()
		trackingTag := field.Tag.Get("tracking")
		tags := strings.Split(trackingTag, ",")
		trackingName := tags[0]

		// Check to see if value needs to be overridden
		var overridden string
		if len(tags) > 1 {
			overridden = tags[1]
		}
		yamlTag := field.Tag.Get("yaml")
		isInline := strings.Contains(yamlTag, ",inline")

		// Append prefix with dot notation for child features key
		key := fmt.Sprintf("%s.%s", prefix, trackingName)
		if prefix == "" || isInline {
			key = trackingName
		}

		if v.Type().Kind() == reflect.Struct {
			if isInline && trackingTag != "" {
				return nil, ErrTrackingInlineStruct
			}
			tf, err := trackingFeaturesHelper(v.Interface(), module, kind, typ, key)
			if err != nil {
				return nil, err
			}
			features = append(features, tf...)
		} else {
			if trackingTag == "" {
				continue
			}
			featureTrackingString, err := asString(v.Interface())
			if err != nil {
				return nil, err
			}
			if overridden != "" {
				featureTrackingString = overridden
			}
			features = append(features, Feature{
				Module: module,
				Kind:   kind,
				Type:   typ,
				Key:    key,
				Value:  featureTrackingString,
			})
		}
	}

	return features, nil
}

func asString(v any) (string, error) {
	var s string
	switch x := v.(type) {
	case bool:
		s = fmt.Sprintf("%t", x)
	case int:
		s = fmt.Sprintf("%d", x)
	case string:
		s = x
	default:
		// TODO Add warning
		return "", fmt.Errorf("cannot create string for type: %s", x)
	}
	return s, nil
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
