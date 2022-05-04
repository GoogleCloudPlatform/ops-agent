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

func (r MetricsReceiverVarnish) Pipelines() []otel.Pipeline {
	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "varnish",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
				"cache_dir":           r.CacheDir,
				"exec_dir":            r.ExecDir,
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
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverVarnish{} })
}

type LoggingProcessorVarnishAccess struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorVarnishAccess) Type() string {
	return "varnish_access"
}

func (p LoggingProcessorVarnishAccess) Components(tag string, uid string) []fluentbit.Component {
	return genericAccessLogParser(tag, uid)
}

type LoggingReceiverVarnishAccess struct {
	LoggingProcessorVarnishAccess           `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverVarnishAccess) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{"/var/log/varnish/varnishncsa.log"}
	}

	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorVarnishAccess.Components(tag, "varnish_access")...)
	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorVarnishAccess{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverVarnishAccess{} })
}
