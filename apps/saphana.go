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

type MetricsReceiverSapHana struct {
	confgenerator.ConfigComponent          `yaml:",inline"`
	confgenerator.MetricsReceiverSharedTLS `yaml:",inline"`
	confgenerator.MetricsReceiverShared    `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,hostname_port|startswith=/"`

	Password string `yaml:"password" validate:"omitempty"`
	Username string `yaml:"username" validate:"omitempty"`
}

const defaultSapHanaEndpoint = "localhost:30015"

func (s MetricsReceiverSapHana) Type() string {
	return "saphana"
}

func (s MetricsReceiverSapHana) Pipelines() []otel.Pipeline {
	if s.Endpoint == "" {
		s.Endpoint = defaultSapHanaEndpoint
	}

	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "saphana",
			Config: map[string]interface{}{
				"collection_interval": s.CollectionIntervalString(),
				"endpoint":            s.Endpoint,
				"password":            s.Password,
				"username":            s.Username,
				"tls":                 s.TLSConfig(true),
			},
		},
		Processors: []otel.Component{
			otel.MetricsFilter(
				"exclude",
				"strict",
				"saphana.uptime",
			),
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com"),
			),
			otel.TransformationMetrics(
				otel.FlattenResourceAttribute("saphana.host", "host"),
			),
		},
	}}
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverSapHana{} })
}
