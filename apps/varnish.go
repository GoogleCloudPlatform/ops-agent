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

func (r MetricsReceiverVarnish) Pipelines(_ context.Context) ([]otel.ReceiverPipeline, error) {
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
			otel.TransformationMetrics(
				otel.SetScopeName("agent.googleapis.com/"+r.Type()),
				otel.SetScopeVersion("1.0"),
			),
		}},
	}}, nil
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverVarnish{} })
}

type LoggingProcessorMacroVarnish struct {
}

func (LoggingProcessorMacroVarnish) Type() string {
	return "varnish"
}

func (p LoggingProcessorMacroVarnish) Expand(ctx context.Context) []confgenerator.InternalLoggingProcessor {
	// Logging documentation: https://github.com/varnishcache/varnish-cache/blob/04455d6c3d8b2d810007239cb1cb2b740d7ec8ab/doc/sphinx/reference/varnishncsa.rst#format
	// Sample line: 127.0.0.1 - - [02/Mar/2022:15:55:05 +0000] "GET http://localhost:8080/test HTTP/1.1" 404 273 "-" "curl/7.64.0"
	return genericAccessLogParserAsInternalLoggingProcessor(ctx, p.Type()) // TODO: Wait for https://github.com/GoogleCloudPlatform/ops-agent/pull/1978 to be merged and/or any follow-up improvements to the genericAccessLogParser
}

func loggingReceiverFilesMixinVarnish() confgenerator.LoggingReceiverFilesMixin {
	return confgenerator.LoggingReceiverFilesMixin{
		IncludePaths: []string{"/var/log/varnish/varnishncsa.log"},
	}
}

func init() {
	confgenerator.RegisterLoggingProcessorMacro[LoggingProcessorMacroVarnish](loggingProcessorMacroVarnish)
}
