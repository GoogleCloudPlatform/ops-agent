# PostgreSQL

Follow [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/postgresql)
for instructions to collect logs and metrics from this application using Ops Agent.

## Metrics

The Ops Agent collects the following metrics from your postgresql instances.

| Metric                                                        | Data Type | Unit        | Labels                        | Description                                                                                                                     |
| ------------------------------------------------------------- | --------- | ----------- | ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| workload.googleapis.com/postgresql.backends                   | gauge     | 1           | database                      | The number of backends.                                                                                                         |
| workload.googleapis.com/postgresql.bgwriter.buffers.allocated | sum       | buffers     |                               | The Number of buffers allocated.                                                                                                |
| workload.googleapis.com/postgresql.bgwriter.buffers.writes    | sum       | buffers     | source                        | The number of buffers written.                                                                                                  |
| workload.googleapis.com/postgresql.bgwriter.checkpoint.count  | sum       | checkpoints |                               | The number of checkpoints performed.                                                                                            |
| workload.googleapis.com/postgresql.bgwriter.duration          | sum       | ms          | type                          | Total time spent writing and syncing files to disk by checkpoints.                                                              |
| workload.googleapis.com/postgresql.bgwriter.maxwritten        | sum       | checkpoints |                               | Number of times the background writer stopped a cleaning scan because it had written too many buffers.                          |
| workload.googleapis.com/postgresql.blocks_read                | sum       | 1           | database, table, source       | The number of blocks read.                                                                                                      |
| workload.googleapis.com/postgresql.connection.max             | gauge     | connections |                               | Configured maximum number of client connections allowed.                                                                        |
| workload.googleapis.com/postgresql.commits                    | sum       | 1           | database                      | The number of commits.                                                                                                          |
| workload.googleapis.com/postgresql.database.count             | sum       | 1           |                               | Number of user databases.                                                                                                       |
| workload.googleapis.com/postgresql.db_size                    | gauge     | By          | database                      | The database disk usage.                                                                                                        |
| workload.googleapis.com/postgresql.index.scans                | sum       | scans       | database, table, index        | The number of index scans on a table.                                                                                           |
| workload.googleapis.com/postgresql.index.size                 | sum       | By          | database, table, index        | The size of the index on disk.                                                                                                  |
| workload.googleapis.com/postgresql.operations                 | sum       | 1           | database, table, operation    | The number of db row operations.                                                                                                |
| workload.googleapis.com/postgresql.replication.data_delay     | gauge     | By          | replication_client            | The amount of data delayed in replication.                                                                                      |
| workload.googleapis.com/postgresql.rollbacks                  | sum       | 1           | database                      | The number of rollbacks.                                                                                                        |
| workload.googleapis.com/postgresql.rows                       | sum       | 1           | database, table, state        | The number of rows in the database.                                                                                             |
| workload.googleapis.com/postgresql.table.count                | sum       | tables      | database                      | The number of user tables in a database.                                                                                        |
| workload.googleapis.com/postgresql.table.size                 | sum       | By          | database, table               | Disk space used by a table.                                                                                                     |
| workload.googleapis.com/postgresql.table.vacuum.count         | sum       | 1           | database, table               | Number of times a table has manually been vacuumed.                                                                             |
| workload.googleapis.com/postgresql.wal.age                    | sum       | s           |                               | Age of the oldest WAL file.                                                                                                     |
| workload.googleapis.com/postgresql.wal.lag                    | sum       | s           | operation, replciation-client | Time between flushing recent WAL locally and receiving notification that the standby server has completed an operation with it. |

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
