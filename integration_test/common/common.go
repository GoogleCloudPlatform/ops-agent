// Copyright 2022 Google LLC
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

//go:build integration_test

package common

import (
	"fmt"
	"reflect"

	"github.com/go-playground/validator/v10"
)

// ExpectedMetric encodes a series of assertions about what data we expect
// to see in the metrics backend.
type ExpectedMetric struct {
	// The metric type, for example workload.googleapis.com/apache.current_connections.
	Type string `yaml:"type" validate:"required"`
	// The value type, for example INT64.
	ValueType string `yaml:"value_type" validate:"required,oneof=BOOL INT64 DOUBLE STRING DISTRIBUTION"`
	// The kind, for example GAUGE.
	Kind string `yaml:"kind" validate:"required,oneof=GAUGE DELTA CUMULATIVE"`
	// The monitored resource, for example gce_instance.
	// Currently we only test with gce_instance.
	MonitoredResource string `yaml:"monitored_resource" validate:"required,oneof=gce_instance"`
	// Mapping of expected label keys to value patterns.
	// Patterns are RE2 regular expressions.
	Labels map[string]string `yaml:"labels" validate:"required"`
	// If Optional is true, the test for this metric will be skipped.
	Optional bool `yaml:"optional" validate:"excluded_with=Representative"`
	// Exactly one metric in each expected_metrics.yaml must
	// have Representative set to true. This metric can be used
	// to test that the integration is enabled.
	Representative bool `yaml:"representative" validate:"excluded_with=Optional"`
}

type LogFields struct {
	Name        string `yaml:"name" validate:"required"`
	ValueRegex  string `yaml:"value_regex"`
	Type        string `yaml:"type" validate:"required"`
	Description string `yaml:"description" validate:"excludesall=‘’“”"`
}

type ExpectedLog struct {
	LogName string       `yaml:"log_name" validate:"required"`
	Fields  []*LogFields `yaml:"fields" validate:"required,dive"`
}

type MinimumSupportedAgentVersion struct {
	Logging string `yaml:"logging"`
	Metrics string `yaml:"metrics"`
}

type ConfigurationFields struct {
	Name        string `yaml:"name" validate:"required"`
	Default     string `yaml:"default"`
	Description string `yaml:"description" validate:"required,excludesall=‘’“”"`
}

type InputConfiguration struct {
	Type   string                 `yaml:"type" validate:"required"`
	Fields []*ConfigurationFields `yaml:"fields" validate:"required,dive"`
}

type ConfigurationOptions struct {
	LogsConfiguration    []*InputConfiguration `yaml:"logs" validate:"required_without=MetricsConfiguration,dive"`
	MetricsConfiguration []*InputConfiguration `yaml:"metrics" validate:"required_without=LogsConfiguration,dive"`
}

type IntegrationMetadata struct {
	PublicUrl                    string                       `yaml:"public_url"`
	AppUrl                       string                       `yaml:"app_url" validate:"required,url"`
	ShortName                    string                       `yaml:"short_name" validate:"required,excludesall=‘’“”"`
	LongName                     string                       `yaml:"long_name" validate:"required,excludesall=‘’“”"`
	LogoPath                     string                       `yaml:"logo_path"`
	Description                  string                       `yaml:"description" validate:"required,excludesall=‘’“”"`
	ConfigurationOptions         *ConfigurationOptions        `yaml:"configuration_options" validate:"required"`
	ConfigureIntegration         string                       `yaml:"configure_integration"`
	ExpectedLogs                 []*ExpectedLog               `yaml:"expected_logs" validate:"dive"`
	ExpectedMetrics              []*ExpectedMetric            `yaml:"expected_metrics" validate:"onetrue=Representative,unique=Type,dive"`
	MinimumSupportedAgentVersion MinimumSupportedAgentVersion `yaml:"minimum_supported_agent_version"`
	SupportedAppVersion          []string                     `yaml:"supported_app_version" validate:"required,unique,min=1"`
	SupportedOperatingSystems    string                       `yaml:"supported_operating_systems" validate:"required,oneof=linux windows linux_and_windows"`
	RestartAfterInstall          bool                         `yaml:"restart_after_install"`
	Troubleshoot                 string                       `yaml:"troubleshoot" validate:"excludesall=‘’“”"`
}

func SliceContains(slice []string, toFind string) bool {
	for _, entry := range slice {
		if entry == toFind {
			return true
		}
	}
	return false
}

func NewIntegrationMetadataValidator() *validator.Validate {
	v := validator.New()
	_ = v.RegisterValidation("onetrue", func(fl validator.FieldLevel) bool {
		field := fl.Field()
		param := fl.Param()

		if param == "" {
			panic("onetrue must contain an argument")
		}

		switch field.Kind() {

		case reflect.Slice, reflect.Array:
			elem := field.Type().Elem()

			// Ignore the case where this field is not actually specified or is left empty.
			if field.Len() == 0 {
				return true
			}

			if elem.Kind() == reflect.Ptr {
				elem = elem.Elem()
			}

			sf, ok := elem.FieldByName(param)
			if !ok {
				panic(fmt.Sprintf("Invalid field name %s", param))
			}
			if sfTyp := sf.Type; sfTyp.Kind() != reflect.Bool {
				panic(fmt.Sprintf("Field %s is %s, not bool", param, sfTyp))
			}

			count := 0
			for i := 0; i < field.Len(); i++ {
				if reflect.Indirect(field.Index(i)).FieldByName(param).Bool() {
					count++
				}
			}

			return count == 1

		default:
			panic(fmt.Sprintf("Invalid field type %T", field.Interface()))
		}
	})
	return v
}
