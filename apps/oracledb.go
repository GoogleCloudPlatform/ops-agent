// Copyright 2022 Google LLC
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
	"net/url"
	"path"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/internal/secret"
)

type MetricsReceiverOracleDB struct {
	confgenerator.ConfigComponent       `yaml:",inline"`
	confgenerator.MetricsReceiverShared `yaml:",inline"`

	Insecure           *bool `yaml:"insecure" validate:"omitempty"`
	InsecureSkipVerify *bool `yaml:"insecure_skip_verify" validate:"omitempty"`

	Endpoint string        `yaml:"endpoint" validate:"omitempty,hostname_port|startswith=/"`
	Username string        `yaml:"username"`
	Password secret.String `yaml:"password"`

	SID         string `yaml:"sid" validate:"omitempty"`
	ServiceName string `yaml:"service_name" validate:"omitempty"`
	Wallet      string `yaml:"wallet" validate:"omitempty"`
}

const defaultOracleDBEndpoint = "localhost:1521"

func (r MetricsReceiverOracleDB) Type() string {
	return "oracledb"
}

func (r MetricsReceiverOracleDB) Pipelines(_ context.Context) ([]otel.ReceiverPipeline, error) {
	endpoint := r.Endpoint
	if r.Endpoint == "" {
		endpoint = defaultOracleDBEndpoint
	}

	// put all parameters that are provided to the datasource as query params
	// in an url.Values so they can be easily encoded
	params := url.Values{}
	if r.SID != "" {
		params.Add("SID", r.SID)
	}
	if r.Wallet != "" {
		params.Add("WALLET", r.Wallet)
	}
	if r.Insecure != nil && *r.Insecure == false {
		params.Add("SSL", "ENABLE")
	}
	if r.InsecureSkipVerify != nil && *r.InsecureSkipVerify == false {
		params.Add("SSL VERIFY", "ENABLE")
	}
	if strings.HasPrefix(r.Endpoint, "/") {
		params.Add("UNIX SOCKET", r.Endpoint)
		endpoint = defaultOracleDBEndpoint
	}

	auth := url.QueryEscape(r.Username)
	secretPassword := r.Password.SecretValue()
	if len(secretPassword) > 0 {
		auth = fmt.Sprintf("%s:%s", auth, url.QueryEscape(secretPassword))
	}

	// create a datasource in the form oracle://username:password@host:port/ServiceName?SID=sid&ssl=enable&...
	datasource := fmt.Sprintf("oracle://%s@%s/%s?%s",
		auth,
		endpoint,
		url.QueryEscape(r.ServiceName),
		params.Encode(),
	)

	config := map[string]interface{}{
		"collection_interval": r.CollectionIntervalString(),
		"driver":              "oracle",
		"datasource":          datasource,
		"queries":             sqlReceiverQueriesConfig(oracleQueries),
	}
	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type:   "sqlquery",
			Config: config,
		},
		Processors: map[string][]otel.Component{"metrics": {
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com",
					// sql query receiver is not able to create these attributes with lowercase names
					otel.RenameLabel("DATABASE_ID", "database_id"),
					otel.RenameLabel("GLOBAL_NAME", "global_name"),
					otel.RenameLabel("INSTANCE_ID", "instance_id"),
					otel.RenameLabel("TABLESPACE_NAME", "tablespace_name"),
					otel.RenameLabel("CONTENTS", "contents"),
					otel.RenameLabel("STATUS", "status"),
					otel.RenameLabel("PROGRAM", "program"),
					otel.RenameLabel("WAIT_CLASS", "wait_class"),
				),
			),
			otel.ModifyInstrumentationScope(r.Type(), "1.0"),
		}},
	}}, nil
}

