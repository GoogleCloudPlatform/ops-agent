# `tomcat` Metrics Receiver

The `tomcat` metrics receiver can fetch stats from a Tomcat server's Java Virtual Machine (JVM) via [JMX](https://www.oracle.com/java/technologies/javase/javamanagement.html).

## Prerequisites

In order to expose a JMX endpoint, you must set the `com.sun.management.jmxremote.port` system property. It is recommended to also set the `com.sun.management.jmxremote.rmi.port` system property to the same port. To expose JMX endpoint remotely, you must also set the `java.rmi.server.hostname` system property. By default, these properties are set in a Tomcat deployment's tomcat-env.sh file and the default Tomcat installation requires no JMX authentication with JMX exposed locally on 127.0.0.1:8050.

## Configuration

| Field                 | Default            | Description |
| ---                   | ---                | ---         |
| `type`                | required           | Must be `tomcat`. |
| `endpoint`            | `localhost:8050`   | The [JMX Service URL](https://docs.oracle.com/javase/8/docs/api/javax/management/remote/JMXServiceURL.html) or host and port used to construct the Service URL. Must be in the form of `service:jmx:<protocol>:<sap>` or `host:port`. Values in `host:port` form will be used to create a Service URL of `service:jmx:rmi:///jndi/rmi://<host>:<port>/jmxrmi`. |
| `collect_jvm_metrics` | true               | Should the set of support [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics) also be collected |
| `username`            | not set by default | The configured username if JMX is configured to require authentication. |
| `password`            | not set by default | The configured password if JMX is configured to require authentication. |
| `collection_interval` | `60s`              | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |


Example Configuration:

```yaml
metrics:
  receivers:
    tomcat_metrics:
      type: tomcat
      collection_interval: 60s
  service:
    pipelines:
      tomcat_pipeline:
        receivers:
          - tomcat_metrics
```

## Metrics
In addition to Tomcat specific metrics, by default Tomcat will also report [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics)

| Metric                                               | Data Type      | Unit        | Labels                         | Description |
| ---                                                  | ---            | ---         | ---                            | ---         | 
| workload.googleapis.com/tomcat.sessions              | Gauge          | sessions    |                                | The number of active sessions. |
| workload.googleapis.com/tomcat.errors                | Cumulative     | errors      | proto_handler                  | The number of errors encountered. |
| workload.googleapis.com/tomcat.processing_time       | Cumulative     | ms          | proto_handler                  | The total processing time. |
| workload.googleapis.com/tomcat.traffic               | Cumulative     | by          | proto_handler, direction       | The number of bytes transmitted and received. |
| workload.googleapis.com/tomcat.threads               | Gauge          | threads     | proto_handler, state           | The number of threads. |
| workload.googleapis.com/tomcat.max_time              | Gauge          | ms          | proto_handler                  | Maximum time to process a request. |
| workload.googleapis.com/tomcat.request_count         | Cumulative     | requests    | proto_handler                  | The total requests. |


