# Apache Flink

## Logs

Flink logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry) for Client, Jobmanager and Taskmanagers:

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.source` | string | Module and/or thread  where the log originated |
| `jsonPayload.logger` | string | Name of the logger where the log originated |
| `jsonPayload.message` | string | Log message, including detailed stacktrace where provided |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received |

Any fields that are blank or missing will not be present in the log entry.
