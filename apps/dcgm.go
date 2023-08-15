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

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

type MetricsReceiverDcgm struct {
	confgenerator.ConfigComponent       `yaml:",inline"`
	confgenerator.MetricsReceiverShared `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,hostname_port"`
}

const defaultDcgmEndpoint = "localhost:5555"

func (r MetricsReceiverDcgm) Type() string {
	return "dcgm"
}

func (r MetricsReceiverDcgm) Pipelines(_ context.Context) []otel.ReceiverPipeline {
	if r.Endpoint == "" {
		r.Endpoint = defaultDcgmEndpoint
	}

	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type: "dcgm",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
				"endpoint":            r.Endpoint,
				"metrics": map[string]interface{}{
					"dcgm.gpu.utilization": map[string]bool{
						"enabled": false,
					},
					"dcgm.gpu.memory.bytes_used": map[string]bool{
						"enabled": false,
					},
				},
			},
		},
		Processors: map[string][]otel.Component{"metrics": {
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com"),
			),
			otel.ModifyInstrumentationScope(r.Type(), "1.0"),
		}},
	}}
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverDcgm{} }, platform.Linux)
}
