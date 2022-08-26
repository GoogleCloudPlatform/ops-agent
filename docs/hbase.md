# `hbase` Metrics Receiver

The `hbase` metric receiver can fetch stats from a HBase server's Java Virtual Machine (JVM) via [JMX](https://www.oracle.com/java/technologies/javase/javamanagement.html). It collects metrics specific to the local region server, as well as metrics presented by the Master node if the node being monitored is indeed the Master.

For High Availability configurations, it is recommended for every master node to report cluster metrics, which will have identical values, to avoid single point of failures when one master goes down.
## Prerequisites

In order to expose a JMX endpoint, you must set the `com.sun.management.jmxremote.port` system property. It is recommended to also set the `com.sun.management.jmxremote.rmi.port` system property to the same port. To expose JMX endpoint remotely, you must also set the `java.rmi.server.hostname` system property. In the default hbase-env.sh file these properties are set. it  requires no JMX authentication with JMX exposed locally on 127.0.0.1:10101 when the default line for the JMX port is uncommented.

## Configuration

| Field     | Default   | Description |
| ---     | ---    | ---   |
| `type`    | required   | Must be `hbase`. |
| `endpoint`   | `localhost:10101` | The [JMX Service URL](https://docs.oracle.com/javase/8/docs/api/javax/management/remote/JMXServiceURL.html) or host and port used to construct the Service URL. Must be in the form of `service:jmx:<protocol>:<sap>` or `host:port`. Values in `host:port` form will be used to create a Service URL of `service:jmx:rmi:///jndi/rmi://<host>:<port>/jmxrmi`. |
| `collect_jvm_metrics` | true    | Should the set of support [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics) also be collected |
| `username`   | not set by default | The configured username if JMX is configured to require authentication. |
| `password`   | not set by default | The configured password if JMX is configured to require authentication. |
| `collection_interval` | `60s`    | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |

Example Configuration:

```yaml
metrics:
   receivers:
      hbase:
         type: hbase
   service:
      pipelines:
         hbase:
            receivers:
               - hbase
```

## Metrics
In addition to Hbase specific metrics, by default Hbase will also report [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics)

| Metric                                                 | Data Type  | Unit           | Labels                       | Description |
| ---                                                    | ---        | ---            | ---                          | ---   |  
| `hbase.master.region_server.count` | Gauge | `{servers}` | `state` | The number of region servers. |
| `hbase.master.regions_in_transition.count` | Gauge | `{regions}` |  | The number of regions that are in transition. |
| `hbase.master.regions_in_transition.over_threshold` | Gauge | `{regions}` |  | The number of regions that have been in transition longer than a threshold time. |
| `hbase.master.regions_in_transition.oldest_age` | Gauge | `ms` |  | The age of the longest region in transition. |
| `hbase.region_server.region.count` | Gauge | `{regions}` |  | The number of regions hosted by the region server. |
| `hbase.region_server.disk.store_file.count` | Gauge | `{files}` |  | The number of store files on disk currently managed by the region server. |
| `hbase.region_server.disk.store_file.size` | Gauge | `By` |  | Aggregate size of the store files on disk. |
| `hbase.region_server.write_ahead_log.count` | Gauge | `{logs}` |  | The number of write ahead logs not yet archived. |
| `hbase.region_server.request.count` | Gauge | `{requests}` | , `state` | The number of requests received. |
| `hbase.region_server.queue.length` | Gauge | `{handlers}` | , `state` | The number of RPC handlers actively servicing requests. |
| `hbase.region_server.blocked_update.time` | Gauge | `ms` |  | Amount of time updates have been blocked so the memstore can be flushed. |
| `hbase.region_server.block_cache.operation.count` | Gauge | `{operations}` | , `state` | Number of block cache hits/misses. |
| `hbase.region_server.files.local` | Gauge | `%` |  | Percent of store file data that can be read from the local. |
| `hbase.region_server.operation.append.latency.p99` | Gauge | `ms` |  | Append operation 99th Percentile latency. |
| `hbase.region_server.operation.append.latency.max` | Gauge | `ms` |  | Append operation max latency. |
| `hbase.region_server.operation.append.latency.min` | Gauge | `ms` |  | Append operation minimum latency. |
| `hbase.region_server.operation.append.latency.mean` | Gauge | `ms` |  | Append operation mean latency. |
| `hbase.region_server.operation.append.latency.median` | Gauge | `ms` |  | Append operation median latency. |
| `hbase.region_server.operation.delete.latency.p99` | Gauge | `ms` |  | Delete operation 99th Percentile latency. |
| `hbase.region_server.operation.delete.latency.max` | Gauge | `ms` |  | Delete operation max latency. |
| `hbase.region_server.operation.delete.latency.min` | Gauge | `ms` |  | Delete operation minimum latency. |
| `hbase.region_server.operation.delete.latency.mean` | Gauge | `ms` |  | Delete operation mean latency. |
| `hbase.region_server.operation.delete.latency.median` | Gauge | `ms` |  | Delete operation median latency. |
| `hbase.region_server.operation.put.latency.p99` | Gauge | `ms` |  | Put operation 99th Percentile latency. |
| `hbase.region_server.operation.put.latency.max` | Gauge | `ms` |  | Put operation max latency. |
| `hbase.region_server.operation.put.latency.min` | Gauge | `ms` |  | Put operation minimum latency. |
| `hbase.region_server.operation.put.latency.mean` | Gauge | `ms` |  | Put operation mean latency. |
| `hbase.region_server.operation.put.latency.median` | Gauge | `ms` |  | Put operation median latency. |
| `hbase.region_server.operation.get.latency.p99` | Gauge | `ms` |  | Get operation 99th Percentile latency. |
| `hbase.region_server.operation.get.latency.max` | Gauge | `ms` |  | Get operation max latency. |
| `hbase.region_server.operation.get.latency.min` | Gauge | `ms` |  | Get operation minimum latency. |
| `hbase.region_server.operation.get.latency.mean` | Gauge | `ms` |  | Get operation mean latency. |
| `hbase.region_server.operation.get.latency.median` | Gauge | `ms` |  | Get operation median latency. |
| `hbase.region_server.operation.replay.latency.p99` | Gauge | `ms` |  | Replay operation 99th Percentile latency. |
| `hbase.region_server.operation.replay.latency.max` | Gauge | `ms` |  | Replay operation max latency. |
| `hbase.region_server.operation.replay.latency.min` | Gauge | `ms` |  | Replay operation minimum latency. |
| `hbase.region_server.operation.replay.latency.mean` | Gauge | `ms` |  | Replay operation mean latency. |
| `hbase.region_server.operation.replay.latency.median` | Gauge | `ms` |  | Replay operation median latency. |
| `hbase.region_server.operation.increment.latency.p99` | Gauge | `ms` |  | Increment operation 99th Percentile latency. |
| `hbase.region_server.operation.increment.latency.max` | Gauge | `ms` |  | Increment operation max latency. |
| `hbase.region_server.operation.increment.latency.min` | Gauge | `ms` |  | Increment operation minimum latency. |
| `hbase.region_server.operation.increment.latency.mean` | Gauge | `ms` |  | Increment operation mean latency. |
| `hbase.region_server.operation.increment.latency.median` | Gauge | `ms` |  | Increment operation median latency. |
| `hbase.region_server.operations.slow` | Gauge | `{operations}` | , `operation` | Number of operations that took over 1000ms to complete. |
| `hbase.region_server.open_connection.count` | Gauge | `{connections}` |  | The number of open connections at the RPC layer. |
| `hbase.region_server.active_handler.count` | Gauge | `{handlers}` |  | The number of RPC handlers actively servicing requests. |
| `hbase.region_server.queue.request.count` | Gauge | `{requests}` | , `state` | The number of currently enqueued requests. |
| `hbase.region_server.authentication.count` | Gauge | `{authentication requests}` | , `state` | Number of client connection authentication failures/successes. |
| `hbase.region_server.gc.time` | Cumulative | `ms` |  | Time spent in garbage collection. |
| `hbase.region_server.gc.young_gen.time` | Cumulative | `ms` |  | Time spent in garbage collection of the young generation. |
| `hbase.region_server.gc.old_gen.time` | Cumulative | `ms` |  | Time spent in garbage collection of the old generation. |

### Metrics labels

| Name | Description | Values |
| ---- | ----------- | ------ |
| hbase.region_server.queue.request.count.state | The type of request queue. | replication, user, priority |
| hbase.master.region_server.count.state | state of server. | dead, live |
| hbase.region_server.request.count.state | The type of request. | read, write |
| hbase.region_server.queue.length.state | The type of handlers. | flush, compaction |
| hbase.region_server.block_cache.operation.count.state | The type of operation. | miss, hit |
| hbase.region_server.authentication.count.state | The type of authentications. | successes, failures |

# `hbase_system` Logging Receiver

## Configuration

To configure a receiver for your hbase system logs, specify the following fields:

| Field                 | Default                           | Description |
| ---                   | ---                               | ---         |
| `type`                | required                          | Must be `hbase_system`. |
| `include_paths`       | `[/opt/hbase/logs/hbase-*-regionserver-*.log, /opt/hbase/logs/hbase-*-master-*.log]` | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `/var/log/hbase*/*.log`. |
| `exclude_paths`       | `[]`                              | A list of filesystem path patterns to exclude from the set matched by `include_paths`.
| `wildcard_refresh_interval` | `60s` | The interval at which wildcard file paths in `include_paths` are refreshed. Specified as a time interval parsable by [time.ParseDuration](https://pkg.go.dev/time#ParseDuration). Must be a multiple of 1s.|

Example Configuration:

```yaml
logging:
  receivers:
    hbase_system:
      type: hbase_system
  service:
    pipelines:
      hbase:
        receivers:
          - hbase_system
```

## Logs

System logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received |
| `jsonPayload.module` | string | Module of hbase where the log originated |
| `jsonPayload.source` | string | source of where the log originated |
| `jsonPayload.message` | string | Log message, including detailed stacktrace where provided |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |

Any fields that are blank or missing will not be present in the log entry.