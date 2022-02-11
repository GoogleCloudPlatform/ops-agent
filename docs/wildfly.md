# `wildfly_server` Logging Receiver

## Configuration

To configure a receiver for your wildfly server logs, specify the following fields:

| Field                 | Default                           | Description |
| ---                   | ---                               | ---         |
| `type`                | required                          | Must be `wildfly_server`. |
| `include_paths`       | `[/opt/wildfly/standalone/log/server.log, /opt/wildfly/domain/servers/*/log/server.log]` | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `/var/log/wildfly*/*.log`.
| `exclude_paths`       | `[]`                              | A list of filesystem path patterns to exclude from the set matched by `include_paths`.
| `wildcard_refresh_interval` | `60s` | The interval at which wildcard file paths in include_paths are refreshed. Given as a time duration, for example 30s, 2m. This property might be useful under high logging throughputs where log files are rotated faster than the default interval. Must be a multiple of 1s.|

Example Configuration:

```yaml
logging:
  receivers:
    wildfly_server:
      type: wildfly_server
  service:
    pipelines:
      wildfly_server:
        receivers:
          - wildfly_server
```

## Logs

Server logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received |
| `jsonPayload.thread` | string | Thread where the log originated |
| `jsonPayload.source` | string | Source where the log originated |
| `jsonPayload.messageCode` | string | Wildfly specific message code preceding the log, where applicable |
| `jsonPayload.message` | string | Log message |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |

Any fields that are blank or missing will not be present in the log entry.
