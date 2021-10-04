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

type LoggingProcessorApacheAccess struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorApacheAccess) Type() string {
	return "apache_access"
}

func (p LoggingProcessorApacheAccess) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseRegex{
		// Documentation: https://httpd.apache.org/docs/current/logs.html#accesslog
		// Sample "common" line: 127.0.0.1 - frank [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.0" 200 2326
		// Sample "combined" line: ::1 - - [26/Aug/2021:16:49:43 +0000] "GET / HTTP/1.1" 200 10701 "-" "curl/7.64.0"
		Regex: `^(?<http_request_remoteIp>[^ ]*) (?<host>[^ ]*) (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<http_request_requestMethod>\S+)(?: +(?<http_request_requestUrl>[^\"]*?)(?: +(?<http_request_protocol>\S+))?)?" (?<http_request_status>[^ ]*) (?<http_request_responseSize>[^ ]*)(?: "(?<http_request_referer>[^\"]*)" "(?<http_request_userAgent>[^\"]*)")?$`,
		LoggingProcessorParseShared: confgenerator.LoggingProcessorParseShared{
			TimeKey:    "time",
			TimeFormat: "%d/%b/%Y:%H:%M:%S %z",
			Types: map[string]string{
				"http_request_status": "integer",
				// N.B. "http_request_responseSize" is a string containing an integer.
				// https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#HttpRequest.FIELDS.response_size
			},
		},
	}.Components(tag, uid)
	// apache logs "-" when a field does not have a value. Remove the field entirely when this happens.
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

type LoggingProcessorApacheError struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorApacheError) Type() string {
	return "apache_error"
}

func (p LoggingProcessorApacheError) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseRegex{
		// Documentation: https://httpd.apache.org/docs/current/logs.html#errorlog
		// Sample line 2.4: [Fri Sep 09 10:42:29.902022 2011] [core:error] [pid 35708:tid 4328636416] (13)Permission denied [client 72.15.99.187] File does not exist: /usr/local/apache2/htdocs/favicon.ico
		// 					[Thu Sep 30 03:18:29.239182 2021] [ssl:error] [pid 2451:tid 140169666050176] AH02217: ssl_stapling_init_cert: Can't retrieve issuer certificate!
		// Sample line 2.2: [Fri Sep 09 10:42:29.902022 2011] [error] [pid 35708:tid 4328636416] [client 72.15.99.187] File does not exist: /usr/local/apache2/htdocs/favicon.ico
		// TODO - Support time parsing for version 2.0 where smallest resolution is seconds
		// Sample line 2.0: [Wed Oct 11 14:32:52 2000] [error] [client 127.0.0.1] client denied by server configuration: /export/home/live/ap/htdocs/test
		Regex: `^\[(?<time>[^\]]+)\] \[(?:(?<module>\w+):)?(?<level>[\w\d]+)\](?: \[pid (?<pid>\d+)(?::tid (?<tid>[0-9]+))?\])?(?: (?<errorCode>[^\[:]*):?)?(?: \[client (?<client>[^\]]*)\])? (?<message>.*)$`,
		LoggingProcessorParseShared: confgenerator.LoggingProcessorParseShared{
			TimeKey:    "time",
			TimeFormat: "%a %b %d %H:%M:%S.%L %Y",
			Types: map[string]string{
				"pid": "integer",
				"tid": "integer",
			},
		},
	}.Components(tag, uid)
	for _, l := range []struct{ level, severity string }{
		// Log levels documented: https://httpd.apache.org/docs/2.4/mod/core.html#loglevel
		// Could separate traceN out into a Key_Value_Matches modify condition but left it in
		// the group for simplicity and clarity of the translations.
		{"emerg", "EMERGENCY"},
		{"alert", "ALERT"},
		{"crit", "CRITICAL"},
		{"error", "ERROR"},
		{"warn", "WARNING"},
		{"notice", "NOTICE"},
		{"info", "INFO"},
		{"debug", "DEBUG"},
		{"trace1", "DEBUG"},
		{"trace2", "DEBUG"},
		{"trace3", "DEBUG"},
		{"trace4", "DEBUG"},
		{"trace5", "DEBUG"},
		{"trace6", "DEBUG"},
		{"trace7", "DEBUG"},
		{"trace8", "DEBUG"},
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

type LoggingReceiverApacheAccess struct {
	LoggingProcessorApacheAccess            `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverApacheAccess) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			// Default log file path on Debian / Ubuntu
			"/var/log/apache2/access.log",
			// Default log file path RHEL / CentOS
			"/var/log/apache2/access_log",
			// Default log file path SLES
			"/var/log/httpd/access_log",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorApacheAccess.Components(tag, "apache_access")...)
	return c
}

type LoggingReceiverApacheError struct {
	LoggingProcessorApacheError             `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverApacheError) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			// Default log file path on Debian / Ubuntu
			"/var/log/apache2/error.log",
			// Default log file path RHEL / CentOS
			"/var/log/apache2/error_log",
			// Default log file path SLES
			"/var/log/httpd/error_log",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorApacheError.Components(tag, "apache_error")...)
	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorApacheAccess{} })
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorApacheError{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverApacheAccess{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverApacheError{} })
}
