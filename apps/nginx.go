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
			otel.TransformationMetrics(
				otel.SetScopeName("agent.googleapis.com/"+r.Type()),
				otel.SetScopeVersion("1.0"),
			),
		}},
	}}, nil
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverNginx{} })
}

type LoggingProcessorMacroNginxAccess struct {
}

func (LoggingProcessorMacroNginxAccess) Type() string {
	return "nginx_access"
}

func (p LoggingProcessorMacroNginxAccess) Expand(ctx context.Context) []confgenerator.InternalLoggingProcessor {
	return []confgenerator.InternalLoggingProcessor{
		confgenerator.LoggingProcessorParseRegex{
			// Documentation:
			// https://docs.nginx.com/nginx/admin-guide/monitoring/logging/#setting-up-the-access-log
			// Sample "common" line: 127.0.0.1 - frank [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.0" 200 2326
			// Sample "combined" line: ::1 - - [26/Aug/2021:16:49:43 +0000] "GET / HTTP/1.1" 200 10701 "-" "curl/7.64.0"
			Regex: `^(?<http_request_remoteIp>[^ ]*) (?<host>[^ ]*) (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<http_request_requestMethod>\S+)(?: +(?<http_request_requestUrl>[^\"]*?)(?: +(?<http_request_protocol>\S+))?)?" (?<http_request_status>[^ ]*) (?<http_request_responseSize>[^ ]*)(?: "(?<http_request_referer>[^\"]*)" "(?<http_request_userAgent>[^\"]*)")?(?: "(?<gzip_ratio>[^\"]*)")?$`,
			ParserShared: confgenerator.ParserShared{
				TimeKey:    "time",
				TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
				Types: map[string]string{
					"http_request_status": "integer",
				},
			},
		},
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				InstrumentationSourceLabel: instrumentationSourceValue(p.Type()),
			},
		},
	}
}

type LoggingProcessorMacroNginxError struct {
}

func (LoggingProcessorMacroNginxError) Type() string {
	return "nginx_error"
}

func (p LoggingProcessorMacroNginxError) Expand(ctx context.Context) []confgenerator.InternalLoggingProcessor {
	return []confgenerator.InternalLoggingProcessor{
		confgenerator.LoggingProcessorParseRegex{
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
		},
		// Log levels documented: https://github.com/nginx/nginx/blob/master/src/core/ngx_syslog.c#L31
		confgenerator.LoggingProcessorModifyFields{
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
		},
	}
}

type LoggingReceiverNginxAccess struct {
	LoggingProcessorMacroNginxAccess `yaml:",inline"`
	ReceiverMixin                    confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func loggingReceiverFilesMixinNginxAccess() confgenerator.LoggingReceiverFilesMixin {
	return confgenerator.LoggingReceiverFilesMixin{
		IncludePaths: []string{"/var/log/nginx/access.log"},
	}
}

type LoggingReceiverNginxError struct {
	LoggingProcessorMacroNginxError `yaml:",inline"`
	ReceiverMixin                   confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func loggingReceiverFilesMixinNginxError() confgenerator.LoggingReceiverFilesMixin {
	return confgenerator.LoggingReceiverFilesMixin{
		IncludePaths: []string{"/var/log/nginx/error.log"},
	}
}

func init() {
	confgenerator.RegisterLoggingFilesProcessorMacro[LoggingProcessorMacroNginxAccess](
		loggingReceiverFilesMixinNginxAccess)
	confgenerator.RegisterLoggingFilesProcessorMacro[LoggingProcessorMacroNginxError](
		loggingReceiverFilesMixinNginxError)
}
