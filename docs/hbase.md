# `hbase_system` Logging Receiver

## Configuration

To configure a receiver for your hbase system logs, specify the following fields:

| Field                 | Default                           | Description |
| ---                   | ---                               | ---         |
| `type`                | required                          | Must be `hbase_system`. |
| `include_paths`       | `[/opt/hbase/logs/hbase-*-regionserver-*.log, /opt/hbase/logs/hbase-*-master-*.log]` | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `/var/log/hbase*/*.log`.
| `exclude_paths`       | `[]`                              | A list of filesystem path patterns to exclude from the set matched by `include_paths`.

Example Configuration:

```yaml
logging:
  receivers:
    hbase_system:
      type: hbase_system
  service:
    pipelines:
      hbase:
        receivers:
          - hbase_system
```

## Logs

System logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received |
| `jsonPayload.module` | string | Module of hbase where the log originated |
| `jsonPayload.source` | string | source of where the log originated |
| `jsonPayload.message` | string | Log message, including detailed stacktrace where provided |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |

Any fields that are blank or missing will not be present in the log entry.