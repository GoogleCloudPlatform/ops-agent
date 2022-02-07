# `solr` Metrics Receiver

The `solr` metrics receiver can fetch stats from a Solr server's Java Virtual Machine (JVM) via [JMX](https://www.oracle.com/java/technologies/javase/javamanagement.html).

## Prerequisites

In order to expose a JMX endpoint, you must set the `com.sun.management.jmxremote.port` system property. It is recommended to also set the `com.sun.management.jmxremote.rmi.port` system property to the same port. To expose JMX endpoint remotely, you must also set the `java.rmi.server.hostname` system property. By default, these properties are set in a Solr deployment's solr-env.sh file and the default Solr installation requires no JMX authentication with JMX exposed locally on 127.0.0.1:18983.

## Configuration

| Field                 | Default            | Description |
| ---                   | ---                | ---         |
| `type`                | required           | Must be `solr`. |
| `endpoint`            | `localhost:18983`  | The [JMX Service URL](https://docs.oracle.com/javase/8/docs/api/javax/management/remote/JMXServiceURL.html) or host and port used to construct the Service URL. Must be in the form of `host:port`. Values in `host:port` form will be used to create a Service URL of `service:jmx:rmi:///jndi/rmi://<host>:<port>/jmxrmi`. |
| `username`            | not set by default | The configured username if JMX is configured to require authentication. |
| `password`            | not set by default | The configured password if JMX is configured to require authentication. |
| `collection_interval` | `60s`              | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |


Example Configuration:

```yaml
metrics:
  receivers:
    solr:
      type: solr
  service:
    pipelines:
      solr:
        receivers:
          - solr
```

## Metrics
The Ops Agent collects the following metrics from Solr.

Note: Limited metrics will be reported if no cores have been created.

| Metric                                               | Data Type      | Unit        | Labels                         | Description |
| ---                                                  | ---            | ---         | ---                            | ---         |
| workload.googleapis.com/solr.document.count          | Gauge          | documents   | core                           | The total number of indexed documents. |
| workload.googleapis.com/solr.index.size              | Gauge          | by          | core                           | The total index size. |
| workload.googleapis.com/solr.request.count           | Cumulative     | queries     | core, type, handler            | The number of queries made. |
| workload.googleapis.com/solr.request.time.average    | Gauge          | ms          | core, type, handler            | The average time of a query, based on Solr's histogram configuration. |
| workload.googleapis.com/solr.request.error.count     | Cumulative     | queries     | core, type, handler            | The number of queries resulting in an error. |
| workload.googleapis.com/solr.request.timeout.count   | Cumulative     | queries     | core, type, handler            | The number of queries resulting in a timeout. |
| workload.googleapis.com/solr.cache.eviction.count    | Cumulative     | evictions   | core, cache                    | The number of evictions from a cache. |
| workload.googleapis.com/solr.cache.hit.count         | Cumulative     | hits        | core, cache                    | The number of hits from a cache. |
| workload.googleapis.com/solr.cache.insert.count      | Cumulative     | inserts     | core, cache                    | The number of inserts from a cache. |
| workload.googleapis.com/solr.cache.lookup.count      | Cumulative     | lookups     | core, cache                    | The number of lookups from a cache. |
| workload.googleapis.com/solr.cache.size              | Gauge          | by          | core, cache                    | The size of the cache occupied in memory. |
