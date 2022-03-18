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
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverMssql struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`
}

func (MetricsReceiverMssql) Type() string {
	return "mssql"
}

func (m MetricsReceiverMssql) Pipelines() []otel.Pipeline {
	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "windowsperfcounters",
			Config: map[string]interface{}{
				"collection_interval": m.CollectionIntervalString(),
				"perfcounters": []map[string]interface{}{
					{
						"object":    "SQLServer:General Statistics",
						"instances": []string{"_Total"},
						"counters":  []string{"User Connections"},
					},
					{
						"object":    "SQLServer:Databases",
						"instances": []string{"_Total"},
						"counters": []string{
							"Transactions/sec",
							"Write Transactions/sec",
						},
					},
				},
			},
		},
		Processors: []otel.Component{
			otel.MetricsTransform(
				otel.RenameMetric(
					`\SQLServer:General Statistics(_Total)\User Connections`,
					"mssql/connections/user",
				),
				otel.RenameMetric(
					`\SQLServer:Databases(_Total)\Transactions/sec`,
					"mssql/transaction_rate",
				),
				otel.RenameMetric(
					`\SQLServer:Databases(_Total)\Write Transactions/sec`,
					"mssql/write_transaction_rate",
				),
				otel.AddPrefix("agent.googleapis.com"),
			),
		},
	}}
}

type LoggingProcessorMssqlLog struct {
        confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorMssqlLog) Type() string {
        return "mssql_log"
}

func (p LoggingProcessorMssqlLog) Components(tag string, uid string) []fluentbit.Component {
        c := confgenerator.LoggingProcessorParseMultilineRegex{
                LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
                        Parsers: []confgenerator.RegexParser{
                                {
                                        Regex: `^(?<logtime>\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}.\d{2}) (?<spid>.{11}) (?<message>[\s|\S]*)?`,
                                        //Regex: `^(?<logtime>\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}.\d{2}) (?<spid>.{11}) (?<message>.*)?`,
                                        Parser: confgenerator.ParserShared{
                                                Types: map[string]string{
                                                        "spid": "integer",
                                                },
                                        },
                                },
                        },
                },
                Rules: []confgenerator.MultilineRule{
                        {
                                StateName: "start_state",
                                NextState: "cont",
                                Regex:     `\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}.\d{2}`,
                        },
                        {
                                StateName: "cont",
                                NextState: "cont",
                                Regex:     `^(?!\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}.\d{2})`,
                        },
                },
        }.Components(tag, uid)

        return c
}

type LoggingReceiverMssqlLog struct {
        LoggingProcessorMssqlLog                `yaml:",inline"`
        confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverMssqlLog) Components(tag string) []fluentbit.Component {
        if len(r.IncludePaths) == 0 {
                r.IncludePaths = []string{
                        // Default log path for Linux installs
                        // https://docs.microsoft.com/en-us/sql/linux/sql-server-linux-overview
                        "/var/opt/mssql/log/errorlog",
                }
        }
        c := r.LoggingReceiverFilesMixin.Components(tag)
        c = append(c, r.LoggingProcessorMssqlLog.Components(tag, "mssql_log")...)
        return c
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverMssql{} }, "windows")
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorMssqlLog{} })
        confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverMssqlLog{} })
}
