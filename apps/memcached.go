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

type MetricsReceiverMemcached struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,hostname_port|startswith=/"`
}

const defaultMemcachedTCPEndpoint = "localhost:11211"

func (r MetricsReceiverMemcached) Type() string {
	return "memcached"
}

func (r MetricsReceiverMemcached) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	if r.Endpoint == "" {
		r.Endpoint = defaultMemcachedTCPEndpoint
	}
	processors := []otel.Component{
		otel.MetricsFilter(
			"exclude",
			"strict",
			"memcached.operation_hit_ratio",
		),
		otel.NormalizeSums(),
		otel.MetricsTransform(
			otel.AddPrefix("workload.googleapis.com"),
		),
		otel.TransformationMetrics(
			otel.SetScopeName("agent.googleapis.com/"+r.Type()),
			otel.SetScopeVersion("1.0"),
		),
		otel.MetricsRemoveServiceAttributes(),
	}
	resource, _ := platform.FromContext(ctx).GetResource()
	exporter := otel.OTel
	if confgenerator.ExperimentsFromContext(ctx)["otlp_exporter"] {
		exporter = otel.OTLP
		processors = append(processors, otel.GCPProjectID(resource.ProjectName()))

	}
	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type: "memcached",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
				"endpoint":            r.Endpoint,
				"transport":           "tcp",
			},
		},
		Processors:    map[string][]otel.Component{"metrics": processors},
		ExporterTypes: map[string]otel.ExporterType{"metrics": exporter},
	}}, nil
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverMemcached{} })
}
