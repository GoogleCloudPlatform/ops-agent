// Copyright 2025 Google LLC
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
)

type OTLPJsonFileReceiver struct {
	confgenerator.ConfigComponent `yaml:",inline"`
	IncludePaths            []string       `yaml:"include_paths" validate:"required,min=1"`
	ExcludePaths            []string       `yaml:"exclude_paths,omitempty"`
}

func (r OTLPJsonFileReceiver) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	// operators := []map[string]any{}
	receiver_config := map[string]any{
		"include":           r.IncludePaths,
		"exclude":           r.ExcludePaths,
	}

	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type: "otlpjsonfile",
			Config: receiver_config,
		},
			ExporterTypes: map[string]otel.ExporterType{
		"metrics": otel.OTel,
	},
			Processors: map[string][]otel.Component{
			"metrics": nil,
		},
	}}, nil
}

func (r OTLPJsonFileReceiver) Type() string {
	return "otlpjsonfile"
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &OTLPJsonFileReceiver{} })
}