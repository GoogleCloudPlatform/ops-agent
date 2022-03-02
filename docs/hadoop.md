# Hadoop

# `hadoop` Metrics Receiver

The `hadoop` metrics receiver can fetch stats from a Hadoop server's Java Virtual Machine (JVM) via [JMX](https://www.oracle.com/java/technologies/javase/javamanagement.html).
It collects metrics specific to the local region server, as well as metrics presented by the Master node if the node being monitored is indeed the Master. For High Availability configurations, it is recommended for every master node to report cluster metrics, which will have identical values, to avoid single point of failures when one master goes down.
## Prerequisites

In order to expose a JMX endpoint, you must set the `com.sun.management.jmxremote.port` system property. It is recommended to also set the `com.sun.management.jmxremote.rmi.port` system property to the same port. To expose JMX endpoint remotely, you must also set the `java.rmi.server.hostname` system property. By default, these properties are set in a Hadoop deployment's hadoop-env.sh file and the default Hadoop installation requires no JMX authentication with JMX exposed locally on 127.0.0.1:8050.

## Configuration

| Field                 | Default            | Description |
| ---                   | ---                | ---         |
| `type`                | required           | Must be `hadoop`. |
| `endpoint`            | `localhost:8004`   | The [JMX Service URL](https://docs.oracle.com/javase/8/docs/api/javax/management/remote/JMXServiceURL.html) or host and port used to construct the Service URL. Must be in the form of `service:jmx:<protocol>:<sap>` or `host:port`. Values in `host:port` form will be used to create a Service URL of `service:jmx:rmi:///jndi/rmi://<host>:<port>/jmxrmi`. |
| `collect_jvm_metrics` | true               | Should the set of support [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics) also be collected |
| `username`            | not set by default | The configured username if JMX is configured to require authentication. |
| `password`            | not set by default | The configured password if JMX is configured to require authentication. |
| `collection_interval` | `60s`              | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |


Example Configuration:

```yaml
metrics:
  receivers:
    hadoop:
      type: hadoop
  service:
    pipelines:
      hadoop:
        receivers:
          - hadoop
```

## Metrics
In addition to Hadoop specific metrics, by default Hadoop will also report [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics)

| Metric                                               | Data Type      | Unit        | Labels                         | Description |
| ---                                                  | ---            | ---         | ---                            | ---         | 
| `hadoop.name_node.capacity.usage` | Gauge | `by` | `node_name` | The current used capacity across all data nodes reporting to the name node. |
| `hadoop.name_node.capacity.limit` | Gauge | `by` | `node_name` | The total capacity allotted to data nodes reporting to the name node. |
| `hadoop.name_node.block.count` | Gauge | `{blocks}` | `node_name` | The total number of blocks on the name node. |
| `hadoop.name_node.block.missing` | Gauge | `{blocks}` | `node_name` | The number of blocks reported as missing to the name node. |
| `hadoop.name_node.block.corrupt` | Gauge | `{blocks}` | `node_name` | The number of blocks reported as corrupt to the name node. |
| `hadoop.name_node.volume.failed` | Gauge | `{volumes}` | `node_name` | The number of failed volumes reported to the name node. |
| `hadoop.name_node.file.count` | Gauge | `{files}` | `node_name` | The total number of files being tracked by the name node. |
| `hadoop.name_node.file.load` | Gauge | `{operations}` | `node_name` | The current number of concurrent file accesses. |
| `hadoop.name_node.data_node.count` | Gauge | `{nodes}` | `node_name`, `state` | The number of data nodes reporting to the name node. |

# `hadoop` Logging Receiver

## Configuration

To configure a logging receiver for hadoop, specify the following fields:

| Field                 | Default                       | Description |
| ---                   | ---                           | ---         |
| `type`                | required                      | Must be `hadoop`. |
| `include_paths`       | `[/opt/hadoop/logs/hadoop-*.log, /opt/hadoop/logs/yarn-*.log]` | The log files to read. |
| `exclude_paths`       | `[]`                          | Log files to exclude (if `include_paths` contains a glob or directory). |
| `wildcard_refresh_interval` | `60s` | The interval at which wildcard file paths in include_paths are refreshed. Specified as a time interval parsable by [time.ParseDuration](https://pkg.go.dev/time#ParseDuration). Must be a multiple of 1s.|

Example Configuration:

```yaml
logging:
  receivers:
    hadoop:
      type: hadoop
  service:
    pipelines:
      hadoop:
        receivers:
          - hadoop
```

## Logs

Hadoop logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.source` | string | The source Java class of the log entry |
| `jsonPayload.message` | string | Log message |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged |

If the fields are absent on the original log record, the will not be present on the resultant LogEntry.
