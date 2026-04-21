package confgenerator

import (
	"os"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

func TestPortOverriddenByEnv(t *testing.T) {
	// Set env vars
	os.Setenv("OPS_AGENT_FLUENT_BIT_METRICS_PORT", "40002")
	os.Setenv("OPS_AGENT_OTEL_METRICS_PORT", "40001")
	defer os.Unsetenv("OPS_AGENT_FLUENT_BIT_METRICS_PORT")
	defer os.Unsetenv("OPS_AGENT_OTEL_METRICS_PORT")

	uc := &UnifiedConfig{}

	if port := uc.GetFluentBitMetricsPort(); port != 40002 {
		t.Errorf("Expected Fluent Bit port 40002, got %d", port)
	}

	if port := uc.GetOtelMetricsPort(); port != 40001 {
		t.Errorf("Expected OTel port 40001, got %d", port)
	}
}

func TestPortDefaultWhenEnvEmpty(t *testing.T) {
	// Ensure env vars are not set
	os.Unsetenv("OPS_AGENT_FLUENT_BIT_METRICS_PORT")
	os.Unsetenv("OPS_AGENT_OTEL_METRICS_PORT")

	uc := &UnifiedConfig{}

	if port := uc.GetFluentBitMetricsPort(); port != fluentbit.MetricsPort {
		t.Errorf("Expected Fluent Bit default port %d, got %d", fluentbit.MetricsPort, port)
	}

	if port := uc.GetOtelMetricsPort(); port != otel.MetricsPort {
		t.Errorf("Expected OTel default port %d, got %d", otel.MetricsPort, port)
	}
}

func TestPortInvalidEnvFallbacksToDefault(t *testing.T) {
	// Set invalid env vars
	os.Setenv("OPS_AGENT_FLUENT_BIT_METRICS_PORT", "invalid")
	os.Setenv("OPS_AGENT_OTEL_METRICS_PORT", "65536") // Out of range for uint16
	defer os.Unsetenv("OPS_AGENT_FLUENT_BIT_METRICS_PORT")
	defer os.Unsetenv("OPS_AGENT_OTEL_METRICS_PORT")

	uc := &UnifiedConfig{}

	if port := uc.GetFluentBitMetricsPort(); port != fluentbit.MetricsPort {
		t.Errorf("Expected Fluent Bit default port %d for invalid env, got %d", fluentbit.MetricsPort, port)
	}

	if port := uc.GetOtelMetricsPort(); port != otel.MetricsPort {
		t.Errorf("Expected OTel default port %d for invalid env, got %d", otel.MetricsPort, port)
	}
}
