# Cassandra

Follow [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/cassandra)
for instructions to collect logs and metrics from this application using Ops Agent.

## Metrics

The following table provides the list of metrics that the Ops Agent collects from this application.

In addition to these Cassandra specific metrics, by default Cassandra will also report [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics)

| Metric                                                                          | Data Type | Unit        | Labels | Description |
| ---                                                                             | ---       | ---         | ---    | ---         | 
| workload.googleapis.com/cassandra.client.request.count                          | cumulative | 1          | operation | Number of requests by operation |
| workload.googleapis.com/cassandra.client.request.error.count                    | cumulative | 1          | operation, status | Number of request errors by operation |
| workload.googleapis.com/cassandra.client.request.read.latency.50p               | gauge     | µs          |        | Standard read request latency - 50th percentile |
| workload.googleapis.com/cassandra.client.request.read.latency.99p               | gauge     | µs          |        | Standard read request latency - 99th percentile |
| workload.googleapis.com/cassandra.client.request.read.latency.max               | gauge     | µs          |        | Maximum standard read request latency |
| workload.googleapis.com/cassandra.client.request.write.latency.50p              | gauge     | µs          |        | Standard write request latency - 50th percentile |
| workload.googleapis.com/cassandra.client.request.write.latency.99p              | gauge     | µs          |        | Standard write request latency - 99th percentile |
| workload.googleapis.com/cassandra.client.request.write.latency.max              | gauge     | µs          |        | Maximum standard write request latency |
| workload.googleapis.com/cassandra.client.request.range_slice.latency.50p        | gauge     | µs          |        | Token range read request latency - 50th percentile |
| workload.googleapis.com/cassandra.client.request.range_slice.latency.99p        | gauge     | µs          |        | Token range read request latency - 99th percentile |
| workload.googleapis.com/cassandra.client.request.range_slice.latency.max        | gauge     | µs          |        | Maximum token range read request latency |
| workload.googleapis.com/cassandra.compaction.tasks.completed                    | cumulative       | 1           |        | Number of completed compactions since server start |
| workload.googleapis.com/cassandra.compaction.tasks.pending                      | gauge     | 1           |        | Estimated number of compactions remaining to perform |
| workload.googleapis.com/cassandra.storage.load.count                            | gauge       | bytes       |        | Size of the on disk data size this node manages |
| workload.googleapis.com/cassandra.storage.total_hints.count                     | cumulative       | 1           |        | Number of hint messages written to this node since start |
| workload.googleapis.com/cassandra.storage.total_hints.in_progress.count         | gauge       | 1           |        | Number of hints attempting to be sent currently |

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
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged |
