package common

import (
	"fmt"
	"strings"

	"go.uber.org/multierr"
)

// ExpectedMetric encodes a series of assertions about what data we expect
// to see in the metrics backend.
type ExpectedMetric struct {
	// The metric type, for example workload.googleapis.com/apache.current_connections.
	Type string `yaml:"type"`
	// The value type, for example INT64.
	ValueType string `yaml:"value_type"`
	// The kind, for example GAUGE.
	Kind string `yaml:"kind"`
	// The monitored resource, for example gce_instance.
	// Currently we only test with gce_instance, so we expect
	// all expectedMetricsEntries to have gce_instance.
	MonitoredResource string `yaml:"monitored_resource"`
	// Mapping of expected label keys to value patterns.
	// Patterns are RE2 regular expressions.
	Labels map[string]string `yaml:"labels"`
	// If Optional is true, the test for this metric will be skipped.
	Optional bool `yaml:"optional,omitempty"`
	// Exactly one metric in each expected_metrics.yaml must
	// have Representative set to true. This metric can be used
	// to test that the integration is enabled.
	Representative bool `yaml:"representative,omitempty"`
}

// ValidateMetrics checks that all enum fields have valid values and that
// there is exactly one representative metric in the slice.
func ValidateMetrics(metrics []ExpectedMetric) error {
	var err error
	// Field validation
	expectedKinds := []string{"GAUGE", "DELTA", "CUMULATIVE"}
	expectedValueTypes := []string{"BOOL", "INT64", "DOUBLE", "STRING", "DISTRIBUTION"}
	expectedResource := "gce_instance"
	for _, metric := range metrics {
		innerErrs := make([]string, 0)
		if !SliceContains(expectedKinds, metric.Kind) {
			innerErrs = append(innerErrs, fmt.Sprintf("invalid kind %s (must be %v)", metric.Kind, expectedKinds))
		}
		if !SliceContains(expectedValueTypes, metric.ValueType) {
			innerErrs = append(innerErrs, fmt.Sprintf("invalid value_type %s (must be %v)", metric.ValueType, expectedValueTypes))
		}
		if expectedResource != metric.MonitoredResource {
			innerErrs = append(innerErrs, fmt.Sprintf("invalid monitored_resource %s (must be %v)", metric.MonitoredResource, expectedResource))
		}
		if len(innerErrs) > 0 {
			err = multierr.Append(err, fmt.Errorf("%s: %v", metric.Type, strings.Join(innerErrs, ", ")))
		}
	}
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
