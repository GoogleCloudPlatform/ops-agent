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

// TODO - Add support for Slow Query log & General Query log once multiline confgenerator support is implemented

type LoggingProcessorMysqlError struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorMysqlError) Type() string {
	return "mysql_error"
}

func (p LoggingProcessorMysqlError) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseRegex{
		// Format documented: https://dev.mysql.com/doc/refman/8.0/en/error-log-format.html
		// Older versions of mysql should have the same general format, but may not have tid, errorCode, subsystem.
		// Sample Line: 2020-08-06T14:25:02.936146Z 0 [Warning] [MY-010068] [Server] CA certificate /var/mysql/sslinfo/cacert.pem is self signed.
		// Sample Line: 2020-08-06T14:25:03.109022Z 5 [Note] Event Scheduler: scheduler thread started with id 5
		Regex: `^(?<time>\d{4}-\d{2}-\d{2}(?:T|\s)\d{2}:\d{2}:\d{2}.\d+(?:Z|[+-]\d{2}:\d{2})?)(?:\s+(?<tid>\d+))?(?:\s+\[(?<level>[^\]]+)])?(?:\s+\[(?<errorCode>[^\]]+)])?(?:\s+\[(?<subsystem>[^\]]+)])?\s+(?<message>.*)$`,
		LoggingProcessorParseShared: confgenerator.LoggingProcessorParseShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%z",
			Types: map[string]string{
				"tid": "integer",
			},
		},
	}.Components(tag, uid)
	for _, l := range []struct{ level, severity string }{
		{"ERROR", "ERROR"},
		{"Error", "ERROR"},
		{"WARNING", "WARNING"},
		{"Warning", "WARNING"},
		{"SYSTEM", "INFO"},
		{"System", "INFO"},
		{"NOTE", "NOTICE"},
		{"Note", "NOTICE"},
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

type LoggingReceiverMysqlError struct {
	LoggingProcessorMysqlError              `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverMysqlError) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{"/var/log/mysql/mysqld.log", "/var/log/mysql/error.log"}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorMysqlError.Components(tag, "mysql_error")...)
	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorMysqlError{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverMysqlError{} })
}
