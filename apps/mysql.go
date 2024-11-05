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
	"fmt"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/internal/secret"
)

type MetricsReceiverMySql struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,hostname_port|startswith=/"`

	Password secret.String `yaml:"password" validate:"omitempty"`
	Username string        `yaml:"username" validate:"omitempty"`
}

const defaultMySqlUnixEndpoint = "/var/run/mysqld/mysqld.sock"

func (r MetricsReceiverMySql) Type() string {
	return "mysql"
}

func (r MetricsReceiverMySql) Pipelines(_ context.Context) ([]otel.ReceiverPipeline, error) {
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

	return []otel.ReceiverPipeline{
		{
			Receiver: otel.Component{
				Type: "mysql",
				Config: map[string]interface{}{
					"collection_interval": r.CollectionIntervalString(),
					"endpoint":            r.Endpoint,
					"username":            r.Username,
					"password":            r.Password.SecretValue(),
					"transport":           transport,
					"metrics": map[string]interface{}{
						"mysql.commands": map[string]interface{}{
							"enabled": true,
						},
						"mysql.index.io.wait.count": map[string]interface{}{
							"enabled": false,
						},
						"mysql.index.io.wait.time": map[string]interface{}{
							"enabled": false,
						},
						"mysql.mysqlx_connections": map[string]interface{}{
							"enabled": false,
						},
						"mysql.opened_resources": map[string]interface{}{
							"enabled": false,
						},
						"mysql.tmp_resources": map[string]interface{}{
							"enabled": false,
						},
						"mysql.prepared_statements": map[string]interface{}{
							"enabled": false,
						},
						"mysql.table.io.wait.count": map[string]interface{}{
							"enabled": false,
						},
						"mysql.table.io.wait.time": map[string]interface{}{
							"enabled": false,
						},
						"mysql.replica.sql_delay": map[string]interface{}{
							"enabled": true,
						},
						"mysql.replica.time_behind_source": map[string]interface{}{
							"enabled": true,
						},
					},
				},
			},
			Processors: map[string][]otel.Component{"metrics": {
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
				otel.ModifyInstrumentationScope(r.Type(), "1.0"),
			}},
		},
	}, nil
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverMySql{} })
}

const (
	// MySQL <5.7, MariaDB <10.1.4
	timeRegexOld  = `\d{6}\s+\d{1,2}:\d{2}:\d{2}`
	timeFormatOld = "%y%m%d %H:%M:%S"
	// MySQL >=5.7
	timeRegexMySQLNew  = `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d+(?:Z|[+-]\d{2}:?\d{2})?`
	timeFormatMySQLNew = "%Y-%m-%dT%H:%M:%S.%L%z"
	// MariaDB >=10.1.5 error log
	timeRegexMariaDBNew  = `\d{4}-\d{2}-\d{2}\s+\d{1,2}:\d{2}:\d{2}`
	timeFormatMariaDBNew = "%Y-%m-%d %H:%M:%S"
)

type LoggingProcessorMysqlError struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorMysqlError) Type() string {
	return "mysql_error"
}

func (p LoggingProcessorMysqlError) Components(ctx context.Context, tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseRegexComplex{
		Parsers: []confgenerator.RegexParser{
			{
				// MySql >=5.7 documented: https://dev.mysql.com/doc/refman/8.0/en/error-log-format.html
				// Sample Line: 2020-08-06T14:25:02.936146Z 0 [Warning] [MY-010068] [Server] CA certificate /var/mysql/sslinfo/cacert.pem is self signed.
				// Sample Line: 2020-08-06T14:25:03.109022Z 5 [Note] Event Scheduler: scheduler thread started with id 5
				Regex: fmt.Sprintf(
					`^(?<time>%s)\s+(?<tid>\d+)\s+\[(?<level>[^\]]+)](?:\s+\[(?<errorCode>[^\]]+)])?(?:\s+\[(?<subsystem>[^\]]+)])?\s+(?<message>.*)$`,
					timeRegexMySQLNew,
				),
				Parser: confgenerator.ParserShared{
					TimeKey:    "time",
					TimeFormat: timeFormatMySQLNew,
					Types: map[string]string{
						"tid": "integer",
					},
				},
			},
			{
				// Mysql <5.7, MariaDB <10.1.4, documented: https://mariadb.com/kb/en/error-log/#format
				// Sample Line: 160615 16:53:08 [Note] InnoDB: The InnoDB memory heap is disabled
				// TODO - time is in system time, not UTC, is there a way to resolve this in fluent bit?
				Regex: fmt.Sprintf(
					`^(?<time>%s)\s+\[(?<level>[^\]]+)]\s+(?<message>.*)$`,
					timeRegexOld,
				),
				Parser: confgenerator.ParserShared{
					TimeKey:    "time",
					TimeFormat: timeFormatOld,
				},
			},
			{
				// MariaDB >=10.1.5, documented: https://mariadb.com/kb/en/error-log/#format
				// Sample Line: 2016-06-15  1:53:33 139651251140544 [Note] InnoDB: The InnoDB memory heap is disabled
				Regex: fmt.Sprintf(
					`^(?<time>%s)(?:\s+(?<tid>\d+))?(?:\s+\[(?<level>[^\]]+)])?\s+(?<message>.*)$`,
					timeRegexMariaDBNew,
				),
				Parser: confgenerator.ParserShared{
					TimeKey:    "time",
					TimeFormat: timeFormatMariaDBNew,
					Types: map[string]string{
						"tid": "integer",
					},
				},
			},
		},
	}.Components(ctx, tag, uid)

	c = append(c,
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				"severity": {
					CopyFrom: "jsonPayload.level",
					MapValues: map[string]string{
						"ERROR":   "ERROR",
						"Error":   "ERROR",
						"WARNING": "WARNING",
						"Warning": "WARNING",
						"SYSTEM":  "INFO",
						"System":  "INFO",
						"NOTE":    "NOTICE",
						"Note":    "NOTICE",
					},
					MapValuesExclusive: true,
				},
				InstrumentationSourceLabel: instrumentationSourceValue(p.Type()),
			},
		}.Components(ctx, tag, uid)...,
	)

	return c
}

type LoggingProcessorMysqlGeneral struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorMysqlGeneral) Type() string {
	return "mysql_general"
}

func (p LoggingProcessorMysqlGeneral) Components(ctx context.Context, tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: []confgenerator.RegexParser{
				{
					// Limited documentation: https://dev.mysql.com/doc/refman/8.0/en/query-log.html
					// Sample line: 2021-10-12T01:12:37.732966Z        14 Connect   root@localhost on  using Socket
					// Sample line: 2021-10-12T01:12:37.733135Z        14 Query     select @@version_comment limit 1
					Regex: fmt.Sprintf(
						`^(?<time>%s)\s+(?<tid>\d+)\s+(?<command>\w+)(\s+(?<message>[\s|\S]*))?`,
						timeRegexMySQLNew,
					),
					Parser: confgenerator.ParserShared{
						TimeKey:    "time",
						TimeFormat: timeFormatMySQLNew,
						Types: map[string]string{
							"tid": "integer",
						},
					},
				},
				{
					// MariaDB uses the same timestamp format here as old versions do for the error log:
					// https://mariadb.com/kb/en/error-log/#format
					// Sample line: 230707  1:41:38     40 Query    select table_catalog, table_schema, table_name from information_schema.tables
					// Sample line:                      5 Connect  root@localhost on  using Socket
					// When a timestamp is present, it is followed by a single tab character.
					// When it is not, it means the timestamp is the same as a previous line, and it is replaced by another tab character.
					Regex: fmt.Sprintf(
						`^((?<time>%s)|\t)\s+(?<tid>\d+)\s+(?<command>\w+)(\s+(?<message>[\s|\S]*))?`,
						timeRegexOld,
					),
					Parser: confgenerator.ParserShared{
						TimeKey:    "time",
						TimeFormat: timeFormatOld,
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
				Regex: fmt.Sprintf(
					`^(%s|%s|\t\t)`,
					timeRegexMySQLNew,
					timeRegexOld,
				),
			},
			{
				StateName: "cont",
				NextState: "cont",
				Regex: fmt.Sprintf(
					`^(?!(%s|%s|\t\t))`,
					timeRegexMySQLNew,
					timeRegexOld,
				),
			},
		},
	}.Components(ctx, tag, uid)

	c = append(c,
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				InstrumentationSourceLabel: instrumentationSourceValue(p.Type()),
			},
		}.Components(ctx, tag, uid)...,
	)
	return c
}

type LoggingProcessorMysqlSlow struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorMysqlSlow) Type() string {
	return "mysql_slow"
}

func (p LoggingProcessorMysqlSlow) Components(ctx context.Context, tag string, uid string) []fluentbit.Component {
	modifyFields := map[string]*confgenerator.ModifyField{
		InstrumentationSourceLabel: instrumentationSourceValue(p.Type()),
	}

	// This format is for MySQL 8.0.14+
	// Fields are split into this array to improve readability of the regex
	mySQLFields := strings.Join([]string{
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
	parsers := []confgenerator.RegexParser{{
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
		Regex: fmt.Sprintf(
			`^(?:# Time: (?<time>%s)\s)?# User@Host:\s+(?<user>[^\[]*)\[(?<database>[^\]]*)\]\s+@\s+((?<host>[^\s]+)\s)?\[(?:(?<ipAddress>[\w\d\.:]+)?)\]\s+Id:\s+(?<tid>\d+)\s+#%s\s+(?<message>[\s\S]+)`,
			timeRegexMySQLNew,
			mySQLFields,
		),
		Parser: confgenerator.ParserShared{
			TimeKey:    "time",
			TimeFormat: timeFormatMySQLNew,
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
	}}

	// This format is for old MySQL and all MariaDB.
	// Docs:
	//   https://mariadb.com/kb/en/slow-query-log-extended-statistics/
	//   https://mariadb.com/kb/en/explain-in-the-slow-query-log/
	// Sample MariaDB line:
	// # User@Host: root[root] @ localhost []
	// # Thread_id: 32  Schema: dbt3sf1  QC_hit: No
	// # Query_time: 0.000130  Lock_time: 0.000068  Rows_sent: 0  Rows_examined: 0
	// # Rows_affected: 0  Bytes_sent: 1351
	// SET timestamp=1689286831;
	// SELECT OBJECT_SCHEMA, OBJECT_NAME, COUNT_DELETE, COUNT_FETCH, COUNT_INSERT, COUNT_UPDATE,SUM_TIMER_DELETE, SUM_TIMER_FETCH, SUM_TIMER_INSERT, SUM_TIMER_UPDATE FROM performance_schema.table_io_waits_summary_by_table WHERE OBJECT_SCHEMA NOT IN ('mysql', 'performance_schema');

	const (
		float   = `[\d\.]+`
		integer = `\d+`
		boolean = `Yes|No`
	)

	oldFields := [][]struct {
		identifier, jsonField, regex string
	}{
		{
			// "# Thread_id: %lu  Schema: %s  QC_hit: %s\n"
			{"Thread_id", "tid", integer},
			{"Schema", "database", `\S*`}, // N.B. MariaDB will still show the field with an empty string if the connection doesn't have an active database.
			{"QC_hit", "queryCacheHit", boolean},
		},
		{
			// "# Query_time: %s  Lock_time: %s  Rows_sent: %lu  Rows_examined: %lu\n"
			{"Query_time", "queryTime", float},
			{"Lock_time", "lockTime", float},
			{"Rows_sent", "rowsSent", integer},
			{"Rows_examined", "rowsExamined", integer},
		},
		{
			// MariaDB 10.3.1+
			// "# Rows_affected: %lu  Bytes_sent: %lu\n",
			{"Rows_affected", "rowsAffected", integer},
			{"Bytes_sent", "bytesSent", integer},
		},
		{
			// MariaDB 5.5.37+ if thd->tmp_tables_used with LOG_SLOW_VERBOSITY_QUERY_PLAN
			// "# Tmp_tables: %lu  Tmp_disk_tables: %lu  Tmp_table_sizes: %s\n"
			{"Tmp_tables", "createdTmpTables", integer},
			{"Tmp_disk_tables", "createdTmpDiskTables", integer},
			{"Tmp_table_sizes", "createdTmpTableSizes", integer},
		},
		{
			// MariaDB 10.3.4+ if thd->spcont != NULL
			// "# Stored_routine: %s\n"
			{"Stored_routine", "storedRoutine", `\S+`},
		},
		{
			// MariaDB 5.5.37+ with LOG_SLOW_VERBOSITY_QUERY_PLAN
			// "# Full_scan: %s  Full_join: %s  Tmp_table: %s  Tmp_table_on_disk: %s\n"
			{"Full_scan", "fullScan", boolean},
			{"Full_join", "fullJoin", boolean},
			{"Tmp_table", "", boolean},
			{"Tmp_table_on_disk", "", boolean},
		},
		{
			// MariaDB 5.5.37+ with LOG_SLOW_VERBOSITY_QUERY_PLAN
			// "# Filesort: %s  Filesort_on_disk: %s  Merge_passes: %lu  Priority_queue: %s\n",
			{"Filesort", "filesort", boolean},
			{"Filesort_on_disk", "filesortOnDisk", boolean},
			{"Merge_passes", "sortMergePasses", integer},
			{"Priority_queue", "priorityQueue", boolean},
		},
	}
	// LOG_SLOW_VERBOSITY_EXPLAIN causes additional comment lines
	// to be added containing the output of EXPLAIN; it's probably
	// not worth parsing them since they're somewhat freeform.
	oldLines := []string{
		fmt.Sprintf(`^(?:# Time: (?<time>%s)\s)?`, timeRegexOld),
		// N.B. MySQL logs two usernames (i.e. "root[root]"). The first username is the "priv_user", i.e. the username used for privilege checking.
		// The second username is the "user", which is the string the user provided when connecting.
		// We only report the priv_user here.
		// See https://dev.mysql.com/doc/refman/8.0/en/audit-log-file-formats.html
		`# User@Host:\s+(?<user>[^\[]*)\[[^\]]*\]\s+@\s+((?<host>[^\s]+)\s)?\[(?:(?<ipAddress>[\w\d\.:]+)?)\]`,
	}
	oldTypes := make(map[string]string)
	for _, lineFields := range oldFields {
		var out []string
		for _, field := range lineFields {
			valueRegex := fmt.Sprintf(`(?:%s)`, field.regex)
			if field.jsonField != "" {
				valueRegex = fmt.Sprintf(`(?<%s>%s)`, field.jsonField, field.regex)
				switch field.regex {
				case float:
					oldTypes[field.jsonField] = "float"
				case integer:
					oldTypes[field.jsonField] = "integer"
				case boolean:
					modifyFields[fmt.Sprintf(`jsonPayload.%s`, field.jsonField)] = &confgenerator.ModifyField{
						Type: "YesNoBoolean",
					}
				}
			}
			optional := "?"
			if len(out) == 0 {
				// First field on each line is not optional.
				// Otherwise we'll consume the "# " of the following line and prevent it from matching the next line's regex.
				optional = ""
			}
			out = append(out, fmt.Sprintf(
				`(?:\s+%s:\s%s)%s`,
				field.identifier,
				valueRegex,
				optional,
			))
		}
		oldLines = append(oldLines, fmt.Sprintf(
			`(?:\s+#%s)?`,
			strings.Join(out, ""),
		))
	}
	oldLines = append(oldLines, `\s+(?<message>[\s\S]+)`)

	parsers = append(parsers, confgenerator.RegexParser{
		Regex: strings.Join(oldLines, ""),
		Parser: confgenerator.ParserShared{
			TimeKey:    "time",
			TimeFormat: timeFormatOld,
			Types:      oldTypes,
		},
	})

	c := confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: parsers,
		},
		Rules: []confgenerator.MultilineRule{
			// Logs start with Time: or User@Host: (omitting time if it's the same as the previous entry).
			{
				StateName: "start_state",
				NextState: "comment",
				Regex: fmt.Sprintf(
					`^# (User@Host: |Time: (%s|%s))`,
					timeRegexMySQLNew,
					timeRegexOld,
				),
			},
			// Explicitly consume the next line, which might be User@Host.
			{
				StateName: "comment",
				NextState: "cont",
				Regex:     `^# `,
			},
			// Then consume everything until the next Time or User@Host.
			{
				StateName: "cont",
				NextState: "cont",
				Regex: fmt.Sprintf(
					`^(?!# (User@Host: |Time: (%s|%s)))`,
					timeRegexMySQLNew,
					timeRegexOld,
				),
			},
		},
	}.Components(ctx, tag, uid)

	c = append(c,
		confgenerator.LoggingProcessorModifyFields{
			Fields: modifyFields,
		}.Components(ctx, tag, uid)...,
	)
	return c
}

type LoggingReceiverMysqlGeneral struct {
	LoggingProcessorMysqlGeneral            `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverMysqlGeneral) Components(ctx context.Context, tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			// Default log path for CentOS / RHEL / SLES / Debain / Ubuntu
			"/var/lib/mysql/${HOSTNAME}.log",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(ctx, tag)
	c = append(c, r.LoggingProcessorMysqlGeneral.Components(ctx, tag, "mysql_general")...)
	return c
}

type LoggingReceiverMysqlSlow struct {
	LoggingProcessorMysqlSlow               `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverMysqlSlow) Components(ctx context.Context, tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			// Default log path for CentOS / RHEL / SLES / Debain / Ubuntu
			"/var/lib/mysql/${HOSTNAME}-slow.log",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(ctx, tag)
	c = append(c, r.LoggingProcessorMysqlSlow.Components(ctx, tag, "mysql_slow")...)
	return c
}

type LoggingReceiverMysqlError struct {
	LoggingProcessorMysqlError              `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverMysqlError) Components(ctx context.Context, tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			// Default log path for CentOS / RHEL
			"/var/log/mysqld.log",
			// Default log path for SLES
			"/var/log/mysql/mysqld.log",
			// Default log path for Oracle MySQL on Debian / Ubuntu
			"/var/log/mysql/error.log",
			// Default log path for MariaDB on Debian
			"/run/mysqld/mysqld.err",
			// Default log path for MariaDB upstream
			// https://mariadb.com/kb/en/error-log/#writing-the-error-log-to-a-file
			"/var/lib/mysql/${HOSTNAME}.err",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(ctx, tag)
	c = append(c, r.LoggingProcessorMysqlError.Components(ctx, tag, "mysql_error")...)
	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.LoggingProcessor { return &LoggingProcessorMysqlError{} })
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.LoggingProcessor { return &LoggingProcessorMysqlGeneral{} })
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.LoggingProcessor { return &LoggingProcessorMysqlSlow{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.LoggingReceiver { return &LoggingReceiverMysqlError{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.LoggingReceiver { return &LoggingReceiverMysqlGeneral{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.LoggingReceiver { return &LoggingReceiverMysqlSlow{} })
}