var oracleQueries = []sqlReceiverQuery{
	{
		query: `SELECT (SELECT DBID FROM SYS.GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, ts.TABLESPACE_NAME, ts.CONTENTS,
				(select sum(df.bytes) from sys.dba_data_files df where df.tablespace_name=ts.tablespace_name)-(select sum(fs.bytes) from sys.dba_free_space fs where fs.tablespace_name=ts.tablespace_name) AS USED_SPACE,
				(select sum(fs.bytes) from sys.dba_free_space fs where fs.tablespace_name=ts.tablespace_name) AS FREE_SPACE
			FROM sys.dba_tablespaces ts 
			WHERE ts.contents <> 'TEMPORARY'
			UNION ALL
			SELECT (SELECT DBID FROM SYS.GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, ts.NAME TABLESPACE_NAME, 'TEMPORARY' as CONTENTS,
					SUM(ss.USED_BLOCKS * t.BLOCK_SIZE) USED_SPACE, 
					SUM(t.BYTES) - SUM(ss.USED_BLOCKS * t.BLOCK_SIZE) FREE_SPACE
			FROM SYS.V_$$sort_segment ss
			JOIN sys.v_$$tablespace ts
			ON ss.TABLESPACE_NAME = ts.NAME
			JOIN sys.v_$$tempfile t
			ON t.TS# = ss.TS#
			GROUP BY ts.NAME`,
		metrics: []sqlReceiverMetric{
			{
				metric_name:       "oracle.tablespace.size",
				value_column:      "FREE_SPACE",
				unit:              "by",
				description:       "The size of tablespaces in the database.",
				data_type:         "sum",
				monotonic:         false,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "TABLESPACE_NAME", "CONTENTS"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"state":     "free",
				},
			},
			{
				metric_name:       "oracle.tablespace.size",
				value_column:      "USED_SPACE",
				unit:              "by",
				description:       "The size of tablespaces in the database.",
				data_type:         "sum",
				monotonic:         false,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "TABLESPACE_NAME", "CONTENTS"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"state":     "used",
				},
			},
		},
	},
	{
		query: "SELECT (SELECT DBID FROM SYS.GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, CONTENTS, STATUS, COUNT(*) COUNT FROM sys.dba_tablespaces GROUP BY STATUS, CONTENTS",
		metrics: []sqlReceiverMetric{
			{
				metric_name:       "oracle.tablespace.count",
				value_column:      "COUNT",
				unit:              "{tablespaces}",
				description:       "The number of tablespaces in the database.",
				data_type:         "sum",
				monotonic:         false,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "STATUS", "CONTENTS"},
				static_attributes: map[string]string{
					"db.system": "oracle",
				},
			},
		},
	},
	// remove uptime until there is a consistent plan for support
	// {
	// 	query: "SELECT (SELECT DBID FROM SYS.GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, INST_ID INSTANCE_ID, INSTANCE_ROLE, (sysdate - startup_time) * 86400 UPTIME FROM SYS.GV_$$instance",
	// 	metrics: []sqlReceiverMetric{
	// 		{
	// 			metric_name:       "oracle.uptime",
	// 			value_column:      "UPTIME",
	// 			unit:              "s",
	// 			description:       "The number of seconds the instance has been up.",
	// 			data_type:         "sum",
	// 			monotonic:         true,
	// 			value_type:        "int",
	// 			attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID", "INSTANCE_ROLE"},
	// 			static_attributes: map[string]string{
	// 				"db.system": "oracle",
	// 			},
	// 		},
	// 	},
	// },
	{
		query: "SELECT (SELECT DBID FROM SYS.GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, (SELECT round(case when max(start_time) is null then -1 when sysdate - max(start_time) > 0 then (sysdate - max(start_time)) * 86400 else 0 end) FROM SYS.V_$$rman_backup_job_details ) LATEST_BACKUP FROM DUAL",
		metrics: []sqlReceiverMetric{
			{
				metric_name:       "oracle.backup.latest",
				value_column:      "LATEST_BACKUP",
				unit:              "s",
				description:       "The number of seconds since the last RMAN backup.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME"},
				static_attributes: map[string]string{
					"db.system": "oracle",
				},
			},
		},
	},
	{
		query: `SELECT DATABASE_ID, GLOBAL_NAME, INST_ID INSTANCE_ID, MAX(PROCESSES_UTIL) PROCESSES_UTIL, MAX(PROCESSES_LIMIT_VAL) PROCESSES_LIMIT_VAL, MAX(SESSIONS_UTIL) SESSIONS_UTIL, MAX(SESSIONS_LIMIT_VAL) SESSIONS_LIMIT_VAL
			FROM (SELECT (SELECT DBID FROM SYS.GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, INST_ID, PROCESSES_UTIL, PROCESSES_LIMIT_VAL, SESSIONS_UTIL, SESSIONS_LIMIT_VAL 
			FROM (SELECT * FROM SYS.GV_$$resource_limit
				WHERE RESOURCE_NAME IN ('processes', 'sessions'))
				PIVOT(
					MAX(TRIM(CURRENT_UTILIZATION)) UTIL,
					MAX(TRIM(LIMIT_VALUE)) LIMIT_VAL
					FOR RESOURCE_NAME
					IN (
						'processes' PROCESSES,
						'sessions' SESSIONS
					)
				)
			)
			GROUP BY DATABASE_ID, GLOBAL_NAME, INST_ID`,
		metrics: []sqlReceiverMetric{
			{
				metric_name:       "oracle.process.count",
				value_column:      "PROCESSES_UTIL",
				unit:              "{processes}",
				description:       "The current number of processes.",
				data_type:         "sum",
				monotonic:         false,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
				},
			},
			{
				metric_name:       "oracle.process.limit",
				value_column:      "PROCESSES_LIMIT_VAL",
				unit:              "{processes}",
				description:       "The maximum number of processes allowed.",
				data_type:         "sum",
				monotonic:         false,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
				},
			},
			{
				metric_name:       "oracle.session.count",
				value_column:      "SESSIONS_UTIL",
				unit:              "{sessions}",
				description:       "The current number of sessions.",
				data_type:         "sum",
				monotonic:         false,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
				},
			},
			{
				metric_name:       "oracle.session.limit",
				value_column:      "SESSIONS_LIMIT_VAL",
				unit:              "{sessions}",
				description:       "The maximum number of sessions allowed.",
				data_type:         "sum",
				monotonic:         false,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
				},
			},
		},
	},
	{
		query: "SELECT (SELECT DBID FROM SYS.GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, INST_ID INSTANCE_ID, PROGRAM, SUM(PGA_USED_MEM) USED_MEM, SUM(PGA_ALLOC_MEM) - SUM(PGA_USED_MEM) FREE_MEM FROM SYS.GV_$$PROCESS WHERE PROGRAM <> 'PSEUDO' GROUP BY PROGRAM, INST_ID",
		metrics: []sqlReceiverMetric{
			{
				metric_name:       "oracle.process.pga_memory.size",
				value_column:      "USED_MEM",
				unit:              "by",
				description:       "The programmable global area memory allocated by process.",
				data_type:         "sum",
				monotonic:         false,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID", "PROGRAM"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"state":     "used",
				},
			},
			{
				metric_name:       "oracle.process.pga_memory.size",
				value_column:      "FREE_MEM",
				unit:              "by",
				description:       "The programmable global area memory allocated by process.",
				data_type:         "sum",
				monotonic:         false,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID", "PROGRAM"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"state":     "free",
				},
			},
		},
	},
	{
		query: "SELECT (SELECT DBID FROM SYS.GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, INST_ID INSTANCE_ID, WAIT_CLASS, SUM(total_waits_fg) AS TOTAL_WAITS_FG, SUM(total_waits)-SUM(total_waits_fg) AS TOTAL_WAITS_BG, SUM(total_timeouts_fg) AS TOTAL_TIMEOUTS_FG, SUM(total_timeouts)-SUM(TOTAL_TIMEOUTS_FG) AS TOTAL_TIMEOUTS_BG, SUM(time_waited_fg) AS TIME_WAITED_FG, SUM(time_waited)-SUM(TIME_WAITED_FG) AS TIME_WAITED_BG FROM SYS.GV_$$system_event WHERE wait_class <> 'Idle' GROUP BY INST_ID, WAIT_CLASS",
		metrics: []sqlReceiverMetric{
			{
				metric_name:       "oracle.wait.count",
				value_column:      "TOTAL_WAITS_FG",
				unit:              "{events}",
				description:       "The number of wait events experienced.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID", "WAIT_CLASS"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"type":      "foreground",
				},
			},
			{
				metric_name:       "oracle.wait.count",
				value_column:      "TOTAL_WAITS_BG",
				unit:              "{events}",
				description:       "The number of wait events experienced.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID", "WAIT_CLASS"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"type":      "background",
				},
			},
			{
				metric_name:       "oracle.wait.time",
				value_column:      "TIME_WAITED_FG",
				unit:              "cs",
				description:       "The amount of time waited for wait events.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID", "WAIT_CLASS"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"type":      "foreground",
				},
			},
			{
				metric_name:       "oracle.wait.time",
				value_column:      "TIME_WAITED_BG",
				unit:              "cs",
				description:       "The amount of time waited for wait events.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID", "WAIT_CLASS"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"type":      "background",
				},
			},
			{
				metric_name:       "oracle.wait.timeouts",
				value_column:      "TOTAL_TIMEOUTS_FG",
				unit:              "{timeouts}",
				description:       "The number of timeouts for wait events.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID", "WAIT_CLASS"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"type":      "foreground",
				},
			},
			{
				metric_name:       "oracle.wait.timeouts",
				value_column:      "TOTAL_TIMEOUTS_BG",
				unit:              "{timeouts}",
				description:       "The number of timeouts for wait events.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID", "WAIT_CLASS"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"type":      "background",
				},
			},
		},
	},
	{
		query: `SELECT DATABASE_ID, GLOBAL_NAME, INST_ID INSTANCE_ID, MAX(RESPONSE_TIME) RESPONSE_TIME, MAX(BUFFER_HIT_RATIO) BUFFER_HIT_RATIO, MAX(ROW_HIT_RATIO) ROW_HIT_RATIO 
			FROM (SELECT (SELECT DBID FROM SYS.GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, INST_ID, END_TIME, RESPONSE_TIME, BUFFER_HIT_RATIO, ROW_HIT_RATIO 
			FROM (SELECT * FROM SYS.GV_$$sysmetric
				WHERE METRIC_NAME IN ('SQL Service Response Time', 'Buffer Cache Hit Ratio', 'Row Cache Hit Ratio')
				AND GROUP_ID = 2)
				PIVOT(
					MAX(VALUE)
					FOR METRIC_NAME
					IN (
						'SQL Service Response Time' RESPONSE_TIME,
						'Buffer Cache Hit Ratio' BUFFER_HIT_RATIO,
						'Row Cache Hit Ratio' ROW_HIT_RATIO
					)
				)
			)
			GROUP BY DATABASE_ID, GLOBAL_NAME, INST_ID`,
		metrics: []sqlReceiverMetric{
			{
				metric_name:       "oracle.service.response_time",
				value_column:      "RESPONSE_TIME",
				unit:              "cs",
				description:       "The average sql service response time.",
				data_type:         "gauge",
				value_type:        "double",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
				},
			},
			{
				metric_name:       "oracle.buffer.cache.ratio",
				value_column:      "BUFFER_HIT_RATIO",
				unit:              "%",
				description:       "Ratio of buffer cache hits to requests.",
				data_type:         "gauge",
				value_type:        "double",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
				},
			},
			{
				metric_name:       "oracle.row.cache.ratio",
				value_column:      "ROW_HIT_RATIO",
				unit:              "%",
				description:       "Ratio of row cache hits to requests.",
				data_type:         "gauge",
				value_type:        "double",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
				},
			},
		},
	},
	{
		query: `SELECT DATABASE_ID, GLOBAL_NAME, INST_ID INSTANCE_ID, MAX(CURSORS_CUMULATIVE) CURSORS_CUMULATIVE, MAX(CURSORS_CURRENT) CURSORS_CURRENT, MAX(SORTS_MEM) SORTS_MEM, MAX(SORTS_DISK) SORTS_DISK, MAX(SORTS_ROWS) SORTS_ROWS, MAX(READ_TOTAL) READ_TOTAL, MAX(WRITE_TOTAL) WRITE_TOTAL, MAX(READ_TOTAL_BY) READ_TOTAL_BY, MAX(WRITE_TOTAL_BY) WRITE_TOTAL_BY, MAX(LOGONS_CURRENT) LOGONS_CURRENT, MAX(CLIENT_RECV_BY) CLIENT_RECV_BY, MAX(DBLINK_RECV_BY) DBLINK_RECV_BY, MAX(CLIENT_SENT_BY) CLIENT_SENT_BY, MAX(DBLINK_SENT_BY) DBLINK_SENT_BY, MAX(LOGONS_CUMULATIVE) LOGONS_CUMULATIVE, MAX(USER_CALLS) USER_CALLS, MAX(USER_COMMITS) USER_COMMITS, MAX(USER_ROLLBACKS) USER_ROLLBACKS 
			FROM (SELECT (SELECT DBID FROM SYS.GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, INST_ID, CURSORS_CUMULATIVE, CURSORS_CURRENT, SORTS_MEM, SORTS_DISK, SORTS_ROWS, READ_TOTAL, WRITE_TOTAL, READ_TOTAL_BY, WRITE_TOTAL_BY, LOGONS_CURRENT, CLIENT_RECV_BY, DBLINK_RECV_BY, CLIENT_SENT_BY, DBLINK_SENT_BY, LOGONS_CUMULATIVE, USER_CALLS, USER_COMMITS, USER_ROLLBACKS 
			FROM (SELECT * FROM SYS.GV_$$sysstat
				WHERE NAME IN ('opened cursors cumulative', 'opened cursors current', 'sorts (memory)', 'sorts (disk)', 'sorts (rows)', 'physical read total IO requests', 'physical write total IO requests', 'physical read total bytes', 'physical write total bytes', 'logons current', 'bytes received via SQL*Net from client', 'bytes received via SQL*Net from dblink', 'bytes sent via SQL*Net to client', 'bytes sent via SQL*Net to dblink', 'logons cumulative', 'user calls', 'user commits', 'user rollbacks')
				)
				PIVOT(
					MAX(VALUE)
					FOR NAME
					IN (
						'opened cursors cumulative' CURSORS_CUMULATIVE,
						'opened cursors current' CURSORS_CURRENT,
						'logons cumulative' LOGONS_CUMULATIVE,
						'logons current' LOGONS_CURRENT,
						'sorts (memory)' SORTS_MEM,
						'sorts (disk)' SORTS_DISK,
						'sorts (rows)' SORTS_ROWS,
						'physical read total IO requests' READ_TOTAL,
						'physical write total IO requests' WRITE_TOTAL,
						'physical read total bytes' READ_TOTAL_BY,
						'physical write total bytes' WRITE_TOTAL_BY,
						'bytes received via SQL*Net from client' CLIENT_RECV_BY,
						'bytes received via SQL*Net from dblink' DBLINK_RECV_BY,
						'bytes sent via SQL*Net to client' CLIENT_SENT_BY,
						'bytes sent via SQL*Net to dblink' DBLINK_SENT_BY,
						'user calls' USER_CALLS,
						'user commits' USER_COMMITS,
						'user rollbacks' USER_ROLLBACKS
					)
				)
			)
			GROUP BY DATABASE_ID, GLOBAL_NAME, INST_ID`,
		metrics: []sqlReceiverMetric{
			{
				metric_name:       "oracle.cursor.count",
				value_column:      "CURSORS_CUMULATIVE",
				unit:              "{cursors}",
				description:       "The total number of cursors.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
				},
			},
			{
				metric_name:       "oracle.cursor.current",
				value_column:      "CURSORS_CURRENT",
				unit:              "{cursors}",
				description:       "The current number of cursors.",
				data_type:         "sum",
				monotonic:         false,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
				},
			},
			{
				metric_name:       "oracle.logon.count",
				value_column:      "LOGONS_CUMULATIVE",
				unit:              "{logons}",
				description:       "The total number of logons.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
				},
			},
			{
				metric_name:       "oracle.logon.current",
				value_column:      "LOGONS_CURRENT",
				unit:              "{logons}",
				description:       "The current number of logons.",
				data_type:         "sum",
				monotonic:         false,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
				},
			},
			{
				metric_name:       "oracle.sort.count",
				value_column:      "SORTS_MEM",
				unit:              "{sorts}",
				description:       "The total number of sorts.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"type":      "memory",
				},
			},
			{
				metric_name:       "oracle.sort.count",
				value_column:      "SORTS_DISK",
				unit:              "{sorts}",
				description:       "The total number of sorts.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"type":      "disk",
				},
			},
			{
				metric_name:       "oracle.sort.row.count",
				value_column:      "SORTS_ROWS",
				unit:              "{rows}",
				description:       "The total number of rows sorted.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
				},
			},
			{
				metric_name:       "oracle.disk.operation.count",
				value_column:      "READ_TOTAL",
				unit:              "{operations}",
				description:       "The number of physical disk operations.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"direction": "read",
				},
			},
			{
				metric_name:       "oracle.disk.operation.size",
				value_column:      "READ_TOTAL_BY",
				unit:              "by",
				description:       "The number of bytes affected by physical disk operations.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"direction": "read",
				},
			},
			{
				metric_name:       "oracle.disk.operation.count",
				value_column:      "WRITE_TOTAL",
				unit:              "{operations}",
				description:       "The number of physical disk operations.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"direction": "write",
				},
			},
			{
				metric_name:       "oracle.disk.operation.size",
				value_column:      "WRITE_TOTAL_BY",
				unit:              "by",
				description:       "The number of bytes affected by physical disk operations.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"direction": "write",
				},
			},
			{
				metric_name:       "oracle.network.data",
				value_column:      "CLIENT_RECV_BY",
				unit:              "by",
				description:       "The total number of bytes communicated on the network.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"target":    "client",
					"direction": "received",
				},
			},
			{
				metric_name:       "oracle.network.data",
				value_column:      "CLIENT_SENT_BY",
				unit:              "by",
				description:       "The total number of bytes communicated on the network.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"target":    "client",
					"direction": "sent",
				},
			},
			{
				metric_name:       "oracle.network.data",
				value_column:      "DBLINK_RECV_BY",
				unit:              "by",
				description:       "The total number of bytes communicated on the network.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"target":    "dblink",
					"direction": "received",
				},
			},
			{
				metric_name:       "oracle.network.data",
				value_column:      "DBLINK_SENT_BY",
				unit:              "by",
				description:       "The total number of bytes communicated on the network.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"target":    "dblink",
					"direction": "sent",
				},
			},
			{
				metric_name:       "oracle.user.calls",
				value_column:      "USER_CALLS",
				unit:              "{calls}",
				description:       "The total number of user calls such as login, parse, fetch, or execute.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
				},
			},
			{
				metric_name:       "oracle.user.commits",
				value_column:      "USER_COMMITS",
				unit:              "{commits}",
				description:       "The total number of user transaction commits.",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
				},
			},
			{
				metric_name:       "oracle.user.rollbacks",
				value_column:      "USER_ROLLBACKS",
				unit:              "{rollbacks}",
				description:       "The total number of times users manually issue the ROLLBACK statement or an error occurs during a user's transactions",
				data_type:         "sum",
				monotonic:         true,
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
				},
			},
		},
	},
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverOracleDB{} })
}

