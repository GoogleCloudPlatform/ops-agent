# `activemq` Metrics Receiver

The `activemq` metrics receiver will be collected through the [Otel JMX Metric Gatherer](https://github.com/open-telemetry/opentelemetry-java-contrib/tree/main/jmx-metrics).
.

## Prerequisites

JMX support must be enabled in the [broker configuration](https://activemq.apache.org/jmx).

## Configuration

| Field                 | Default            | Description |
| ---                   |--------------------| ---         |
| `endpoint`            | `localhost:1099`   | Must be in the form of `host:port`.|
| `username`            | not set by default | The configured username if JMX is configured to require authentication. |
| `password`            | not set by default | The configured password if JMX is configured to require authentication. |
| `collection_interval` | `60s`              | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |


Example Configuration:

```yaml
metrics:
  receivers:
    activemq:
      type: activemq
      endpoint: localhost:1099
  service:
    pipelines:
      activemq:
        receivers:
          - activemq
```

## Metrics
In addition to ActiveMQ specific metrics, by default ActiveMQ will also report [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics)

| Metric                                                                          | Data Type | Unit        | Labels      | Description |
| ---                                                                             | ---       | ---         | ---         | ---         | 
| workload.googleapis.com/activemq.consumer.count                                 | gauge     | consumers   | destination | The number of consumers currently reading from the broker.|
| workload.googleapis.com/activemq.producer.count                                 | gauge     | producers   | destination | The number of producers currently attached to the broker. |
| workload.googleapis.com/activemq.connection.count                               | gauge     | connections |             | The total number of current connections. |
| workload.googleapis.com/activemq.memory.usage                                   | gauge     | %           | destination | The percentage of configured memory used. |
| workload.googleapis.com/activemq.disk.store_usage                               | gauge     | %           |             | The percentage of configured disk used for persistent messages. |
| workload.googleapis.com/activemq.disk.temp_usage                                | gauge     | %           |             | The percentage of configured disk used for non-persistent messages. |
| workload.googleapis.com/activemq.message.current                                | gauge     | messages    | destination | The current number of messages waiting to be consumed. |
| workload.googleapis.com/activemq.message.expired                                | cumulative| messages    | destination | The total number of messages not delivered because they expired. |
| workload.googleapis.com/activemq.message.enqueued                               | cumulative| messages    | destination | The total number of messages received by the broker. |
| workload.googleapis.com/activemq.message.dequeued                               | cumulative| messages    | destination | The total number of messages delivered to consumers. |
| workload.googleapis.com/activemq.message.wait_time.avg                          | gauge     | ms          | destination | The average time a message was held on a destination. |


#  Logging Receivers

## Configuration

ActiveMQ is configured by default to write logs to both console and file. Since the Ops Agent has a syslog input enabled by default, there is no need to add additional collection functionality.



## Logs

System logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field                    | Type | Description                                      |
|--------------------------| ---- |--------------------------------------------------|
| `jsonPayload.source`     | string | Source file which log originated                 |
| `jsonPayload.thread`     | string | thread name of activemq where the log originated |
| `jsonPayload.exception`  | string | exception that occured while running             |
| `jsonPayload.message`    | string | Log message                                      |
| `severity`               | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated)                     |
| `timestamp`              | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received                    |

Any fields that are blank or missing will not be present in the log entry.
