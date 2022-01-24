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
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverZookeeper struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,hostname_port"`
}

const defaultZookeeperEndpoint = "localhost:2181"

func (MetricsReceiverZookeeper) Type() string {
	return "zookeeper"
}

func (r MetricsReceiverZookeeper) Pipelines() []otel.Pipeline {
	if r.Endpoint == "" {
		r.Endpoint = defaultZookeeperEndpoint
	}

	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "zookeeper",
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
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverZookeeper{} })
}
