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

	"github.com/go-playground/validator/v10"
	"go.uber.org/multierr"
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
	Optional bool `yaml:"optional,omitempty" validate:"excluded_with=Representative"`
	// Exactly one metric in each expected_metrics.yaml must
	// have Representative set to true. This metric can be used
	// to test that the integration is enabled.
	Representative bool `yaml:"representative,omitempty" validate:"excluded_with=Optional"`
}

type LogFields struct {
	Name        string `yaml:"name" validate:"required"`
	ValueRegex  string `yaml:"value_regex,omitempty"`
	Type        string `yaml:"type" validate:"required"`
	Description string `yaml:"description" validate:"required"`
}

type ExpectedLog struct {
	LogName string      `yaml:"log_name" validate:"required"`
	Fields  []LogFields `yaml:"fields" validate:"required"`
}

type MinimumSupportedAgentVersion struct {
	Logging string `yaml:"logging,omitempty"`
	Metrics string `yaml:"metrics,omitempty"`
}

type IntegrationMetadata struct {
	PublicUrl                    string                       `yaml:"public_url,omitempty"`
	ShortName                    string                       `yaml:"short_name" validate:"required"`
	LongName                     string                       `yaml:"long_name" validate:"required"`
	ConfigureIntegration         string                       `yaml:"configure_integration,omitempty"`
	ExpectedLogs                 []ExpectedLog                `yaml:"expected_logs,omitempty"`
	ExpectedMetrics              []ExpectedMetric             `yaml:"expected_metrics,omitempty"`
	MinimumSupportedAgentVersion MinimumSupportedAgentVersion `yaml:"minimum_supported_agent_version,omitempty"`
	SupportedAppVersion          []string                     `yaml:"supported_app_version" validate:"required"`
}

var validate *validator.Validate

// ValidateMetrics checks that all enum fields have valid values and that
// there is exactly one representative metric in the slice.
func ValidateMetrics(metrics []ExpectedMetric) error {
	var err error

	// Field validation, one-by-one for better error messages
	for _, m := range metrics {
		vErr := validate.Struct(m)
		if vErr != nil {
			err = multierr.Append(err, fmt.Errorf("%s: %v", m.Type, vErr))
		}
	}

	// Representative validation
	representativeCount := 0
	for _, m := range metrics {
		if m.Representative {
			representativeCount += 1
		}
	}
	if representativeCount != 1 {
		err = multierr.Append(err, fmt.Errorf("there must be exactly one metric with representative: true, but %d were found", representativeCount))
	}
	return err
}

func SliceContains(slice []string, toFind string) bool {
	for _, entry := range slice {
		if entry == toFind {
			return true
		}
	}
	return false
}

func init() {
	validate = validator.New()
}
