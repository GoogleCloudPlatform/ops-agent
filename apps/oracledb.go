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
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

// MetricsReceiverOracleDB is the struct for ops agent monitoring metrics for oracledb
type MetricsReceiverOracleDB struct {
	confgenerator.ConfigComponent          `yaml:",inline"`
	confgenerator.MetricsReceiverShared    `yaml:",inline"`
	confgenerator.MetricsReceiverSharedTLS `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,hostname_port|startswith=/"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`

	SID         string `yaml:"sid" validate:"omitempty"`
	ServiceName string `yaml:"service_name" validate:"omitempty"`
	Wallet      string `yaml:"wallet" validate:"omitempty"`
}

const defaultOracleDBEndpoint = "localhost:1521"

// Type returns the configuration type key of the oracledb receiver
func (r MetricsReceiverOracleDB) Type() string {
	return "oracledb"
}

// Pipelines will construct the sql query receiver configuration
func (r MetricsReceiverOracleDB) Pipelines() []otel.Pipeline {
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

	// create a datasource in the form oracle://username:password@host:port/ServiceName?SID=sid&sslmode=enable&...
	datasource := fmt.Sprintf("oracle://%s:%s@%s/%s?%s",
		url.QueryEscape(r.Username),
		url.QueryEscape(r.Password),
		endpoint,
		url.QueryEscape(r.ServiceName),
		url.QueryEscape(params.Encode()),
	)

	config := map[string]interface{}{
		"collection_interval": r.CollectionIntervalString(),
		"driver":              "oracle",
		"datasource":          datasource,
		"queries":             r.queryConfig(),
	}
	return []otel.Pipeline{
		{
			Receiver: otel.Component{
				Type:   "sqlquery",
				Config: config,
			},
			Processors: []otel.Component{
				otel.NormalizeSums(),
				otel.MetricsTransform(
					otel.UpdateAllMetrics(
						// sql query receiver is not able to create these attributes
						// with lowercased names
						otel.RenameLabel("DATABASE_ID", "database_id"),
						otel.RenameLabel("GLOBAL_NAME", "global_name"),
						otel.RenameLabel("INSTANCE_ID", "instance_id"),
						otel.RenameLabel("TABLESPACE_NAME", "tablespace_name"),
						otel.RenameLabel("CONTENTS", "contents"),
						otel.RenameLabel("STATUS", "status"),
						otel.RenameLabel("PROGRAM", "program"),
						otel.RenameLabel("WAIT_CLASS", "wait_class"),
						otel.AddPrefix("workload.googleapis.com"),
					),
				),
			},
		},
	}
}

type oracleMetric struct {
	metric_name       string
	value_column      string
	unit              string
	description       string
	data_type         string
	monotonic         string
	value_type        string
	attribute_columns []string
	static_attributes map[string]string
}

type oracleQuery struct {
	query   string
	metrics []oracleMetric
}

func (r MetricsReceiverOracleDB) queryConfig() []map[string]interface{} {
	collectionIntervalSeconds := time.Second * 60
	// Ignore error as there is no meaningful error handling available
	// in this location in the code
	if parsedInterval, err := time.ParseDuration(r.CollectionIntervalString()); err == nil {
		collectionIntervalSeconds = parsedInterval
	}

	collectionIntervalString := fmt.Sprintf("%d", int(math.Round(collectionIntervalSeconds.Round(time.Second).Seconds())))

	cfg := []map[string]interface{}{}
	for _, q := range oracleQueries {
		metrics := []map[string]interface{}{}
		for _, m := range q.metrics {
			metric := map[string]interface{}{
				"metric_name":       m.metric_name,
				"value_column":      m.value_column,
				"unit":              m.unit,
				"description":       m.description,
				"data_type":         m.data_type,
				"value_type":        m.value_type,
				"attribute_columns": m.attribute_columns,
				"static_attributes": m.static_attributes,
			}
			if m.data_type == "sum" {
				metric["monotonic"] = m.monotonic
			}

			metrics = append(metrics, metric)
		}

		// Metrics being pulled from `GV_$$sysmetric` are limited to only metrics
		// collected in the last collection interval to prevent repeat metric data points
		sql := strings.ReplaceAll(q.query, "$$COLLECTION_INTERVAL", collectionIntervalString)

		query := map[string]interface{}{
			"sql":     sql,
			"metrics": metrics,
		}

		cfg = append(cfg, query)
	}

	return cfg
}

var oracleQueries = []oracleQuery{
	{
		query: `SELECT (SELECT DBID FROM GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, ts.TABLESPACE_NAME, ts.CONTENTS,
				(select sum(df.bytes) from sys.dba_data_files df where df.tablespace_name=ts.tablespace_name)-(select sum(fs.bytes) from sys.dba_free_space fs where fs.tablespace_name=ts.tablespace_name) AS USED_SPACE,
				(select sum(fs.bytes) from sys.dba_free_space fs where fs.tablespace_name=ts.tablespace_name) AS FREE_SPACE
			FROM sys.dba_tablespaces ts 
			WHERE ts.contents <> 'TEMPORARY'
			UNION ALL
			SELECT (SELECT DBID FROM GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, ts.NAME TABLESPACE_NAME, 'TEMPORARY' as CONTENTS,
					SUM(ss.USED_BLOCKS * t.BLOCK_SIZE) USED_SPACE, 
					SUM(t.BYTES) - SUM(ss.USED_BLOCKS * t.BLOCK_SIZE) FREE_SPACE
			FROM v_$$sort_segment ss
			JOIN v_$$tablespace ts
			ON ss.TABLESPACE_NAME = ts.NAME
			JOIN v_$$tempfile t
			ON t.TS# = ss.TS#
			GROUP BY ts.NAME`,
		metrics: []oracleMetric{
			{
				metric_name:       "oracle.tablespace.size",
				value_column:      "FREE_SPACE",
				unit:              "by",
				description:       "The size of tablespaces in the database.",
				data_type:         "sum",
				monotonic:         "false",
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "TABLESPACE_NAME", "CONTENTS"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"state":     "free",
				},
			},
			{
				metric_name:       "oracle.db.tablespace.size",
				value_column:      "USED_SPACE",
				unit:              "by",
				description:       "The size of tablespaces in the database.",
				data_type:         "sum",
				monotonic:         "false",
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
		query: "SELECT (SELECT DBID FROM GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, CONTENTS, STATUS, COUNT(*) COUNT FROM sys.dba_tablespaces GROUP BY STATUS, CONTENTS",
		metrics: []oracleMetric{
			{
				metric_name:       "oracle.tablespace.count",
				value_column:      "COUNT",
				unit:              "{tablespaces}",
				description:       "The number of tablespaces in the database.",
				data_type:         "sum",
				monotonic:         "false",
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
	// 	query: "SELECT (SELECT DBID FROM GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, INST_ID INSTANCE_ID, INSTANCE_ROLE, (sysdate - startup_time) * 86400 UPTIME FROM GV_$$instance",
	// 	metrics: []oracleMetric{
	// 		{
	// 			metric_name:       "oracle.uptime",
	// 			value_column:      "UPTIME",
	// 			unit:              "s",
	// 			description:       "The number of seconds the instance has been up.",
	// 			data_type:         "sum",
	// 			monotonic:         "true",
	// 			value_type:        "int",
	// 			attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID", "INSTANCE_ROLE"},
	// 			static_attributes: map[string]string{
	// 				"db.system": "oracle",
	// 			},
	// 		},
	// 	},
	// },
	{
		query: "SELECT (SELECT DBID FROM GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, (SELECT round(case when max(start_time) is null then -1 when sysdate - max(start_time) > 0 then (sysdate - max(start_time)) * 86400 else 0 end) FROM v_$$rman_backup_job_details ) LATEST_BACKUP FROM DUAL",
		metrics: []oracleMetric{
			{
				metric_name:       "oracle.backup.latest",
				value_column:      "LATEST_BACKUP",
				unit:              "s",
				description:       "The number of seconds since the last RMAN backup.",
				data_type:         "sum",
				monotonic:         "true",
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
			FROM (SELECT (SELECT DBID FROM GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, INST_ID, PROCESSES_UTIL, PROCESSES_LIMIT_VAL, SESSIONS_UTIL, SESSIONS_LIMIT_VAL 
			FROM (SELECT * FROM GV_$$resource_limit
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
		metrics: []oracleMetric{
			{
				metric_name:       "oracle.process.count",
				value_column:      "PROCESSES_UTIL",
				unit:              "{processes}",
				description:       "The current number of processes.",
				data_type:         "sum",
				monotonic:         "false",
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
				monotonic:         "false",
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
				monotonic:         "false",
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
				monotonic:         "false",
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
				},
			},
		},
	},
	{
		query: "SELECT (SELECT DBID FROM GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, INST_ID INSTANCE_ID, PROGRAM, SUM(PGA_USED_MEM) USED_MEM, SUM(PGA_ALLOC_MEM) - SUM(PGA_USED_MEM) FREE_MEM FROM GV_$$PROCESS WHERE PROGRAM <> 'PSEUDO' GROUP BY PROGRAM, INST_ID",
		metrics: []oracleMetric{
			{
				metric_name:       "oracle.process.pga_memory.size",
				value_column:      "USED_MEM",
				unit:              "by",
				description:       "The programmable global area memory allocated by process.",
				data_type:         "sum",
				monotonic:         "false",
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
				monotonic:         "false",
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
		query: "SELECT (SELECT DBID FROM GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, INST_ID INSTANCE_ID, WAIT_CLASS, SUM(total_waits_fg) AS TOTAL_WAITS_FG, SUM(total_waits)-SUM(total_waits_fg) AS TOTAL_WAITS_BG, SUM(total_timeouts_fg) AS TOTAL_TIMEOUTS_FG, SUM(total_timeouts)-SUM(TOTAL_TIMEOUTS_FG) AS TOTAL_TIMEOUTS_BG, SUM(time_waited_fg) AS TIME_WAITED_FG, SUM(time_waited)-SUM(TIME_WAITED_FG) AS TIME_WAITED_BG FROM GV_$$system_event WHERE wait_class <> 'Idle' GROUP BY INST_ID, WAIT_CLASS",
		metrics: []oracleMetric{
			{
				metric_name:       "oracle.wait.count",
				value_column:      "TOTAL_WAITS_FG",
				unit:              "{events}",
				description:       "The number of wait events experienced.",
				data_type:         "sum",
				monotonic:         "true",
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
				monotonic:         "true",
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
				monotonic:         "true",
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
				monotonic:         "true",
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
				monotonic:         "true",
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
				monotonic:         "true",
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
			FROM (SELECT (SELECT DBID FROM GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, INST_ID, END_TIME, RESPONSE_TIME, BUFFER_HIT_RATIO, ROW_HIT_RATIO 
			FROM (SELECT * FROM GV_$$sysmetric
				WHERE METRIC_NAME IN ('SQL Service Response Time', 'Buffer Cache Hit Ratio', 'Row Cache Hit Ratio')
				AND INTSIZE_CSEC = 6001
				AND (sysdate - END_TIME) * 86400 < $$COLLECTION_INTERVAL)
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
		metrics: []oracleMetric{
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
			FROM (SELECT (SELECT DBID FROM GV_$$DATABASE) DATABASE_ID, (SELECT GLOBAL_NAME FROM sys.GLOBAL_NAME) GLOBAL_NAME, INST_ID, CURSORS_CUMULATIVE, CURSORS_CURRENT, SORTS_MEM, SORTS_DISK, SORTS_ROWS, READ_TOTAL, WRITE_TOTAL, READ_TOTAL_BY, WRITE_TOTAL_BY, LOGONS_CURRENT, CLIENT_RECV_BY, DBLINK_RECV_BY, CLIENT_SENT_BY, DBLINK_SENT_BY, LOGONS_CUMULATIVE, USER_CALLS, USER_COMMITS, USER_ROLLBACKS 
			FROM (SELECT * FROM GV_$$sysstat
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
		metrics: []oracleMetric{
			{
				metric_name:       "oracle.cursor.count",
				value_column:      "CURSORS_CUMULATIVE",
				unit:              "{cursors}",
				description:       "The total number of cursors.",
				data_type:         "sum",
				monotonic:         "true",
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
				monotonic:         "false",
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
				monotonic:         "true",
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
				monotonic:         "false",
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
				monotonic:         "true",
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
				description:       "The total number of sorts. ",
				data_type:         "sum",
				monotonic:         "true",
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
				monotonic:         "true",
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
				monotonic:         "true",
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
				monotonic:         "true",
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
				monotonic:         "true",
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
				monotonic:         "true",
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"direction": "write",
				},
			},
			{
				metric_name:       "oracle.network.received.size",
				value_column:      "CLIENT_RECV_BY",
				unit:              "by",
				description:       "The total number of bytes received.",
				data_type:         "sum",
				monotonic:         "true",
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"target":    "client",
				},
			},
			{
				metric_name:       "oracle.network.sent.size",
				value_column:      "CLIENT_SENT_BY",
				unit:              "by",
				description:       "The total number of bytes sent.",
				data_type:         "sum",
				monotonic:         "true",
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"target":    "client",
				},
			},
			{
				metric_name:       "oracle.network.received.size",
				value_column:      "DBLINK_RECV_BY",
				unit:              "by",
				description:       "The total number of bytes received.",
				data_type:         "sum",
				monotonic:         "true",
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"target":    "dblink",
				},
			},
			{
				metric_name:       "oracle.network.sent.size",
				value_column:      "DBLINK_SENT_BY",
				unit:              "by",
				description:       "The total number of bytes sent.",
				data_type:         "sum",
				monotonic:         "true",
				value_type:        "int",
				attribute_columns: []string{"DATABASE_ID", "GLOBAL_NAME", "INSTANCE_ID"},
				static_attributes: map[string]string{
					"db.system": "oracle",
					"target":    "dblink",
				},
			},
			{
				metric_name:       "oracle.user.calls",
				value_column:      "USER_CALLS",
				unit:              "{calls}",
				description:       "The total number of user calls such as login, parse, fetch, or execute.",
				data_type:         "sum",
				monotonic:         "true",
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
				monotonic:         "true",
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
				monotonic:         "true",
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
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverOracleDB{} })
}

// LoggingReceiverOracleDB is a struct used for generating the fluentbit component for oracledb logs
type LoggingReceiverOracleDB struct {
	confgenerator.ConfigComponent           `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

// Type returns the string identifier for the general oracledb logs
func (lr LoggingReceiverOracleDB) Type() string {
	return "oracledb"
}

// Components returns the logging components of the oracledb logs
func (lr LoggingReceiverOracleDB) Components(tag string) []fluentbit.Component {
	if len(lr.IncludePaths) == 0 {
		lr.IncludePaths = []string{
			"/oracle/log/paths",
		}
	}
	components := lr.LoggingReceiverFilesMixin.Components(tag)
	components = append(components, confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: []confgenerator.RegexParser{
				{
					Regex: `^\[(?<type>[^:]*):(?<level>[^,]*),(?<timestamp>\d+-\d+-\d+T\d+:\d+:\d+.\d+Z),(?<node_name>[^:]*):([^:]+):(?<source>[^\]]+)\](?<message>.*)$`,
					Parser: confgenerator.ParserShared{
						TimeKey:    "timestamp",
						TimeFormat: "%Y-%m-%dT%H:%M:%S.%L",
					},
				},
			},
		},
		Rules: []confgenerator.MultilineRule{
			{
				StateName: "start_state",
				NextState: "cont",
				Regex:     `^\[([^\s+:]*):`,
			},
			{
				StateName: "cont",
				NextState: "cont",
				Regex:     `^(?!\[([^\s+:]*):).*$`,
			},
		},
	}.Components(tag, lr.Type())...)

	components = append(components,
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				"severity": {
					CopyFrom: "jsonPayload.level",
					MapValues: map[string]string{
						"debug": "DEBUG",
						"info":  "INFO",
						"warn":  "WARNING",
						"error": "ERROR",
					},
					MapValuesExclusive: true,
				},
				InstrumentationSourceLabel: instrumentationSourceValue(lr.Type()),
			},
		}.Components(tag, lr.Type())...)
	return components
}

func init() {
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverOracleDB{} })
}