type LoggingProcessorOracleDBAlert struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (lr LoggingProcessorOracleDBAlert) Type() string {
	return "oracledb_alert"
}

func (lr LoggingProcessorOracleDBAlert) Components(ctx context.Context, tag string, uid string) []fluentbit.Component {
	components := confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: []confgenerator.RegexParser{
				{
					// Sample log:  2021-12-21T10:19:47.339827-05:00
					//				Thread 1 opened at log sequence 1
					//				Current log# 1 seq# 1 mem# 0: /u01/oracle/oradata/DB19C/redo01.log
					//				Successful open of redo thread 1
					Regex: `^(?<timestamp>\d+-\d+-\d+T\d+:\d+:\d+.\d+(?:[-+]\d+:\d+|Z))\n(?<message>[\s\S]+)`,
					Parser: confgenerator.ParserShared{
						TimeKey:    "timestamp",
						TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%z",
					},
				},
			},
		},
		Rules: []confgenerator.MultilineRule{
			{
				StateName: "start_state",
				NextState: "cont",
				Regex:     `^\d+-\d+-\d+T\d+:\d+:\d+.\d+(?:[-+]\d+:\d+|Z)`,
			},
			{
				StateName: "cont",
				NextState: "cont",
				Regex:     `^(?!\d+-\d+-\d+T\d+:\d+:\d+.\d+(?:[-+]\d+:\d+|Z)).*$`,
			},
		},
	}.Components(ctx, tag, uid)

	severityVal := "ALERT"
	components = append(components,
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				"severity":                 {StaticValue: &severityVal},
				InstrumentationSourceLabel: instrumentationSourceValue(lr.Type()),
			},
		}.Components(ctx, tag, uid)...)
	return components
}

