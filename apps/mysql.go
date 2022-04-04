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

	"fmt"
	"strings"
)

type MetricsReceiverMySql struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,hostname_port|startswith=/"`

	Password string `yaml:"password" validate:"omitempty"`
	Username string `yaml:"username" validate:"omitempty"`
}

const defaultMySqlUnixEndpoint = "/var/run/mysqld/mysqld.sock"

func (r MetricsReceiverMySql) Type() string {
	return "mysql"
}

func (r MetricsReceiverMySql) Pipelines() []otel.Pipeline {
	transport := "tcp"
	if r.Endpoint == "" {
		transport = "unix"
		r.Endpoint = defaultMySqlUnixEndpoint
	} else if strings.HasPrefix(r.Endpoint, "/") {
		transport = "unix"
	}

	if r.Username == "" {
		r.Username = "root"
	}

	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "mysql",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
				"endpoint":            r.Endpoint,
				"username":            r.Username,
				"password":            r.Password,
				"transport":           transport,
			},
		},
		Processors: []otel.Component{
			otel.NormalizeSums(),
			otel.MetricsTransform(
				// The following changes are here to ensure maximum backwards compatibility after the fixes
				// introduced https://github.com/open-telemetry/opentelemetry-collector-contrib/pull/7924
				otel.ChangePrefix("mysql\\.buffer_pool\\.", "mysql.buffer_pool_"),
				otel.UpdateMetric("mysql.buffer_pool_pages",
					otel.ToggleScalarDataType,
				),
				otel.UpdateMetric("mysql.threads",
					otel.ToggleScalarDataType,
				),
				otel.RenameMetric("mysql.buffer_pool_usage", "mysql.buffer_pool_size",
					otel.RenameLabel("status", "kind"),
					otel.ToggleScalarDataType,
				),
				otel.AddPrefix("workload.googleapis.com"),
			),
		},
	}}
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverMySql{} })
}

type LoggingProcessorMysqlError struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorMysqlError) Type() string {
	return "mysql_error"
}

func (p LoggingProcessorMysqlError) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseRegexComplex{
		Parsers: []confgenerator.RegexParser{
			{
				// MySql >=5.7 documented: https://dev.mysql.com/doc/refman/8.0/en/error-log-format.html
				// Sample Line: 2020-08-06T14:25:02.936146Z 0 [Warning] [MY-010068] [Server] CA certificate /var/mysql/sslinfo/cacert.pem is self signed.
				// Sample Line: 2020-08-06T14:25:03.109022Z 5 [Note] Event Scheduler: scheduler thread started with id 5
				Regex: `^(?<time>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d+(?:Z|[+-]\d{2}:?\d{2})?)\s+(?<tid>\d+)\s+\[(?<level>[^\]]+)](?:\s+\[(?<errorCode>[^\]]+)])?(?:\s+\[(?<subsystem>[^\]]+)])?\s+(?<message>.*)$`,
				Parser: confgenerator.ParserShared{
					TimeKey:    "time",
					TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%z",
					Types: map[string]string{
						"tid": "integer",
					},
				},
			},
			{
				// Mysql <5.7, MariaDB <10.1.4, documented: https://mariadb.com/kb/en/error-log/#format
				// Sample Line: 160615 16:53:08 [Note] InnoDB: The InnoDB memory heap is disabled
				// TODO - time is in system time, not UTC, is there a way to resolve this in fluent bit?
				Regex: `^(?<time>\d{6} \d{2}:\d{2}:\d{2})\s+\[(?<level>[^\]]+)]\s+(?<message>.*)$`,
				Parser: confgenerator.ParserShared{
					TimeKey:    "time",
					TimeFormat: "%y%m%d %H:%M:%S",
				},
			},
			{
				// MariaDB >=10.1.5, documented: https://mariadb.com/kb/en/error-log/#format
				// Sample Line: 2016-06-15 16:53:33 139651251140544 [Note] InnoDB: The InnoDB memory heap is disabled
				Regex: `^(?<time>\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})(?:\s+(?<tid>\d+))?(?:\s+\[(?<level>[^\]]+)])?\s+(?<message>.*)$`,
				Parser: confgenerator.ParserShared{
					TimeKey:    "time",
					TimeFormat: "%Y-%m-%d %H:%M:%S",
					Types: map[string]string{
						"tid": "integer",
					},
				},
			},
		},
	}.Components(tag, uid)

	c = append(c,
		fluentbit.TranslationComponents(tag, "level", "logging.googleapis.com/severity", false,
			[]struct{ SrcVal, DestVal string }{
				{"ERROR", "ERROR"},
				{"Error", "ERROR"},
				{"WARNING", "WARNING"},
				{"Warning", "WARNING"},
				{"SYSTEM", "INFO"},
				{"System", "INFO"},
				{"NOTE", "NOTICE"},
				{"Note", "NOTICE"},
			},
		)...,
	)
	return c
}

type LoggingProcessorMysqlGeneral struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorMysqlGeneral) Type() string {
	return "mysql_general"
}

func (p LoggingProcessorMysqlGeneral) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: []confgenerator.RegexParser{
				{
					// Limited documentation: https://dev.mysql.com/doc/refman/8.0/en/query-log.html
					// Sample line: 2021-10-12T01:12:37.732966Z        14 Connect   root@localhost on  using Socket
					// Sample line: 2021-10-12T01:12:37.733135Z        14 Query     select @@version_comment limit 1
					Regex: `^(?<time>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d+Z)\s+(?<tid>\d+)\s+(?<command>\w+)(\s+(?<message>[\s|\S]*))?`,
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

type LoggingProcessorMysqlSlow struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorMysqlSlow) Type() string {
	return "mysql_slow"
}

func (p LoggingProcessorMysqlSlow) Components(tag string, uid string) []fluentbit.Component {
	// Fields are split into this array to improve readability of the regex
	fields := strings.Join([]string{
		// Always present slow query log fields
		`\s+Query_time:\s+(?<queryTime>[\d\.]+)`,
		`\s+Lock_time:\s+(?<lockTime>[\d\.]+)`,
		`\s+Rows_sent:\s+(?<rowsSent>\d+)`,
		`\s+Rows_examined:\s(?<rowsExamined>\d+)`,

		// Extra fields present if log_slow_extra == ON
		`(?:\s+Thread_id:\s+\d+)?`, // Field also present in the 2nd line of the multiline log
		`(?:\s+Errno:\s(?<errorNumber>\d+))?`,
		`(?:\s+Killed:\s(?<killed>\d+))?`,
		`(?:\s+Bytes_received:\s(?<bytesReceived>\d+))?`,
		`(?:\s+Bytes_sent:\s(?<bytesSent>\d+))?`,
		`(?:\s+Read_first:\s(?<readFirst>\d+))?`,
		`(?:\s+Read_last:\s(?<readLast>\d+))?`,
		`(?:\s+Read_key:\s(?<readKey>\d+))?`,
		`(?:\s+Read_next:\s(?<readNext>\d+))?`,
		`(?:\s+Read_prev:\s(?<readPrev>\d+))?`,
		`(?:\s+Read_rnd:\s(?<readRnd>\d+))?`,
		`(?:\s+Read_rnd_next:\s(?<readRndNext>\d+))?`,
		`(?:\s+Sort_merge_passes:\s(?<sortMergePasses>\d+))?`,
		`(?:\s+Sort_range_count:\s(?<sortRangeCount>\d+))?`,
		`(?:\s+Sort_rows:\s(?<sortRows>\d+))?`,
		`(?:\s+Sort_scan_count:\s(?<sortScanCount>\d+))?`,
		`(?:\s+Created_tmp_disk_tables:\s(?<createdTmpDiskTables>\d+))?`,
		`(?:\s+Created_tmp_tables:\s(?<createdTmpTables>\d+))?`,
		`(?:\s+Start:\s(?<startTime>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d+Z))?`,
		`(?:\s+End:\s(?<endTime>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d+Z))?`,
	}, "")

	c := confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: []confgenerator.RegexParser{
				{
					// Fields documented: https://dev.mysql.com/doc/refman/8.0/en/slow-query-log.html
					// Sample line: # Time: 2021-10-12T01:13:38.132884Z
					//              # User@Host: root[root] @ localhost []  Id:    15
					//              # Query_time: 0.001855  Lock_time: 0.000000 Rows_sent: 0  Rows_examined: 0
					//              SET timestamp=1634001218;
					//              SET GLOBAL slow_query_log = 1;
					// Extra fields w/ low_slow_extra = 'ON'
					// Sample line: # Time: 2021-10-12T01:34:15.231930Z
					//              # User@Host: root[root] @ localhost []  Id:    21
					//              # Query_time: 0.012740  Lock_time: 0.000810 Rows_sent: 327  Rows_examined: 586 Thread_id: 21 Errno: 0 Killed: 0 Bytes_received: 0 Bytes_sent: 41603 Read_first: 2 Read_last: 0 Read_key: 361 Read_next: 361 Read_prev: 0 Read_rnd: 0 Read_rnd_next: 5 Sort_merge_passes: 0 Sort_range_count: 0 Sort_rows: 0 Sort_scan_count: 0 Created_tmp_disk_tables: 0 Created_tmp_tables: 0 Start: 2021-10-12T01:34:15.219190Z End: 2021-10-12T01:34:15.231930Z
					//              SET timestamp=1634002455;
					//              select * from information_schema.tables;
					Regex: fmt.Sprintf(`^# Time: (?<time>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d+Z)\s# User@Host:\s+(?<user>[^\[]*)\[(?<database>[^\]]*)\]\s+@\s+((?<host>[^\s]+)\s)?\[(?:(?<ipAddress>[\w\d\.:]+)?)\]\s+Id:\s+(?<tid>\d+)\s+#%s\s+(?<message>[\s\S]+)`, fields),
					Parser: confgenerator.ParserShared{
						TimeKey:    "time",
						TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%z",
						Types: map[string]string{
							"tid":                  "integer",
							"queryTime":            "float",
							"lockTime":             "float",
							"rowsSent":             "integer",
							"rowsExamined":         "integer",
							"errorNumber":          "integer",
							"killed":               "integer",
							"bytesReceived":        "integer",
							"bytesSent":            "integer",
							"readFirst":            "integer",
							"readLast":             "integer",
							"readKey":              "integer",
							"readNext":             "integer",
							"readPrev":             "integer",
							"readRnd":              "integer",
							"readRndNext":          "integer",
							"sortMergePasses":      "integer",
							"sortRangeCount":       "integer",
							"sortRows":             "integer",
							"sortScanCount":        "integer",
							"createdTmpDiskTables": "integer",
							"createdTmpTables":     "integer",
						},
					},
				},
			},
		},
		Rules: []confgenerator.MultilineRule{
			{
				StateName: "start_state",
				NextState: "cont",
				Regex:     `# Time: \d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d+Z`,
			},
			{
				StateName: "cont",
				NextState: "cont",
				Regex:     `^(?!# Time: \d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d+Z)`,
			},
		},
	}.Components(tag, uid)

	return c
}

