# `jetty` Metrics Receiver

The `jetty` metric receiver can fetch stats from a Jetty server's Java Virtual Machine (JVM) via [JMX](https://www.oracle.com/java/technologies/javase/javamanagement.html).
## Prerequisites

In order to expose a JMX endpoint, you must set the `com.sun.management.jmxremote.port` system property. It is recommended to also set the `com.sun.management.jmxremote.rmi.port` system property to the same port. To expose a JMX endpoint remotely, you must also set the `java.rmi.server.hostname` system property. You should set these properties when running the `start.jar` file when starting the server. Also, the `jmx` module needs to be enabled on the jetty server.

## Configuration

| Field                          | Default            | Description      |
| ---                            | ---                | ---              |
| `type`                         | required           | Must be `jetty`. |
| `endpoint`                     | `localhost:1099`   | The [JMX Service URL](https://docs.oracle.com/javase/8/docs/api/javax/management/remote/JMXServiceURL.html) or host and port used to construct the Service URL. Must be in the form of `service:jmx:<protocol>:<sap>` or `host:port`. Values in `host:port` form will be used to create a Service URL of `service:jmx:rmi:///jndi/rmi://<host>:<port>/jmxrmi`. |
| `collect_jvm_metrics`          | true               | Should the set of support [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics) also be collected |
| `username`                     | not set by default | The configured username if JMX is configured to require authentication. |
| `password`                     | not set by default | The configured password if JMX is configured to require authentication. |
| `collection_interval`          | `60s`              | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |

Example Configuration:

```yaml
metrics:
   receivers:
      jetty:
         type: jetty
   service:
      pipelines:
         jetty:
            receivers:
               - jetty
```

## Metrics
In addition to Jetty specific metrics, by default Jetty will also report [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics)

| Metric                     | Data Type  | Unit           | Labels       | Description |
| ---                        | ---        | ---            | ---          | ---   |  
| `jetty.select.count`       | cumulative | `{operations}` |              | The number of select calls. |
| `jetty.session.count`      | cumulative | `{sessions}`   | resource     | The number of sessions created. |
| `jetty.session.time.total` | gauge      | `s`            | resource     | The total time sessions have been active. |
| `jetty.session.time.max`   | gauge      | `s`            | resource     | The maximum amount of time a session has been active. |
| `jetty.thread.count`       | gauge      | `{threads}`    | state        | The current number of threads. |
| `jetty.thread.queue.count` | gauge      | `{threads}`    |              | The current number of threads in the queue. |