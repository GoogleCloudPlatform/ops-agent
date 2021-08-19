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

import "github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"

type MetricsReceiverNginx struct {
	ConfigComponent `yaml:",inline"`

	MetricsReceiverShared `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"required,url"`
}

func (r MetricsReceiverNginx) Type() string {
	return "nginx"
}

func (r MetricsReceiverNginx) Pipelines() []otel.Pipeline {
	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "nginx",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
				"endpoint":            r.Endpoint,
			},
		},
		Processors: []otel.Component{
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.AddPrefix("custom.googleapis.com"),
			),
		},
	}}
}

func init() {
	metricsReceiverTypes.registerType(func() component { return &MetricsReceiverNginx{} })
}