type LoggingReceiverMysqlGeneral struct {
	LoggingProcessorMysqlGeneral            `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverMysqlGeneral) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			// Default log path for CentOS / RHEL / SLES / Debain / Ubuntu
			"/var/lib/mysql/${HOSTNAME}.log",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorMysqlGeneral.Components(tag, "mysql_general")...)
	return c
}

type LoggingReceiverMysqlSlow struct {
	LoggingProcessorMysqlSlow               `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverMysqlSlow) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			// Default log path for CentOS / RHEL / SLES / Debain / Ubuntu
			"/var/lib/mysql/${HOSTNAME}-slow.log",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorMysqlSlow.Components(tag, "mysql_slow")...)
	return c
}

type LoggingReceiverMysqlError struct {
	LoggingProcessorMysqlError              `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverMysqlError) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			// Default log path for CentOS / RHEL
			"/var/log/mysqld.log",
			// Default log path for SLES
			"/var/log/mysql/mysqld.log",
			// Default log path for Debian / Ubuntu
			"/var/log/mysql/error.log",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorMysqlError.Components(tag, "mysql_error")...)
	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorMysqlError{} })
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorMysqlGeneral{} })
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorMysqlSlow{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverMysqlError{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverMysqlGeneral{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverMysqlSlow{} })
}
