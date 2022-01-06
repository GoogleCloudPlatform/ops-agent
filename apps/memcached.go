package apps

import (
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverMemcached struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,hostname_port|startswith=/"`
}

const defaultMemcachedTCPEndpoint = "localhost:11211"

func (r MetricsReceiverMemcached) Type() string {
	return "memcached"
}

func (r MetricsReceiverMemcached) Pipelines() []otel.Pipeline {
	if r.Endpoint == "" {
		r.Endpoint = defaultMemcachedTCPEndpoint
	}

	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "memcached",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
				"endpoint":            r.Endpoint,
			},
		},
		Processors: []otel.Component{
			otel.MetricsFilter(
				"exclude",
				"strict",
				"memcached.operation_hit_ratio",
			),
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com"),
			),
		},
	}}
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverMemcached{} })
}
