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
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverNginx struct {
	ConfigComponent `yaml:",inline"`

	MetricsReceiverShared `yaml:",inline"`

	StubStatusURL string `yaml:"stub_status_url" validate:"omitempty,url"`
}

const defaultStubStatusURL = "http://localhost/status"

func (r MetricsReceiverNginx) Type() string {
	return "nginx"
}

func (r MetricsReceiverNginx) Pipelines() []otel.Pipeline {
	if r.StubStatusURL == "" {
		r.StubStatusURL = defaultStubStatusURL
	}
	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "nginx",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
				"endpoint":            r.StubStatusURL,
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
	metricsReceiverTypes.registerType(func() component { return &MetricsReceiverNginx{} })
}

type LoggingProcessorNginxAccess struct {
	ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorNginxAccess) Type() string {
	return "nginx_access"
}

func (p LoggingProcessorNginxAccess) Components(tag string, uid string) []fluentbit.Component {
	c := LoggingProcessorParseRegex{
		// Sample line: ::1 - - [26/Aug/2021:16:49:43 +0000] "GET / HTTP/1.1" 200 10701 "-" "curl/7.64.0"
		// FIXME: legacy OA parser did not have http_x_forwarded_for, fluentd did
		//Regex: `^(?<remote>[^ ]*) (?<host>[^ ]*) (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")`,
		Regex: `^(?<http_request_remoteIp>[^ ]*) (?<host>[^ ]*) (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<http_request_requestMethod>\S+)(?: +(?<http_request_requestUrl>[^\"]*?)(?: +(?<http_request_protocol>\S+))?)?" (?<http_request_status>[^ ]*) (?<http_request_responseSize>[^ ]*)(?: "(?<http_request_referer>[^\"]*)" "(?<http_request_userAgent>[^\"]*)"(?:\s+(?<http_x_forwarded_for>[^ ]+))?)?$`,
		LoggingProcessorParseShared: LoggingProcessorParseShared{
			TimeKey:    "time",
			TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
			Types: map[string]string{
				"http_request_status": "integer",
				// N.B. "requestSize" is a string containing an integer.
				// https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#httprequest
			},
		},
	}.Components(tag, uid)
	// nginx logs "-" when a field does not have a value. Remove the field entirely when this happens.
	for _, field := range []string{
		"host", // always empty with default nginx config
		"user",
		"http_request_referer",
	} {
		c = append(c, fluentbit.Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":      "modify",
				"Match":     tag,
				"Condition": fmt.Sprintf("Key_Value_Equals %s -", field),
				"Remove":    field,
			},
		})
	}
	// Generate the httpRequest structure.
	c = append(c, fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":          "nest",
			"Match":         tag,
			"Operation":     "nest",
			"Wildcard":      "http_request_*",
			"Nest_under":    "logging.googleapis.com/http_request",
			"Remove_prefix": "http_request_",
		},
	})
	return c
}

type LoggingProcessorNginxError struct {
	ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorNginxError) Type() string {
	return "nginx_error"
}

func (p LoggingProcessorNginxError) Components(tag string, uid string) []fluentbit.Component {
	c := LoggingProcessorParseRegex{
		// Format is not documented, sadly.
		// Basic fields: https://github.com/nginx/nginx/blob/c231640eba9e26e963460c83f2907ac6f9abf3fc/src/core/ngx_log.c#L102
		// Request fields: https://github.com/nginx/nginx/blob/7bcb50c0610a18bf43bef0062b2d2dc550823b53/src/http/ngx_http_request.c#L3836
		// Sample line: 2021/08/26 16:50:17 [error] 29060#29060: *2191 open() "/var/www/html/forbidden.html" failed (13: Permission denied), client: ::1, server: _, request: "GET /forbidden.html HTTP/1.1", host: "localhost:8080"
		Regex: `^(?<time>[0-9]+[./-][0-9]+[./-][0-9]+[- ][0-9]+:[0-9]+:[0-9]+) \[(?<level>[^\]]*)\] (?<pid>[0-9]+)#(?<tid>[0-9]+):(?: \*(?<connection>[0-9]+))? (?<message>.*?)(?:, client: (?<client>[^,]+))?(?:, server: (?<server>[^,]+))?(?:, request: "(?<request>[^"]*)")?(?:, subrequest: \"(?<subrequest>[^\"]*)\")?(?:, upstream: \"(?<upstream>[^"]*)\")?(?:, host: \"(?<host>[^\"]*)\")?(?:, referrer: \"(?<referer>[^"]*)\")?$`,
		LoggingProcessorParseShared: LoggingProcessorParseShared{
			TimeKey:    "time",
			TimeFormat: "%Y/%m/%d %H:%M:%S",
			Types: map[string]string{
				"pid":        "integer",
				"tid":        "integer",
				"connection": "integer",
			},
		},
	}.Components(tag, uid)
	for _, l := range []struct{ level, severity string }{
		{"emerg", "EMERGENCY"},
		{"alert", "ALERT"},
		{"crit", "CRITICAL"},
		{"error", "ERROR"},
		{"warn", "WARNING"},
		{"notice", "NOTICE"},
		{"info", "INFO"},
		{"debug", "DEBUG"},
	} {
		c = append(c, fluentbit.Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":      "modify",
				"Match":     tag,
				"Condition": fmt.Sprintf("Key_Value_Equals level %s", l.level),
				"Add":       fmt.Sprintf("logging.googleapis.com/severity %s", l.severity),
			},
		})
	}
	return c
}

type LoggingReceiverNginxAccess struct {
	LoggingProcessorNginxAccess `yaml:",inline"`
	LoggingReceiverFilesMixin   `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverNginxAccess) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{"/var/log/nginx/access.log"}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorNginxAccess.Components(tag, "nginx_access")...)
	return c
}

type LoggingReceiverNginxError struct {
	LoggingProcessorNginxError `yaml:",inline"`
	LoggingReceiverFilesMixin  `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverNginxError) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{"/var/log/nginx/error.log"}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorNginxError.Components(tag, "nginx_error")...)
	return c
}

func init() {
	loggingProcessorTypes.registerType(func() component { return &LoggingProcessorNginxAccess{} })
	loggingProcessorTypes.registerType(func() component { return &LoggingProcessorNginxError{} })
	loggingReceiverTypes.registerType(func() component { return &LoggingReceiverNginxAccess{} })
	loggingReceiverTypes.registerType(func() component { return &LoggingReceiverNginxError{} })
}
