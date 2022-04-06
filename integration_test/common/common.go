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
	Type string `yaml:"type"`
	// The value type, for example INT64.
	ValueType string `yaml:"value_type" validate:"oneof=BOOL INT64 DOUBLE STRING DISTRIBUTION"`
	// The kind, for example GAUGE.
	Kind string `yaml:"kind" validate:"oneof=GAUGE DELTA CUMULATIVE"`
	// The monitored resource, for example gce_instance.
	// Currently we only test with gce_instance.
	MonitoredResource string `yaml:"monitored_resource" validate:"oneof=gce_instance"`
	// Mapping of expected label keys to value patterns.
	// Patterns are RE2 regular expressions.
	Labels map[string]string `yaml:"labels"`
	// If Optional is true, the test for this metric will be skipped.
	Optional bool `yaml:"optional,omitempty" validate:"necsfield=Representative"`
	// Exactly one metric in each expected_metrics.yaml must
	// have Representative set to true. This metric can be used
	// to test that the integration is enabled.
	Representative bool `yaml:"representative,omitempty" validate:"necsfield=Optional"`
}

// ValidateMetrics checks that all enum fields have valid values and that
// there is exactly one representative metric in the slice.
func ValidateMetrics(metrics []ExpectedMetric) error {
	// Field validation
	v := validator.Validate{}
	err := v.Struct(metrics)
	// Representative validation
	representativeCount := 0
	for _, metric := range metrics {
		if metric.Representative {
			representativeCount += 1
			if metric.Optional {
				err = multierr.Append(err, fmt.Errorf("%s: metric cannot be both representative and optional", metric.Type))
			}
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
