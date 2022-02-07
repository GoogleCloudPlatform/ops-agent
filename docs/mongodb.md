# `mongodb` Logging Receiver

## Configuration

To configure a receiver for your mongodb logs, specify the following fields:

| Field                 | Default                       | Description |
| ---                   | ---                           | ---         |
| `type`                | required                      | Must be `redis`. |
| `include_paths`       | `[/var/log/mongodb/mongod.log*]` | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `/var/log/mongodb/*.log`.
| `exclude_paths`       | `[]`                          | A list of filesystem path patterns to exclude from the set matched by `include_paths`.
| `wildcard_refresh_interval` | `60s` | The interval at which wildcard file paths in include_paths are refreshed. Specified as a time interval parsable by [time.ParseDuration](https://pkg.go.dev/time#ParseDuration). Must be a multiple of 1s.|


Example Configuration:

```yaml
logging:
  receivers:
    mongodb:
      type: mongodb
  service:
    pipelines:
      mongodb:
        receivers: [mongodb]
```

## Logs

MongoDB logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.component` | string | Categorization of the log message. A full list can be found [here](https://docs.mongodb.com/manual/reference/log-messages/#std-label-log-message-components) |
| `jsonPayload.ctx` | string | The name of the thread issuing the log statement |
| `jsonPayload.id` | number | Log ID |
| `jsonPayload.message` | string | Log message |
| `jsonPayload.attributes` | object (optional) | Object containing one or more key-value pairs for any additional attributes provided |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged |
