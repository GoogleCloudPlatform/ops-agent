# `hbase` Metrics Receiver

The `hbase` metrics receiver can fetch stats from a Hbase server's Java Virtual Machine (JVM) via [JMX](https://www.oracle.com/java/technologies/javase/javamanagement.html).

## Prerequisites

In order to expose a JMX endpoint, you must set the `com.sun.management.jmxremote.port` system property. It is recommended to also set the `com.sun.management.jmxremote.rmi.port` system property to the same port. To expose JMX endpoint remotely, you must also set the `java.rmi.server.hostname` system property. By default, these properties are set in a Hbase deployment's hbase-env.sh file and the default Hbase installation requires no JMX authentication with JMX exposed locally on 127.0.0.1:8050.

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
 hbase_metrics:
  type: hbase
  collection_interval: 60s
 service:
 pipelines:
  hbase_pipeline:
  receivers:
   - hbase_metrics
```

## Metrics
In addition to Hbase specific metrics, by default Hbase will also report [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics)

| Metric                                                 | Data Type  | Unit           | Labels                       | Description |
| ---                                                    | ---        | ---            | ---                          | ---   |  
|  hbase.master.region_server.count                      | gauge      |  {servers}     |  state                       | The number of region servers. 
|  hbase.master.in_transition_regions.count              | gauge      |  {regions}     |                              | The number of regions that are in transition. 
|  hbase.master.in_transition_regions.over_threshold     | gauge      |  {regions}     |                              | The number of regions that have been in transition longer than a threshold time. 
|  hbase.master.in_transition_regions.oldest_age         | gauge      |  ms            |                              | The age of the longest region in transition. 
|  hbase.region_server.region.count                      | gauge      |  {regions}     |  region_server               | The number of regions hosted by the region server. 
|  hbase.region_server.disk.store_file.count             | gauge      |  {files}       |  region_server               | The number of store files on disk currently managed by the region server. 
|  hbase.region_server.disk.store_file.size              | gauge      |  By            |  region_server               | Aggregate size of the store files on disk. 
|  hbase.region_server.write_ahead_log.count             | gauge      |  {logs}        |  region_server               | The number of write ahead logs not yet archived. 
|  hbase.region_server.request.count                     | gauge      |  {requests}    |  region_server , state       | The number of requests received. 
|  hbase.region_server.queue.length                      | gauge      |  {handlers}    |  region_server , state       | The number of RPC handlers actively servicing requests. 
|  hbase.region_server.blocked_update.time               | gauge      |  ms            |  region_server               | Amount of time updates have been blocked so the memstore can be flushed. 
|  hbase.region_server.block_cache.operation.count       | gauge      |  {operations}  |  region_server , state       | Number of block cache hits/misses. 
|  hbase.region_server.files.local                       | gauge      |  %             |  region_server               | Percent of store file data that can be read from the local. 
|  hbase.region_server.operation.append.latency.p99      | gauge      |  ms            |  region_server               | Append operation 99th Percentile latency. 
|  hbase.region_server.operation.append.latency.max      | gauge      |  ms            |  region_server               | Append operation max latency. 
|  hbase.region_server.operation.delete.latency.p99      | gauge      |  ms            |  region_server               | Delete operation 99th Percentile latency. 
|  hbase.region_server.operation.delete.latency.max      | gauge      |  ms            |  region_server               | Delete operation max latency. 
|  hbase.region_server.operation.put.latency.p99         | gauge      |  ms            |  region_server               | Put operation 99th Percentile latency. 
|  hbase.region_server.operation.put.latency.max         | gauge      |  ms            |  region_server               | Put operation max latency. 
|  hbase.region_server.operation.get.latency.p99         | gauge      |  ms            |  region_server               | Get operation 99th Percentile latency. 
|  hbase.region_server.operation.get.latency.max         | gauge      |  ms            |  region_server               | Get operation max latency. 
|  hbase.region_server.operation.replay.latency.p99      | gauge      |  ms            |  region_server               | Replay operation 99th Percentile latency. 
|  hbase.region_server.operation.replay.latency.max      | gauge      |  ms            |  region_server               | Replay operation max latency. 
|  hbase.region_server.operation.increment.latency.p99   | gauge      |  ms            |  region_server               | Increment operation 99th Percentile latency. 
|  hbase.region_server.operation.increment.latency.max   | gauge      |  ms            |  region_server               | Increment operation max latency. 
|  hbase.region_server.operations.slow                   | gauge      |  {operations}  |  region_server , operation   | Number of operations that took over 1000ms to complete. 
|  hbase.region_server.open_connection.count             | gauge      |  {connections} |  region_server               | The number of open connections at the RPC layer. 
|  hbase.region_server.active_handler.count              | gauge      |  {handlers}    |  region_server               | The number of RPC handlers actively servicing requests. 
|  hbase.region_server.queue.request.count               | gauge      |  {requests}    |  region_server , state       | The number of currently enqueued requests. 
|  hbase.region_server.authentication.count              | gauge      |  1             |  region_server , state       | Number of client connection authentication failures/successes. 
|  hbase.region_server.gc.time                           | cumulative |  ms            |  region_server               | Time spent in garbage collection. 
|  hbase.region_server.gc.young_gen.time                 | cumulative |  ms            |  region_server               | Time spent in garbage collection of the young generation. 
|  hbase.region_server.gc.old_gen.time                   | cumulative |  ms            |  region_server               | Time spent in garbage collection of the old generation.
         