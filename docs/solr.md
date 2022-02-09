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

# `solr_system` Logging Receiver

## Configuration

To configure a receiver for your Solr system logs, specify the following fields:

| Field                 | Default                           | Description |
| ---                   | ---                               | ---         |
| `type`                | required                          | Must be `solr_system`. |
| `include_paths`       | `[/var/solr/logs/solr.log]`       | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `/var/solr/logs/*.log`.
| `exclude_paths`       | `[]`                              | A list of filesystem path patterns to exclude from the set matched by `include_paths`.
| `wildcard_refresh_interval` | `60s`                       | The interval at which wildcard file paths in `include_paths` are refreshed. Given as a time duration, for example `30s`, `2m`. This property might be useful under high logging throughputs where log files are rotated faster than the default interval.|

Example Configuration:

```yaml
logging:
  receivers:
    solr_system:
      type: solr_system
  service:
    pipelines:
      solr:
        receivers:
          - solr_system
```

## Logs

System logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received |
| `jsonPayload.collection` | string | Solr collection related to the log |
| `jsonPayload.shard` | string | Solr shard related to the log |
| `jsonPayload.replica` | string | Solr replica related to the log |
| `jsonPayload.core` | string | Solr core related to the log |
| `jsonPayload.source` | string | Source of where the log originated |
| `jsonPayload.thread` | string | Thread where the log originated |
| `jsonPayload.message` | string | Log message |
| `jsonPayload.exception` | string | Exception related to the log, including detailed stacktrace where provided |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |

Any fields that are blank or missing will not be present in the log entry.