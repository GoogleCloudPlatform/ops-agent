package apps

import (
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

// MetricsReceiverAerospike is configuration for the Aerospike metrics receiver
type MetricsReceiverAerospike struct {
	confgenerator.ConfigComponent              `yaml:",inline"`
	confgenerator.MetricsReceiverShared        `yaml:",inline"`
	confgenerator.MetricsReceiverSharedCluster `yaml:",inline"`

	Endpoint string        `yaml:"endpoint" validate:"omitempty,hostname_port"`
	Username string        `yaml:"username" validate:"required_with=Password"`
	Password string        `yaml:"password" validate:"required_with=Username"`
	Timeout  time.Duration `yaml:"timeout"`
}

// Type is the MetricsReceiverType for the Aerospike metrics receiver
func (r MetricsReceiverAerospike) Type() string {
	return "aerospike"
}

const (
	defaultAerospikeEndpoint     = "localhost:3000"
	defaultCollectClusterMetrics = false // We assume the agent's running on each node
)

// Pipelines is the OTEL pipelines created from MetricsReceiverAerospike
func (r MetricsReceiverAerospike) Pipelines() []otel.Pipeline {
	if r.Endpoint == "" {
		r.Endpoint = defaultAerospikeEndpoint
	}

	return []otel.Pipeline{
		{
			Receiver: otel.Component{
				Type: "aerospike",
				Config: map[string]interface{}{
					"collection_interval":     r.CollectionInterval,
					"endpoint":                r.Endpoint,
					"collect_cluster_metrics": r.CollectClusterMetrics,
					"username":                r.Username,
					"password":                r.Password,
					"timeout":                 r.Timeout,
				},
			},
			Processors: []otel.Component{
				otel.NormalizeSums(),
				otel.MetricsTransform(
					otel.AddPrefix("workload.googleapis.com"),
				),
			},
		},
	}
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverAerospike{} })
}
