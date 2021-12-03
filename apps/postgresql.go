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
)

type LoggingProcessorPostgresql struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorPostgresql) Type() string {
	return "postgresql_general"
}

func (p LoggingProcessorPostgresql) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: []confgenerator.RegexParser{
				{
					// Limited documentation: https://dev.postgresql.com/doc/refman/8.0/en/query-log.html
					// Sample line: 2021-10-12T01:12:37.732966Z        14 Connect   root@localhost on  using Socket
					// Sample line: 2021-10-12T01:12:37.733135Z        14 Query     select @@version_comment limit 1
					Regex: `^(?<time>\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}.\d{3,} \w+)\s+\[(?<tid>\d+)\](?:\s+(?<role>.*)@(?<user>.*))? (?<level>\w+):\s+(?<message>.*)$`,
					Parser: confgenerator.ParserShared{
						TimeKey:    "time",
						TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%z",
						Types: map[string]string{
							"tid": "integer",
						},
					},
				},
			},
		},
		Rules: []confgenerator.MultilineRule{
			{
				StateName: "start_state",
				NextState: "cont",
				Regex:     `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d+Z`,
			},
			{
				StateName: "cont",
				NextState: "cont",
				Regex:     `^(?!\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d+Z)`,
			},
		},
	}.Components(tag, uid)

	return c
}

type LoggingReceiverPostgresql struct {
	LoggingProcessorPostgresql              `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverPostgresql) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			// Default log path for CentOS / RHEL / SLES / Debain / Ubuntu
			"/var/lib/postgresql/${HOSTNAME}.log",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorPostgresql.Components(tag, "postgresql")...)
	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorPostgresql{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverPostgresql{} })
}