type LoggingReceiverOracleDBAlert struct {
	LoggingProcessorOracleDBAlert           `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
	OracleHome                              string   `yaml:"oracle_home,omitempty" validate:"required_without=IncludePaths,excluded_with=IncludePaths"`
	IncludePaths                            []string `yaml:"include_paths,omitempty" validate:"required_without=OracleHome,excluded_with=OracleHome"`
}

func (lr LoggingReceiverOracleDBAlert) Components(ctx context.Context, tag string) []fluentbit.Component {
	if len(lr.OracleHome) > 0 {
		lr.IncludePaths = []string{
			path.Join(lr.OracleHome, "/diag/rdbms/*/*/trace/alert_*.log"),
		}
	}

	lr.LoggingReceiverFilesMixin.IncludePaths = lr.IncludePaths

	c := lr.LoggingReceiverFilesMixin.Components(ctx, tag)
	c = append(c, lr.LoggingProcessorOracleDBAlert.Components(ctx, tag, lr.Type())...)
	return c
}

type LoggingProcessorOracleDBAudit struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (lr LoggingProcessorOracleDBAudit) Type() string {
	return "oracledb_audit"
}

func (lr LoggingProcessorOracleDBAudit) Components(ctx context.Context, tag string, uid string) []fluentbit.Component {
	components := confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: []confgenerator.RegexParser{
				{
					// Sample log:  Wed Sep 14 16:18:03 2022 +00:00
					//				LENGTH : '623'
					//				ACTION :[373] 'select distinct 'ALTER SYSTEM KILL SESSION ''' || stat.sid || ',' ||
					//								sess.serial# ||
					//								decode(substr(inst.version, 1, 4),
					//										'12.1', ''' immediate ', ''' force timeout 0 ') ||
					//								'-- process 73841'
					//				FROM SYS.V$mystat stat, v$session sess, v$instance inst
					//				where stat.sid=sess.sid
					//				union all
					//				select '/' from dual'
					//				DATABASE USER:[1] '/'
					//				PRIVILEGE :[6] 'SYSDBA'
					//				CLIENT USER:[6] 'oracle'
					//				CLIENT TERMINAL:[5] 'pts/1'
					//				STATUS:[1] '0'
					//				DBID:[10] '1643176521'
					//				SESSIONID:[10] '4294967295'
					//				USERHOST:[7] 'oradb19'
					//				CLIENT ADDRESS:[0] ''
					//				ACTION NUMBER:[1] '3'
					Regex: `^(?<timestamp>\w+\s+\w+\s+\d+\s+\d+:\d+:\d+\s+\d+\s+(?:[-+]\d+:\d+|Z))\n` +
						`LENGTH\s*:(?:\[\d*\])?\s*'(?<length>.*)'\n` +
						`ACTION\s*:(?:\[\d*\])?\s*'(?<action>[\s\S]*)'\n` +
						`DATABASE USER\s*:(?:\[\d*\])?\s*'(?<database_user>.*)'\n` +
						`PRIVILEGE\s*:(?:\[\d*\])?\s*'(?<privilege>.*)'\n` +
						`CLIENT USER\s*:(?:\[\d*\])?\s*'(?<client_user>.*)'\n` +
						`CLIENT TERMINAL\s*:(?:\[\d*\])?\s*'(?<client_terminal>.*)'\n` +
						`STATUS\s*:(?:\[\d*\])?\s*'(?<status>.*)'\n` +
						`DBID\s*:(?:\[\d*\])?\s*'(?<dbid>.*)'\n` +
						`SESSIONID\s*:(?:\[\d*\])?\s*'(?<sessionid>.*)'\n` +
						`USERHOST\s*:(?:\[\d*\])?\s*'(?<user_host>.*)'\n` +
						`CLIENT ADDRESS\s*:(?:\[\d*\])?\s*'(?<client_address>.*)'\n` +
						`ACTION NUMBER\s*:(?:\[\d*\])?\s*'(?<action_number>.*)'\n?`,
					Parser: confgenerator.ParserShared{
						TimeKey:    "timestamp",
						TimeFormat: "%a %b %d %H:%M:%S %Y %z",
						Types: map[string]string{
							"length":        "int",
							"action_number": "int",
							"dbid":          "int",
							"sessionid":     "int",
							"status":        "int",
						},
					},
				},
			},
		},
		Rules: []confgenerator.MultilineRule{
			{
				StateName: "start_state",
				NextState: "cont",
				Regex:     `^\w+ \w+ {1,2}\d+ {1,2}\d+:\d+:\d+ \d+ (?:[-+]\d+:\d+|Z)`,
			},
			{
				StateName: "cont",
				NextState: "cont",
				Regex:     `^(?!\w+ \w+ {1,2}\d+ {1,2}\d+:\d+:\d+ \d+ (?:[-+]\d+:\d+|Z)).*$`,
			},
		},
	}.Components(ctx, tag, uid)

	severityVal := "INFO"

	components = append(components,
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				"severity":                 {StaticValue: &severityVal},
				InstrumentationSourceLabel: instrumentationSourceValue(lr.Type()),
			},
		}.Components(ctx, tag, uid)...)
	return components
}

type LoggingReceiverOracleDBAudit struct {
	LoggingProcessorOracleDBAudit           `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
	OracleHome                              string   `yaml:"oracle_home,omitempty" validate:"required_without=IncludePaths,excluded_with=IncludePaths"`
	IncludePaths                            []string `yaml:"include_paths,omitempty" validate:"required_without=OracleHome,excluded_with=OracleHome"`
}

func (lr LoggingReceiverOracleDBAudit) Components(ctx context.Context, tag string) []fluentbit.Component {
	if len(lr.OracleHome) > 0 {
		lr.IncludePaths = []string{
			path.Join(lr.OracleHome, "/admin/*/adump/*.aud"),
		}
	}

	lr.LoggingReceiverFilesMixin.IncludePaths = lr.IncludePaths

	c := lr.LoggingReceiverFilesMixin.Components(ctx, tag)
	c = append(c, lr.LoggingProcessorOracleDBAudit.Components(ctx, tag, lr.Type())...)
	return c
}

func init() {
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.LoggingReceiver { return &LoggingReceiverOracleDBAlert{} })
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.LoggingProcessor { return &LoggingProcessorOracleDBAlert{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.LoggingReceiver { return &LoggingReceiverOracleDBAudit{} })
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.LoggingProcessor { return &LoggingProcessorOracleDBAudit{} })
}
