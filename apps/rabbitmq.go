// Copyright 2021 Google LLC
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

type MetricsReceiverRabbitmq struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared    `yaml:",inline"`
	confgenerator.MetricsReceiverSharedTLS `yaml:",inline"`

	Password string `yaml:"password" validate:"required"`
	Username string `yaml:"username" validate:"required"`
	Endpoint string `yaml:"endpoint" validate:"omitempty,url"`
}

const defaultRabbitmqTCPEndpoint = "http://localhost:15672"

func (r MetricsReceiverRabbitmq) Type() string {
	return "rabbitmq"
}

func (r MetricsReceiverRabbitmq) Pipelines() []otel.Pipeline {
	if r.Endpoint == "" {
		r.Endpoint = defaultRabbitmqTCPEndpoint
	}

	cfg := map[string]interface{}{
		"collection_interval": r.CollectionIntervalString(),
		"endpoint":            r.Endpoint,
		"username":            r.Username,
		"password":            r.Password,
		"tls":                 r.TLSConfig(true),
	}

	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type:   "rabbitmq",
			Config: cfg,
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
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverRabbitmq{} })
}
