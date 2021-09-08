# JVM Receiver

The `jvm` receiver can fetch stats from a Jave Virtual Machine via [JMX](https://www.oracle.com/java/technologies/javase/javamanagement.html).


## Prerequisites

In order to expose a JMX endpoint, you must set the `com.sun.management.jmxremote.port` system property. It is recommended to also set the `com.sun.management.jmxremote.rmi.port` system property to the same port. To expose JMX endpoint remotely, you must also set the `java.rmi.server.hostname` system property. Java system properties can be set via command line args by prepending the property name with `-D` . For example: `-Dcom.sun.management.jmxremote.port`

## Configuration

| Field                 | Default          | Description |
| ---                   | ---              | ---         |
| `type`                | required         | Must be `jvm`. |
| `endpoint`            | `localhost:9999` | The [JMX Service URL](https://docs.oracle.com/javase/8/docs/api/javax/management/remote/JMXServiceURL.html) or host and port used to construct the Service URL. Must be in the form of `service:jmx:<protocol>:<sap>` or `host:port`. Values in `host:port` form will be used to create a Service URL of `service:jmx:rmi:///jndi/rmi://<host>:<port>/jmxrmi`. |
| `collection_interval` | `60s`            | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |

Example Configuration:

```yaml
metrics:
  receivers:
    jvm_metrics:
      type: jvm
      endpoint: localhost:9999
      collection_interval: 30s
  service:
    pipelines:
      jvm_pipeline:
        receivers:
          - jvm_metrics
```

## Metrics

| Metric                                             | Data Type | Unit        | Labels | Description |
| ---                                                | ---       | ---         | ---    | ---         | 
| workload.googleapis.com/jvm.classes.loade          | gauge     | 1           |        | Current number of loaded classes |
| workload.googleapis.com/jvm.gc.collections.count   | sum       | 1           | name   | Total number of garbage collections |
| workload.googleapis.com/jvm.gc.collections.elapsed | sum       | ms          | name   | Time spent garbage collecting |
| workload.googleapis.com/jvm.memory.heap            | gauge     | by          |        | Current heap usage |
| workload.googleapis.com/jvm.memory.nonheap         | gauge     | by          |        | Current non-heap usage |
| workload.googleapis.com/jvm.memory.jvm.memory.pool | gauge     | by          | name   | Current memory pool usage |
| workload.googleapis.com/jvm.threads.count          | gauge     | 1           |        | Current number of threads |