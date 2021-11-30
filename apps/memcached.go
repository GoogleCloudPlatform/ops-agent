package apps

import (
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverMemcached struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,hostname_port|file"`
}

const defaultMemcachedTCPEndpoint = "localhost:11211"
const defaultMemcachedUnixEndpoint = "/var/run/memcached/memcached.sock"

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
