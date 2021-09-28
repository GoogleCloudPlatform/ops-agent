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

type MetricsReceiverApache struct {
	ConfigComponent `yaml:",inline"`

	MetricsReceiverShared `yaml:",inline"`

	ServerStatusURL string `yaml:"server_status_url" validate:"omitempty,url"`
}

const defaultServerStatusURL = "http://localhost:8080/server-status?auto"

func (r MetricsReceiverApache) Type() string {
	return "apache"
}

func (r MetricsReceiverApache) Pipelines() []otel.Pipeline {
	if r.ServerStatusURL == "" {
		r.ServerStatusURL = defaultServerStatusURL
	}
	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "httpd",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
				"endpoint":            r.ServerStatusURL,
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
	metricsReceiverTypes.registerType(func() component { return &MetricsReceiverApache{} })
}
