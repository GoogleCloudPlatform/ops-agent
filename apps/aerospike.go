// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package apps

import (
	"context"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/internal/secret"
)

// MetricsReceiverAerospike is configuration for the Aerospike metrics receiver
type MetricsReceiverAerospike struct {
	confgenerator.ConfigComponent              `yaml:",inline"`
	confgenerator.MetricsReceiverShared        `yaml:",inline"`
	confgenerator.MetricsReceiverSharedCluster `yaml:",inline"`

	Endpoint string        `yaml:"endpoint" validate:"omitempty,hostname_port"`
	Username string        `yaml:"username" validate:"required_with=Password"`
	Password secret.String `yaml:"password" validate:"required_with=Username"`
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
func (r MetricsReceiverAerospike) Pipelines(_ context.Context) ([]otel.ReceiverPipeline, error) {
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

	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type: "aerospike",
			Config: map[string]interface{}{
				"collection_interval":     collectionInterval,
				"endpoint":                endpoint,
				"collect_cluster_metrics": collectClusterMetrics,
				"username":                r.Username,
				"password":                r.Password.SecretValue(),
				"timeout":                 timeout,
			},
		},
		Processors: map[string][]otel.Component{"metrics": {
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com"),
			),
			otel.TransformationMetrics(
				otel.FlattenResourceAttribute("aerospike.node.name", "node_name"),
				otel.FlattenResourceAttribute("aerospike.namespace", "namespace_name"),
			),
			otel.ModifyInstrumentationScope(r.Type(), "1.0"),
		}},
	}}, nil
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverAerospike{} })
}
