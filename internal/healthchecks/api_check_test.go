package healthchecks

import (
	"errors"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
	"github.com/GoogleCloudPlatform/ops-agent/internal/logs"
	"gotest.tools/v3/assert"
)

func TestAPICheck_RunCheck_Legacy(t *testing.T) {
	// Save original functions and restore them later
	origRunMonitoringCheck := runMonitoringCheckFunc
	origRunLoggingCheck := runLoggingCheckFunc
	defer func() {
		runMonitoringCheckFunc = origRunMonitoringCheck
		runLoggingCheckFunc = origRunLoggingCheck
	}()

	// Mock functions
	monitoringCalled := false
	loggingCalled := false
	runMonitoringCheckFunc = func(logger logs.StructuredLogger, resource resourcedetector.Resource) error {
		monitoringCalled = true
		return nil
	}
	runLoggingCheckFunc = func(logger logs.StructuredLogger, resource resourcedetector.Resource) error {
		loggingCalled = true
		return nil
	}

	c := APICheck{}
	logger, _ := logs.DiscardLogger()
	err := c.RunCheck(logger)

	assert.NilError(t, err)
	assert.Check(t, monitoringCalled, "monitoring check should be called")
	assert.Check(t, loggingCalled, "logging check should be called")
}

func TestAPICheck_RunCheck_Telemetry(t *testing.T) {
	// Save original functions and restore them later
	origRunTelemetryMetricsCheck := runTelemetryMetricsCheckFunc
	origRunTelemetryLogsCheck := runTelemetryLogsCheckFunc
	defer func() {
		runTelemetryMetricsCheckFunc = origRunTelemetryMetricsCheck
		runTelemetryLogsCheckFunc = origRunTelemetryLogsCheck
	}()

	// Mock functions
	metricsCalled := false
	logsCalled := false
	runTelemetryMetricsCheckFunc = func(logger logs.StructuredLogger, resource resourcedetector.Resource) error {
		metricsCalled = true
		return nil
	}
	runTelemetryLogsCheckFunc = func(logger logs.StructuredLogger, resource resourcedetector.Resource) error {
		logsCalled = true
		return nil
	}

	c := APICheck{
		Experiments: map[string]bool{"otlp_exporter": true},
	}
	logger, _ := logs.DiscardLogger()
	err := c.RunCheck(logger)

	assert.NilError(t, err)
	assert.Check(t, metricsCalled, "telemetry metrics check should be called")
	assert.Check(t, logsCalled, "telemetry logs check should be called")
}

func TestAPICheck_RunCheck_TelemetryError(t *testing.T) {
	// Save original functions and restore them later
	origRunTelemetryMetricsCheck := runTelemetryMetricsCheckFunc
	origRunTelemetryLogsCheck := runTelemetryLogsCheckFunc
	defer func() {
		runTelemetryMetricsCheckFunc = origRunTelemetryMetricsCheck
		runTelemetryLogsCheckFunc = origRunTelemetryLogsCheck
	}()

	// Mock functions returning errors
	runTelemetryMetricsCheckFunc = func(logger logs.StructuredLogger, resource resourcedetector.Resource) error {
		return errors.New("metrics error")
	}
	runTelemetryLogsCheckFunc = func(logger logs.StructuredLogger, resource resourcedetector.Resource) error {
		return errors.New("logs error")
	}

	c := APICheck{
		Experiments: map[string]bool{"otlp_exporter": true},
	}
	logger, _ := logs.DiscardLogger()
	err := c.RunCheck(logger)

	assert.ErrorContains(t, err, "metrics error")
	assert.ErrorContains(t, err, "logs error")
}
