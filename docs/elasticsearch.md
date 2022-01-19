# `elasticsearch_json` and `elasticsearch_gc` Logging Receivers

## Configuration

To configure a receiver for your Elasticsearch JSON logs, specify the following fields:

| Field                 | Default                       | Description |
| ---                   | ---                           | ---         |
| `type`                | required                      | Must be `elasticsearch_json`. |
| `include_paths`       | `[/var/log/elasticsearch/*_server.json, /var/log/elasticsearch/*_deprecation.json, /var/log/elasticsearch/*_index_search_slowlog.json, /var/log/elasticsearch/*_index_indexing_slowlog.json, /var/log/elasticsearch/*_audit.json]` | The log files to read. |
| `exclude_paths`       | `[]`                          | Log files to exclude (if `include_paths` contains a glob or directory). |
| `wildcard_refresh_interval` | `1m0s` | The interval at which wildcard file paths in include_paths are refreshed. Specified as a time interval parsable by [time.ParseDuration](https://pkg.go.dev/time#ParseDuration). |
To configure a receiver for Elasticsearch GC logs, specify the following fields:

| Field                 | Default                      | Description |
| ---                   | ---                          | ---         |
| `type`                | required                     | Must be `elasticsearch_gc`. |
| `include_paths`       | `[/var/log/elasticsearch/gc.log]` | The log files to read. |
| `exclude_paths`       | `[]`                         | Log files to exclude (if `include_paths` contains a glob or directory). |
| `wildcard_refresh_interval` | `1m0s` | The interval at which wildcard file paths in include_paths are refreshed. Specified as a time interval parsable by [time.ParseDuration](https://pkg.go.dev/time#ParseDuration). |

Example Configuration:

```yaml
logging:
  receivers:
    elasticsearch_default_json:
      type: elasticsearch_json
    elasticsearch_default_gc:
      type: elasticsearch_gc
  service:
    pipelines:
      elasticsearch:
        receivers:
          - elasticsearch_default_json
          - elasticsearch_default_gc
```

## Logs

JSON logs commonly contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.component` | string | The component of Elasticsearch that emitted the log |
| `jsonPayload.type` | string | The type of log, indicating which log the record came from (e.g. `server` indicates this LogEntry came from the server log) |
| `jsonPayload.cluster.name` | string | The name of the cluster emitting the log record |
| `jsonPayload.cluster.uuid` | string | The uuid of the cluster emitting the log record |
| `jsonPayload.node.name` | string | The name of the node emitting the log record |
| `jsonPayload.node.uuid` | string | The uuid of the node emitting the log record |
| `jsonPayload.message` | string | Log message |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged |

If the fields are absent on the original log record, the will not be present on the resultant LogEntry.

GC logs may contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.gc_run` | number | The run of the garbage collector|
| `jsonPayload.message` | string | Log message |
| `jsonPayload.type` | string | The type of the log record  |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged |

If the fields are absent on the original log record, the will not be present on the resultant LogEntry.
