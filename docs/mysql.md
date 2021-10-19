# `mysql_error`, `mysql_general` and `mysql_slow` Logging Receivers

## Configuration

To configure a receiver for your mysql error logs, specify the following fields:

| Field                 | Default                       | Description |
| ---                   | ---                           | ---         |
| `type`                | required                      | Must be `mysql_error`. |
| `include_paths`       | `[/var/log/mysqld.log, /var/log/mysql/mysqld.log, /var/log/mysql/error.log]` | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `/var/log/apache*/*.log`.
| `exclude_paths`       | `[]`                          | A list of filesystem path patterns to exclude from the set matched by `include_paths`.

To configure a receiver for your mysql general query logs, specify the following fields:

| Field                 | Default                      | Description |
| ---                   | ---                          | ---         |
| `type`                | required                     | Must be `mysql_general`. |
| `include_paths`       | `[/var/lib/mysql/${HOSTNAME}.log]` | The log files to read. |
| `exclude_paths`       | `[]`                         | Log files to exclude (if `include_paths` contains a glob or directory). |

To configure a receiver for your mysql slow query logs, specify the following fields:

| Field                 | Default                      | Description |
| ---                   | ---                          | ---         |
| `type`                | required                     | Must be `mysql_slow`. |
| `include_paths`       | `[/var/lib/mysql/${HOSTNAME}-slow.log` | The log files to read. |
| `exclude_paths`       | `[]`                         | Log files to exclude (if `include_paths` contains a glob or directory). |

Example Configuration:

```yaml
logging:
  receivers:
    mysql_error:
      type: mysql_error
    mysql_general:
      type: mysql_general
    mysql_slow:
      type: mysql_slow
  service:
    pipelines:
      mysql:
        receivers:
          - mysql_error
          - mysql_general
          - mysql_slow
```

## Logs

Error logs may contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry) depending on the version of MySQL:

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.level` | string | Log entry level |
| `jsonPayload.tid` | number | Thread ID where the log originated |
| `jsonPayload.errorCode` | string | MySQL error code associated with the log |
| `jsonPayload.subsystem` | string | MySQL subsystem where the log originated |
| `jsonPayload.message` | string | Log message |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received |

Any fields that are blank or missing will not be present in the log entry.

General Query logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.tid` | number | Thread ID where the log originated |
| `jsonPayload.command` | string | Type of database action being logged |
| `jsonPayload.message` | string | Log of the database action |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged |

Slow Query logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):
Documentation for the meaning of each field can be found [in the MySQL documentation](https://dev.mysql.com/doc/refman/8.0/en/slow-query-log.html)

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.user` | string | User that executed the query |
| `jsonPayload.database` | string | Database where the query was executed |
| `jsonPayload.host` | string | Host of the database instance |
| `jsonPayload.ipAddress` | string | Address of the database instance |
| `jsonPayload.tid` | number | Thread ID where the query was logged |
| `jsonPayload.queryTime` | number | The statement execution time in seconds |
| `jsonPayload.lockTime` | number | The time to acquire locks in seconds |
| `jsonPayload.rowsSent` | number | The number of rows sent to the client |
| `jsonPayload.rowsExamined` | number | The number of rows examined by the server layer |
| `jsonPayload.errorNumber`* | number | The statement error number, or 0 if no error occurred |
| `jsonPayload.killed`* | number | If the statement was terminated, the error number indicating why, or 0 if the statement terminated normally |
| `jsonPayload.bytesReceived`* | number | The number of bytes received from all clients |
| `jsonPayload.bytesSent`* | number | The number of bytes sent to all clients |
| `jsonPayload.readFirst`* | number | The number of times the first entry in an index was read |
| `jsonPayload.readLast`* | number | The number of requests to read the last key in an index |
| `jsonPayload.readKey`* | number | The number of requests to read a row based on a key |
| `jsonPayload.readNext`* | number | The number of requests to read the next row in key order |
| `jsonPayload.readPrev`* | number | The number of requests to read the previous row in key order |
| `jsonPayload.readRnd`* | number | The number of requests to read a row based on a fixed position |
| `jsonPayload.readRndNext`* | number | The number of requests to read the next row in the data file |
| `jsonPayload.sortMergePasses`* | number | The number of merge passes that the sort algorithm has had to do |
| `jsonPayload.sortRangeCount`* | number | The number of sorts that were done using ranges |
| `jsonPayload.sortRows`* | number | The number of sorted rows |
| `jsonPayload.sortScanCount`* | number | The number of sorts that were done by scanning the table |
| `jsonPayload.createdTmpDiskTables`* | number | The number of internal on-disk temporary tables created by the server |
| `jsonPayload.createdTmpTables`* | number | The number of internal temporary tables created by the server |
| `jsonPayload.startTime`* | string | The statement execution start time |
| `jsonPayload.endTime`* | string | The statement execution end time |
| `jsonPayload.message` | string | Full text of the query |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged |

* These fields are only provided if the `log_slow_extra` (available as of MySQL 8.0.14) system variable is set to `'ON'`