package common

// expectedMetric encodes a series of assertions about what data we expect
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
