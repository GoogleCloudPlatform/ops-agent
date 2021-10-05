# `cassandra_system`, `cassandra_debug` and `cassandra_gc` Logging Receivers

## Configuration

To configure a receiver for your cassandra system logs, specify the following fields:

| Field                 | Default                       | Description |
| ---                   | ---                           | ---         |
| `type`                | required                      | Must be `cassandra_system`. |
| `include_paths`       | `[/var/log/cassandra/system*.log]` | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `/var/log/apache*/*.log`.
| `exclude_paths`       | `[]`                          | A list of filesystem path patterns to exclude from the set matched by `include_paths`.

To configure a receiver for your cassandra debug logs, specify the following fields:

| Field                 | Default                      | Description |
| ---                   | ---                          | ---         |
| `type`                | required                     | Must be `cassandra_debug`. |
| `include_paths`       | `[/var/log/cassandra/debug*.log]` | The log files to read. |
| `exclude_paths`       | `[]`                         | Log files to exclude (if `include_paths` contains a glob or directory). |

To configure a receiver for your cassandra gc logs, specify the following fields:

| Field                 | Default                      | Description |
| ---                   | ---                          | ---         |
| `type`                | required                     | Must be `cassandra_gc`. |
| `include_paths`       | `[/var/log/cassandra/gc.log.*.current]` | The log files to read. |
| `exclude_paths`       | `[]`                         | Log files to exclude (if `include_paths` contains a glob or directory). |

Example Configuration:

```yaml
logging:
  receivers:
    cassandra_default_system:
      type: cassandra_system
    cassandra_default_debug:
      type: cassandra_debug
    cassandra_default_gc:
      type: cassandra_gc
  service:
    pipelines:
      apache:
        receivers:
          - cassandra_default_system
          - cassandra_default_debug
          - cassandra_default_gc
```

## Logs

System and Debug logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.level` | string | Log entry level |
| `jsonPayload.module` | string | Module of cassandra where the log originated |
| `jsonPayload.javaClass` | string | Java class where the log originated |
| `jsonPayload.lineNumber` | number | Line number of the source file where the log originated |
| `jsonPayload.message` | string | Log message, including detailed stacktrace where provided |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received |

Any fields that are blank or missing will not be present in the log entry.

GC logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.uptime` | number | Seconds the JVM has been active |
| `jsonPayload.timeStopped` | number | Seconds the JVM was stopped for garbage collection |
| `jsonPayload.timeStopping` | number | Seconds the JVM took to stop threads before garbage collection |
| `jsonPayload.message` | string | Log message |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged |
