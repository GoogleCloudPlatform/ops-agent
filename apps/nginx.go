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

type MetricsReceiverNginx struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`

	StubStatusURL string `yaml:"stub_status_url" validate:"omitempty,url"`
}

const defaultStubStatusURL = "http://127.0.0.1/status"

func (r MetricsReceiverNginx) Type() string {
	return "nginx"
}

func (r MetricsReceiverNginx) Pipelines(_ context.Context) ([]otel.ReceiverPipeline, error) {
	if r.StubStatusURL == "" {
		r.StubStatusURL = defaultStubStatusURL
	}
	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type: "nginx",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
				"endpoint":            r.StubStatusURL,
			},
		},
		Processors: map[string][]otel.Component{"metrics": {
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com"),
			),
			otel.ModifyInstrumentationScope(r.Type(), "1.0"),
		}},
	}}, nil
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverNginx{} })
}

type LoggingMultiProcessorMixinNginxAccess struct {
}

func (LoggingMultiProcessorMixinNginxAccess) Type() string {
	return "nginx_access"
}

func (p LoggingMultiProcessorMixinNginxAccess) Processors(ctx context.Context) []confgenerator.LoggingProcessorMixin {
	return genericAccessLogParser(ctx, p.Type())
}

type LoggingMultiProcessorMixinNginxError struct {
}

func (LoggingMultiProcessorMixinNginxError) Type() string {
	return "nginx_error"
}

func (p LoggingMultiProcessorMixinNginxError) Processors(ctx context.Context) []confgenerator.LoggingProcessorMixin {
	parseRegex := confgenerator.LoggingProcessorParseRegex{
		// Format is not documented, sadly.
		// Basic fields: https://github.com/nginx/nginx/blob/c231640eba9e26e963460c83f2907ac6f9abf3fc/src/core/ngx_log.c#L102
		// Request fields: https://github.com/nginx/nginx/blob/7bcb50c0610a18bf43bef0062b2d2dc550823b53/src/http/ngx_http_request.c#L3836
		// Sample line: 2021/08/26 16:50:17 [error] 29060#29060: *2191 open() "/var/www/html/forbidden.html" failed (13: Permission denied), client: ::1, server: _, request: "GET /forbidden.html HTTP/1.1", host: "localhost:8080"
		Regex: `^(?<time>[0-9]+[./-][0-9]+[./-][0-9]+[- ][0-9]+:[0-9]+:[0-9]+) \[(?<level>[^\]]*)\] (?<pid>[0-9]+)#(?<tid>[0-9]+):(?: \*(?<connection>[0-9]+))? (?<message>.*?)(?:, client: (?<client>[^,]+))?(?:, server: (?<server>[^,]+))?(?:, request: "(?<request>[^"]*)")?(?:, subrequest: \"(?<subrequest>[^\"]*)\")?(?:, upstream: \"(?<upstream>[^"]*)\")?(?:, host: \"(?<host>[^\"]*)\")?(?:, referrer: \"(?<referer>[^"]*)\")?$`,
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y/%m/%d %H:%M:%S",
			Types: map[string]string{
				"pid":        "integer",
				"tid":        "integer",
				"connection": "integer",
			},
		},
	}
	// Log levels documented: https://github.com/nginx/nginx/blob/master/src/core/ngx_syslog.c#L31
	modifyFields := confgenerator.LoggingProcessorModifyFields{
		Fields: map[string]*confgenerator.ModifyField{
			"severity": {
				CopyFrom: "jsonPayload.level",
				MapValues: map[string]string{
					"emerg":  "EMERGENCY",
					"alert":  "ALERT",
					"crit":   "CRITICAL",
					"error":  "ERROR",
					"warn":   "WARNING",
					"notice": "NOTICE",
					"info":   "INFO",
					"debug":  "DEBUG",
				},
				MapValuesExclusive: true,
			},
			InstrumentationSourceLabel: instrumentationSourceValue(p.Type()),
		},
	}
	return []confgenerator.LoggingProcessorMixin{parseRegex, modifyFields}
}

type LoggingReceiverMixinNginxAccess struct {
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverMixinNginxAccess) Components(ctx context.Context, tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{"/var/log/nginx/access.log"}
	}
	return r.LoggingReceiverFilesMixin.Components(ctx, tag)
}

type LoggingReceiverMixinNginxError struct {
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverMixinNginxError) Components(ctx context.Context, tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{"/var/log/nginx/error.log"}
	}
	return r.LoggingReceiverFilesMixin.Components(ctx, tag)
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.LoggingProcessor {
		return &confgenerator.LoggingMultiProcessor[LoggingMultiProcessorMixinNginxAccess]{}
	})
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.LoggingProcessor {
		return &confgenerator.LoggingMultiProcessor[LoggingMultiProcessorMixinNginxError]{}
	})

	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.LoggingReceiver {
		return &confgenerator.LoggingCompositeReceiver[LoggingReceiverMixinNginxAccess, LoggingMultiProcessorMixinNginxAccess]{}
	})
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.LoggingReceiver {
		return &confgenerator.LoggingCompositeReceiver[LoggingReceiverMixinNginxError, LoggingMultiProcessorMixinNginxError]{}
	})
}
