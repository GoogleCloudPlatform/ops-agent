# PostgreSQL

Follow [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/postgresql)
for instructions to collect logs and metrics from this application using Ops Agent.

## Metrics

The Ops Agent collects the following metrics from your postgresql instances.

| Metric                                                 | Data Type | Unit        | Labels                          | Description    |
| ---                                                    | ---       | ---         | ---                             | ---            | 
| workload.googleapis.com/postgresql.backends            | gauge     | 1           | database                        | The number of backends. |
| workload.googleapis.com/postgresql.blocks_read         | sum       | 1           | database, table, source         | The number of blocks read. |
| workload.googleapis.com/postgresql.commits             | sum       | 1           | database                        | The number of commits.     |
| workload.googleapis.com/postgresql.db_size             | gauge     | By          | database                        | The database disk usage. |
| workload.googleapis.com/postgresql.operations          | sum       | 1           | database, table, operation      | The number of db row operations. |
| workload.googleapis.com/postgresql.rollbacks           | sum       | 1           | database                        | The number of rollbacks. |
| workload.googleapis.com/postgresql.rows                | sum       | 1           | database, table, state          | The number of rows in the database. |

## Logs

General Query logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.tid` | number | Thread ID where the log originated |
| `jsonPayload.role` | string | Authenticated role for the action being logged when relevant |
| `jsonPayload.user` | string | Authenticated user for the action being logged when relevant |
| `jsonPayload.level` | string | Log severity or type of database interaction type for some logs |
| `jsonPayload.message` | string | Log of the database action |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged |
