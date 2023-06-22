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
	"context"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverVarnish struct {
	confgenerator.ConfigComponent       `yaml:",inline"`
	confgenerator.MetricsReceiverShared `yaml:",inline"`
	CacheDir                            string `yaml:"cache_dir" validate:"omitempty"`
	ExecDir                             string `yaml:"exec_dir" validate:"omitempty"`
}

func (MetricsReceiverVarnish) Type() string {
	return "varnish"
}

func (r MetricsReceiverVarnish) Pipelines() []otel.ReceiverPipeline {
	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type: "varnish",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
				"cache_dir":           r.CacheDir,
				"exec_dir":            r.ExecDir,
			},
		},
		Processors: map[string][]otel.Component{"metrics": {
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com"),
			),
			otel.ModifyInstrumentationScope(r.Type(), "1.0"),
		}},
	}}
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverVarnish{} })
}

type LoggingProcessorVarnish struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorVarnish) Type() string {
	return "varnish"
}

func (p LoggingProcessorVarnish) Components(ctx context.Context, tag string, uid string) []fluentbit.Component {
	// Logging documentation: https://github.com/varnishcache/varnish-cache/blob/04455d6c3d8b2d810007239cb1cb2b740d7ec8ab/doc/sphinx/reference/varnishncsa.rst#format
	// Sample line: 127.0.0.1 - - [02/Mar/2022:15:55:05 +0000] "GET http://localhost:8080/test HTTP/1.1" 404 273 "-" "curl/7.64.0"
	return genericAccessLogParser(ctx, p.Type(), tag, uid)
}

type LoggingReceiverVarnish struct {
	LoggingProcessorVarnish                 `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverVarnish) Components(ctx context.Context, tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{"/var/log/varnish/varnishncsa.log"}
	}

	c := r.LoggingReceiverFilesMixin.Components(ctx, tag)
	c = append(c, r.LoggingProcessorVarnish.Components(ctx, tag, "varnish")...)
	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.LoggingProcessor { return &LoggingProcessorVarnish{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.LoggingReceiver { return &LoggingReceiverVarnish{} })
}
