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

type LoggingProcessorVarnish struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorVarnish) Type() string {
	return "varnish"
}

func (p LoggingProcessorVarnish) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseRegex{
		// Logging documentation: https://github.com/varnishcache/varnish-cache/blob/04455d6c3d8b2d810007239cb1cb2b740d7ec8ab/doc/sphinx/reference/varnishncsa.rst#format
		// Sample line: 127.0.0.1 - - [02/Mar/2022:15:55:05 +0000] "GET http://localhost:8080/test HTTP/1.1" 404 273 "-" "curl/7.64.0"
		Regex: `^(?<http_request_serverIp>[^ ]*) ([^ ]*) (?<http_request_remoteIp>[^ ]*) \[(?<time>[^\]]*)\] "(?<http_request_requestMethod>\S+)(?: +(?<http_request_requestUrl>[^\"]*?)(?: +(?<http_request_protocol>\S+))?)?" (?<http_request_status>[^ ]*) (?<http_request_responseSize>[^ ]*)(?: "(?<http_request_referer>[^\"]*)" "(?<http_request_userAgent>[^\"]*)")?$`,
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
		},
	}.Components(tag, uid)

	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorVarnish{} })
}
