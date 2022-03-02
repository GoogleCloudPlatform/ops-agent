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

type MetricsReceiverActivemq struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	Endpoint                               string `yaml:"endpoint" validate:"omitempty,hostname_port|startswith=service:jmx:"`
	Username                               string `yaml:"username" validate:"required_with=Password"`
	Password                               string `yaml:"password" validate:"required_with=Username"`
	confgenerator.MetricsReceiverSharedJVM `yaml:",inline"`

	confgenerator.MetricsReceiverSharedCollectJVM `yaml:",inline"`
}

const defaultActivemqEndpoint = "localhost:1099"

func (r MetricsReceiverActivemq) Type() string {
	return "activemq"
}

func (r MetricsReceiverActivemq) Pipelines() []otel.Pipeline {

	targetSystem := "activemq"

	return r.MetricsReceiverSharedJVM.
		WithDefaultEndpoint(defaultActivemqEndpoint).
		ConfigurePipelines(
			targetSystem,
			[]otel.Component{
				otel.NormalizeSums(),
				otel.MetricsTransform(
					otel.AddPrefix("workload.googleapis.com"),
				),
			},
		)
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverActivemq{} })
}
