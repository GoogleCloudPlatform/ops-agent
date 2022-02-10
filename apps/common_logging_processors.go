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
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

func genericAccessLogParser(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseRegex{
		// Documentation:
		// https://httpd.apache.org/docs/current/logs.html#accesslog
		// https://docs.nginx.com/nginx/admin-guide/monitoring/logging/#setting-up-the-access-log
		// Sample "common" line: 127.0.0.1 - frank [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.0" 200 2326
		// Sample "combined" line: ::1 - - [26/Aug/2021:16:49:43 +0000] "GET / HTTP/1.1" 200 10701 "-" "curl/7.64.0"
		Regex: `^(?<http_request_remoteIp>[^ ]*) (?<host>[^ ]*) (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<http_request_requestMethod>\S+)(?: +(?<http_request_requestUrl>[^\"]*?)(?: +(?<http_request_protocol>\S+))?)?" (?<http_request_status>[^ ]*) (?<http_request_responseSize>[^ ]*)(?: "(?<http_request_referer>[^\"]*)" "(?<http_request_userAgent>[^\"]*)")?$`,
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
			Types: map[string]string{
				"http_request_status": "integer",
				// N.B. "http_request_responseSize" is a string containing an integer.
				// https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#HttpRequest.FIELDS.response_size
			},
		},
	}.Components(tag, uid)
	// apache/nginx logs "-" when a field does not have a value. Remove the field entirely when this happens.
	for _, field := range []string{
		"host",
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
