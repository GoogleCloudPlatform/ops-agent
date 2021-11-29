package apps

import (
	"os"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverMemcached struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,hostname_port|file"`
}

const defaultMemcachedTCPEndpoint = "localhost:3306"
const defaultMemcachedUnixEndpoint = "/var/run/memcached/memcached.sock"

func (r MetricsReceiverMemcached) Type() string {
	return "memcached"
}

func (r MetricsReceiverMemcached) Pipelines() []otel.Pipeline {
	if r.Endpoint == "" {
		if _, err := os.Stat(defaultMemcachedUnixEndpoint); err != nil {
			r.Endpoint = defaultMemcachedUnixEndpoint
		} else {
			r.Endpoint = defaultMemcachedTCPEndpoint
		}
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
