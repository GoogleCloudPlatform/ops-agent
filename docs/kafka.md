# `kafka` Metrics Receiver

The `kafka` metrics receiver can fetch stats from a Kafka broker's Java Virtual Machine (JVM) via [JMX](https://www.oracle.com/java/technologies/javase/javamanagement.html).

## Prerequisites

In order to expose a JMX endpoint, you must set the `com.sun.management.jmxremote.port` system property. It is recommended to also set the `com.sun.management.jmxremote.rmi.port` system property to the same port. To expose JMX endpoint remotely, you must also set the `java.rmi.server.hostname` system property. These are often set by setting the environment variable "KAFKA_JMX_OPTS" in `bin/kafka-run-class.sh` and "JMX_PORT" in `bin/kafka-server-start.sh` for the Kafka service environment.

## Configuration

| Field                 | Default            | Description |
| ---                   | ---                | ---         |
| `type`                | required           | Must be `kafka`. |
| `endpoint`            | `localhost:9999`   | The [JMX Service URL](https://docs.oracle.com/javase/8/docs/api/javax/management/remote/JMXServiceURL.html) or host and port used to construct the Service URL. Must be in the form of `service:jmx:<protocol>:<sap>` or `host:port`. Values in `host:port` form will be used to create a Service URL of `service:jmx:rmi:///jndi/rmi://<host>:<port>/jmxrmi`. |
| `collect_jvm_metrics` | true               | Should the set of support [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics) also be collected |
| `username`            | not set by default | The configured username if JMX is configured to require authentication. |
| `password`            | not set by default | The configured password if JMX is configured to require authentication. |
| `collection_interval` | `60s`              | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |


Example Configuration:

```yaml
metrics:
  receivers:
    kafka:
      type: kafka
  service:
    pipelines:
      kafka:
        receivers:
          - kafka
```

## Metrics
In addition to Kafka specific metrics, by default Kafka will also report [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics)

| Metric                                                    | Data Type | Unit        | Labels | Description |
| ---                                                       | ---       | ---         | ---    | ---         | 
| workload.googleapis.com/kafka.message.count               | cumulative | messages | operation | The number of messages received by the broker |
| workload.googleapis.com/kafka.request.count               | cumulative | requests          | type | The number of requests received by the broker |
| workload.googleapis.com/kafka.request.failed              | cumulative | requests          | type | The number of requests to the broker resulting in a failure |
| workload.googleapis.com/kafka.request.time.total          | cumulative | ms          | type | The total time the broker has taken to service requests |
| workload.googleapis.com/kafka.network.io                  | cumulative | by          | state | The bytes received or sent by the broker |
| workload.googleapis.com/kafka.purgatory.size              | gauge | requests          | type | The number of requests waiting in purgatory |
| workload.googleapis.com/kafka.partition.count             | gauge | partitions          |  | The number of partitions on the broker |
| workload.googleapis.com/kafka.partition.offline           | gauge | partitions          |  | The number of partitions offline |
| workload.googleapis.com/kafka.partition.under_replicated  | gauge | partitions          |  | The number of under replicated partitions |
| workload.googleapis.com/kafka.isr.operation.count         | cumulative | 1          | operation | The number of in-sync replica shrink and expand operations |


# `kafka` Logging Receiver

## Configuration

To configure a receiver for your kafka logs, specify the following fields:

| Field                 | Default                       | Description |
| ---                   | ---                           | ---         |
| `type`                | required                      | Must be `kafka`. |
| `include_paths`       | `[/var/log/kafka/*.log]` | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `/var/log/kafka*/*.log`.
| `exclude_paths`       | `[]`                          | A list of filesystem path patterns to exclude from the set matched by `include_paths`.
| `wildcard_refresh_interval` | `60s` | The interval at which wildcard file paths in include_paths are refreshed. Given as a time duration, for example 30s, 2m. This property might be useful under high logging throughputs where log files are rotated faster than the default interval. Must be a multiple of 1s.|

Example Configuration:

```yaml
logging:
  receivers:
    kafka:
      type: kafka
  service:
    pipelines:
      kafka:
        receivers:
          - kafka
```

## Logs

Kafka logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.source` | string | Module and/or thread  where the log originated |
| `jsonPayload.logger` | string | Name of the logger where the log originated |
| `jsonPayload.message` | string | Log message, including detailed stacktrace where provided |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received |

Any fields that are blank or missing will not be present in the log entry.
