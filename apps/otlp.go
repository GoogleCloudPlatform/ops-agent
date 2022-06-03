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

// TODO: The collector defaults to this, but should we default to 127.0.0.1 or ::1 instead?
const defaultGRPCEndpoint = "0.0.0.0:4317"

type ReceiverOTLP struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	GRPCEndpoint string `yaml:"grpc_endpoint" validate:"omitempty,hostname_port"`
}

func (r ReceiverOTLP) Type() string {
	return "otlp"
}

func (r ReceiverOTLP) Pipelines() []otel.Pipeline {
	endpoint := r.GRPCEndpoint
	if endpoint == "" {
		endpoint = defaultGRPCEndpoint
	}
	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "otlp",
			Config: map[string]interface{}{
				"grpc": map[string]interface{}{
					"endpoint": endpoint,
				},
			},
		},
		Processors: []otel.Component{
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com"),
			),
			// TODO: Set instrumentation_source labels, etc.
		},
	}}
}

func init() {
	confgenerator.GenericReceiverTypes.RegisterType(func() confgenerator.GenericReceiver { return &ReceiverOTLP{} })
}
