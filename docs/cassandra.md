# `cassandra` Metrics Receiver

The `cassandra` metrics receiver can fetch stats from a Cassandra node's Java Virtual Machine (JVM) via [JMX](https://www.oracle.com/java/technologies/javase/javamanagement.html).

## Prerequisites

In order to expose a JMX endpoint, you must set the `com.sun.management.jmxremote.port` system property. It is recommended to also set the `com.sun.management.jmxremote.rmi.port` system property to the same port. To expose JMX endpoint remotely, you must also set the `java.rmi.server.hostname` system property. By default, these properties are set in a Cassandra deployment's cassandra-env.sh file and the default Cassandra installation requires no JMX authentication with JMX exposed locally on 127.0.0.1:7199.

## Configuration

| Field                 | Default            | Description |
| ---                   | ---                | ---         |
| `type`                | required           | Must be `cassandra`. |
| `endpoint`            | `localhost:7199`   | The [JMX Service URL](https://docs.oracle.com/javase/8/docs/api/javax/management/remote/JMXServiceURL.html) or host and port used to construct the Service URL. Must be in the form of `service:jmx:<protocol>:<sap>` or `host:port`. Values in `host:port` form will be used to create a Service URL of `service:jmx:rmi:///jndi/rmi://<host>:<port>/jmxrmi`. |
| `collect_jvm_metrics` | true               | Should the set of support [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics) also be collected |
| `username`            | not set by default | The configured username if JMX is configured to require authentication. |
| `password`            | not set by default | The configured password if JMX is configured to require authentication. |
| `collection_interval` | `60s`              | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |


Example Configuration:

```yaml
metrics:
  receivers:
    cassandra_metrics:
      type: cassandra
      endpoint: localhost:7199
      collection_interval: 30s
  service:
    pipelines:
      cassandra_pipeline:
        receivers:
          - cassandra_metrics
```

## Metrics
In addition to Cassandra specific metrics, by default Cassandra will also report [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics)

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
      cassandra:
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
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged |
