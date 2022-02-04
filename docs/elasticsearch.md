# Elasticsearch

Follow [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/elasticsearch)
for instructions to collect logs and metrics from this application using Ops Agent.

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
