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

var (
	defaultAerospikeTimeout            = 20 * time.Second
	defaultAerospikeCollectionInterval = 60 * time.Second
)

// Pipelines is the OTEL pipelines created from MetricsReceiverAerospike
func (r MetricsReceiverAerospike) Pipelines() []otel.Pipeline {
	if r.Endpoint == "" {
		r.Endpoint = defaultAerospikeEndpoint
	}

	collectClusterMetrics := defaultCollectClusterMetrics
	if r.CollectClusterMetrics != nil {
		collectClusterMetrics = *r.CollectClusterMetrics
	}

	timeout := defaultAerospikeTimeout
	if r.Timeout != 0 {
		timeout = r.Timeout
	}

	collectionInterval := defaultAerospikeCollectionInterval.String()
	if r.CollectionInterval != "" {
		collectionInterval = r.CollectionInterval
	}

	endpoint := defaultAerospikeEndpoint
	if r.Endpoint != "" {
		endpoint = r.Endpoint
	}

	return []otel.Pipeline{
		{
			Receiver: otel.Component{
				Type: "aerospike",
				Config: map[string]interface{}{
					"collection_interval":     collectionInterval,
					"endpoint":                endpoint,
					"collect_cluster_metrics": collectClusterMetrics,
					"username":                r.Username,
					"password":                r.Password,
					"timeout":                 timeout,
				},
			},
			Processors: []otel.Component{
				otel.NormalizeSums(),
				otel.MetricsTransform(
					otel.AddPrefix("workload.googleapis.com"),
				),
				otel.TransformationMetrics(
					otel.FlattenResourceAttribute("aerospike.node.name", "node_name"),
					otel.FlattenResourceAttribute("aerospike.namespace.name", "namespace_name"),
				),
			},
		},
	}
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverAerospike{} })
}
