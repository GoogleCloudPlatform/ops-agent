# `redis` Logging Receiver

## Configuration

To configure a receiver for your redis logs, specify the following fields:

| Field                 | Default                       | Description |
| ---                   | ---                           | ---         |
| `type`                | required                      | Must be `redis`. |
| `include_paths`       | `[/var/log/redis/redis-server.log, /var/log/redis_6379.log, /var/log/redis/redis.log, /var/log/redis/default.log, /var/log/redis/redis_6379.log]` | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `/var/log/redis/*.log`.
| `exclude_paths`       | `[]`                          | A list of filesystem path patterns to exclude from the set matched by `include_paths`.


Example Configuration:

```yaml
logging:
  receivers:
    redis_default:
      type: redis
  service:
    pipelines:
      apache:
        receivers:
          - redis
```

## Logs

Redis logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.roleChar` | string | redis role character (X, C, S, M) |
| `jsonPayload.role` | string | translated from redis role character (sentinel, RDB/AOF_writing_child, slave, master) |
| `jsonPayload.level` | string | Log entry level |
| `jsonPayload.pid` | number | Process ID |
| `jsonPayload.message` | string | Log message |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged |
