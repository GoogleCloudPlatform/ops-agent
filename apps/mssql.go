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
        "github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
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
					// SAMPLE LOG ENTRIES, including multiline:
					//
					// 2022-03-20 00:00:00.27 spid56      Microsoft SQL Server 2019 (RTM-CU4) (KB4548597) - 15.0.4033.1 (X64) 
					// 	Mar 14 2020 16:10:35 
					// 	Copyright (C) 2019 Microsoft Corporation
					// 	Developer Edition (64-bit) on Windows Server 2019 Datacenter 10.0 <X64> (Build 17763: ) (Hypervisor)
					// 
					// 2022-03-20 00:00:00.28 spid56      UTC adjustment: -5:00
					// 2022-03-20 00:00:00.28 spid56      (c) Microsoft Corporation.
					// 2022-03-20 00:00:00.28 spid56      All rights reserved.
					// 2022-03-20 00:00:00.28 spid56      Server process ID is 3432.
					// 2022-03-20 00:00:00.28 spid56      System Manufacturer: 'Google', System Model: 'Google Compute Engine'.
					// 2022-03-20 00:00:00.28 spid56      Authentication mode is MIXED.
					// 2022-03-20 00:00:01.90 Backup      Log was backed up. Database: demo, creation date(time): 2020/01/31(10:33:17), first LSN: 582441:259880:1, last LSN: 582441:259912:1, number of dump devices: 1, device information: (FILE=1, TYPE=DISK: {'\\server\share\DatabaseBackups\demo.trn'}). This is an informational message only. No user action is required.
					// 2022-03-20 00:00:03.76 Logon       Error: 18456, Severity: 14, State: 38.
					//
                                        Regex: `^(?<logtime>\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}.\d{2}) (?<spid>\w+)\s+(?<message>[\s|\S]*)?`,
					// Not sending the log timestamp "logtime" from above in as the Time_Key as there will be time zone issues.
					// MS SQL Server writes logs in server time, not a standard time zone such as UTC
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

	// set default log level to info.  Couldn't get this to work via config.yaml
        comp := fluentbit.Component{
                        Kind: "FILTER",
                        Config: map[string]string{
                                "Name":      "modify",
                                "Match":     tag,
                                "Add":       "logging.googleapis.com/severity info",
                        },
                }

        c = append(c, comp)
	
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
