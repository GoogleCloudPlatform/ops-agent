package apps

import (
	"context"
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type ReceiverAutoInstrumentation struct {
	confgenerator.ConfigComponent `yaml:",inline"`
	GenerateToPath                string `yaml:"generate_to_path" validate:"required"`
	GenerationType                string `yaml:"generation_type" validate:"required"`
	TargetReceiver                string `yaml:"target_receiver" validate:"required"`
	MetricsEnabled                bool   `yaml:"metrics_enabled"`
	TraceEnabled                  bool   `yaml:"trace_enabled"`
	ServiceName                   string `yaml:"-"`
	Endpoint                      string `yaml:"-"`
}

func (r *ReceiverAutoInstrumentation) Pipelines(_ context.Context) []otel.ReceiverPipeline {
	return nil
}

func (r *ReceiverAutoInstrumentation) Type() string {
	return "autoinstrumentation"
}

func (r *ReceiverAutoInstrumentation) validateTargetReceiver(uc confgenerator.UnifiedConfig) bool {
	if _, ok := uc.Combined.Receivers[r.TargetReceiver]; ok {
		return true
	}
	return false
}

func (r *ReceiverAutoInstrumentation) GenerateConfig() (map[string]interface{}, error) {
	config := map[string]interface{}{
		"otel.traces.exporter":                "otlp",
		"otel.metrics.exporter":               "otlp",
		"otel.logs.exporter":                  "none",
		"otel.exporter.otlp.traces.endpoint":  fmt.Sprintf("http://%s", defaultGRPCEndpoint),
		"otel.exporter.otlp.metrics.endpoint": fmt.Sprintf("http://%s", defaultGRPCEndpoint),
	}

	if r.Endpoint != "" {
		endpoint := fmt.Sprintf("http://%s", r.Endpoint)
		config["otel.exporter.otlp.traces.endpoint"] = endpoint
		config["otel.exporter.otlp.metrics.endpoint"] = endpoint
	}

	return config, nil
}

func (r *ReceiverAutoInstrumentation) GetTargetReceiver() string {
	return r.TargetReceiver
}

func (r *ReceiverAutoInstrumentation) GetGenerationType() string {
	return r.GenerationType
}

func (r *ReceiverAutoInstrumentation) GetGenerateToPath() string {
	return r.GenerateToPath
}

func (r *ReceiverAutoInstrumentation) SetServiceName(name string) {
	r.ServiceName = name
}

func (r *ReceiverAutoInstrumentation) SetEndpoint(endpoint string) {
	r.Endpoint = endpoint
}

func init() {
	confgenerator.CombinedReceiverTypes.RegisterType(func() confgenerator.CombinedReceiver { return &ReceiverAutoInstrumentation{} })
}
