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

package confgenerator

import (
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverMySQL struct {
	ConfigComponent `yaml:",inline"`

	MetricsReceiverShared `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,url"`

	Password string `yaml:"password" validate:"omitempty"`

	Username string `yaml:"username" validate:"omitempty"`
}

const defaultEndpoint = "localhost:3306"

func (r MetricsReceiverMySQL) Type() string {
	return "mysql"
}

func (r MetricsReceiverMySQL) Pipelines() []otel.Pipeline {
	if r.Endpoint == "" {
		r.Endpoint = defaultEndpoint
	}
	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "mysql",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
				"endpoint":            r.Endpoint,
				"username":            r.Username,
				"password":            r.Password,
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
	metricsReceiverTypes.registerType(func() component { return &MetricsReceiverMySQL{} })
}
