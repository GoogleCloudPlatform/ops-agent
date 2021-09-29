# Cassandra Receiver

The `cassandra` receiver can fetch stats from a Cassandra node's Java Virtual Machine (JVM) via [JMX](https://www.oracle.com/java/technologies/javase/javamanagement.html).


## Prerequisites

In order to expose a JMX endpoint, you must set the `com.sun.management.jmxremote.port` system property. It is recommended to also set the `com.sun.management.jmxremote.rmi.port` system property to the same port. To expose JMX endpoint remotely, you must also set the `java.rmi.server.hostname` system property. By default, these properties are set in a Cassandra deployment's cassandra-env.sh file and the default Cassandra installation requires no JMX authentication with JMX exposed locally on 127.0.0.1:7199.

## Configuration

| Field                 | Default            | Description |
| ---                   | ---                | ---         |
| `type`                | required           | Must be `jvm`. |
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
| workload.googleapis.com/cassandra.client.request.read.latency.50p               | gauge     | µs          |        | Standard read request latency - 50th percentile |
| workload.googleapis.com/cassandra.client.request.read.latency.99p               | gauge     | µs          |        | Standard read request latency - 99th percentile |
| workload.googleapis.com/cassandra.client.request.read.latency.count             | sum       | µs          |        | Total standard read request latency |
| workload.googleapis.com/cassandra.client.request.read.latency.max               | gauge     | µs          |        | Maximum standard read request latency |
| workload.googleapis.com/cassandra.client.request.read.timeout.count             | sum       | 1           |        | Number of standard read request timeouts encountered |
| workload.googleapis.com/cassandra.client.request.read.unavailable.count         | sum       | 1           |        | Number of standard read request unavailable exceptions encountered |
| workload.googleapis.com/cassandra.client.request.write.latency.50p              | gauge     | µs          |        | Standard write request latency - 50th percentile |
| workload.googleapis.com/cassandra.client.request.write.latency.99p              | gauge     | µs          |        | Standard write request latency - 99th percentile |
| workload.googleapis.com/cassandra.client.request.write.latency.count            | sum       | µs          |        | Total standard write request latency |
| workload.googleapis.com/cassandra.client.request.write.latency.max              | gauge     | µs          |        | Maximum standard write request latency |
| workload.googleapis.com/cassandra.client.request.write.timeout.count            | sum       | 1           |        | Number of standard write request timeouts encountered |
| workload.googleapis.com/cassandra.client.request.write.unavailable.count        | sum       | 1           |        | Number of standard write request unavailable exceptions encountered |
| workload.googleapis.com/cassandra.client.request.range_slice.latency.50p        | gauge     | µs          |        | Token range read request latency - 50th percentile |
| workload.googleapis.com/cassandra.client.request.range_slice.latency.99p        | gauge     | µs          |        | Token range read request latency - 99th percentile |
| workload.googleapis.com/cassandra.client.request.range_slice.latency.count      | sum       | µs          |        | Total token range read request latency |
| workload.googleapis.com/cassandra.client.request.range_slice.latency.max        | gauge     | µs          |        | Maximum token range read request latency |
| workload.googleapis.com/cassandra.client.request.range_slice.timeout.count      | sum       | 1           |        | Number of token range read request timeouts encountered |
| workload.googleapis.com/cassandra.client.request.range_slice.unavailable.count  | sum       | 1           |        | Number of token range read request unavailable exceptions encountered |
| workload.googleapis.com/cassandra.compaction.tasks.completed                    | sum       | 1           |        | Number of completed compactions since server start |
| workload.googleapis.com/cassandra.compaction.tasks.pending                      | gauge     | 1           |        | Estimated number of compactions remaining to perform |
| workload.googleapis.com/cassandra.storage.load.count                            | sum       | bytes       |        | Size of the on disk data size this node manages |
| workload.googleapis.com/cassandra.storage.total_hints.count                     | sum       | 1           |        | Number of hint messages written to this node since start |
| workload.googleapis.com/cassandra.storage.total_hints.in_progress.count         | sum       | 1           |        | Number of hints attempting to be sent currently |
