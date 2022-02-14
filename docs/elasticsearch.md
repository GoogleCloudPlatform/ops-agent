# Elasticsearch
# `elasticsearch` Metrics Receiver

The `elasticsearch` metrics receiver fetches node and cluster level stats from Elasticsearch nodes. The receiver is meant to be run 

## Prerequisites
Elasticsearch supports collecting metrics out of the box. If [Elasticsearch security features](https://www.elastic.co/guide/en/elasticsearch/reference/7.16/secure-cluster.html) are enabled, a user with the `monitor` or `manage` [cluster privilege](https://www.elastic.co/guide/en/elasticsearch/reference/7.16/security-privileges.html#privileges-list-cluster) must be configured.

## Configurations
To configure a receiver for your Elasticsearch JSON logs, specify the following fields:

| Field                  | Required | Default                 | Description                                                                                                                                                             |
|------------------------|----------|-------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `type`                 | required |                         | Must be `elasticsearch`.                                                                                                                                                |
| `collection_interval`  | optional | `60s`                   | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`.                                                                                  |
| `endpoint`             | optional | `http://localhost:9200` | Base URL for the Elasticsearch REST API                                                                                                                                 |
| `username`             | optional |                         | Username for authentication with Elasticsearch. Required if `password` is set.                                                                                          |
| `password`             | optional |                         | Password for authentication with Elasticsearch. Required if `username` is set.                                                                                          |
| `collect_jvm_metrics`  | optional | `true`                  | If true, supported JVM metrics will be collected                                                                                                                        |
| `collect_cluster_metrics` | optional | `true`                  | If true, cluster level metrics will be collected. To prevent duplicate metrics, this should be set to true for one node only.                                      |
| `insecure`             | optional | true                    | Signals whether to use a secure TLS connection or not. If insecure is true TLS will not be enabled.                                                                     |
| `insecure_skip_verify` | optional | false                   | Whether to skip verifying the certificate or not. A false value of insecure_skip_verify will not be used if insecure is true as the connection will not use TLS at all. |
| `cert_file`            | optional |                         | Path to the TLS cert to use for mTLS required connections.                                                                                                              |
| `key_file`             | optional |                         | Path to the TLS key to use for mTLS required connections.                                                                                                               |
| `ca_file`              | optional |                         | Path to the CA cert. As a client this verifies the server certificate. If empty, uses system root CA.                                                                   |

Example Configuration:

```yaml
metrics:
  receivers:
    elasticsearch:
      type: elasticsearch
  service:
    pipelines:
      elasticsearch:
        receivers:
          - elasticsearch
```

Example Configuration with Credentials:
```yaml
metrics:
  receivers:
    elasticsearch:
      type: elasticsearch
      username: "user"
      password: "password"
  service:
    pipelines:
      elasticsearch:
        receivers:
          - elasticsearch
```

Example Configuration with TLS and Credentials:
```yaml
metrics:
  receivers:
    elasticsearch:
      type: elasticsearch
      endpoint: "https://example.com/elasticsearch"
      username: "user"
      password: "password"
      insecure: false
      insecure_skip_verify: false
      ca_file: /path/to/ca
  service:
    pipelines:
      elasticsearch:
        receivers:
          - elasticsearch
```
## Metrics

The Ops Agent collects the following metrics from your Elasticsearch nodes:

| Metric                                        | Data Type          | Unit          | Labels                  | Description                                                                              |
|-----------------------------------------------|--------------------|---------------|-------------------------|------------------------------------------------------------------------------------------|
| workload.googleapis.com/elasticsearch.node.cache.memory.usage         | Gauge (INT64)      | By            | cache_name              | The size in bytes of the cache.                                                          |
| workload.googleapis.com/elasticsearch.node.cache.evictions            | Cumulative (INT64) | {evictions}   | cache_name              | The number of evictions from the cache.                                                  |
| workload.googleapis.com/elasticsearch.node.fs.disk.available          | Gauge (INT64)      | By            |                         | The amount of disk space available across all file stores for this node.                 |
| workload.googleapis.com/elasticsearch.node.cluster.io                 | Cumulative (INT64) | By            | direction               | The number of bytes sent and received on the network for internal cluster communication. |
| workload.googleapis.com/elasticsearch.node.cluster.connections        | Gauge (INT64)      | {connections} |                         | The number of open tcp connections for internal cluster communication.                   |
| workload.googleapis.com/elasticsearch.node.http.connections           | Gauge (INT64)      | {connections} |                         | The number of HTTP connections to the node.                                              |
| workload.googleapis.com/elasticsearch.node.operations.completed       | Cumulative (INT64) | {operations}  | operation               | The number of operations completed.                                                      |
| workload.googleapis.com/elasticsearch.node.operations.time            | Cumulative (INT64) | ms            | operation               | Time spent on operations.                                                                |
| workload.googleapis.com/elasticsearch.node.shards.size                | Gauge (INT64)      | By            |                         | The size of the shards assigned to this node.                                            |
| workload.googleapis.com/elasticsearch.node.thread_pool.threads        | Gauge (INT64)      | {threads}     | thread_pool_name, state | The number of threads in the thread pool.                                                |
| workload.googleapis.com/elasticsearch.node.thread_pool.tasks.queued   | Gauge (INT64)      | {tasks}       | thread_pool_name        | The number of queued tasks in the thread pool.                                           |
| workload.googleapis.com/elasticsearch.node.thread_pool.tasks.finished | Cumulative (INT64) | {tasks}       | thread_pool_name, state | The number of tasks finished by the thread pool.                                         |
| workload.googleapis.com/elasticsearch.node.documents                  | Gauge (INT64)      | {documents}   | state                   | The number of documents on the node.                                                     |
| workload.googleapis.com/elasticsearch.node.open_files                 | Gauge (INT64)      | {files}       |                         | The number of open file descriptors held by the node.                                    |

Labels:
| Metric Name                                                           | Label Name       | Description                    | Values                                                                           |
|-----------------------------------------------------------------------|------------------|--------------------------------|----------------------------------------------------------------------------------|
| workload.googleapis.com/elasticsearch.node.cache.memory.usage         | cache_name       | The name of cache.             | fielddata, query                                                                 |
| workload.googleapis.com/elasticsearch.node.cache.evictions            | cache_name       | The name of cache.             | fielddata, query                                                                 |
| workload.googleapis.com/elasticsearch.node.cluster.io                 | direction        | The direction of network data. | received, sent                                                                   |
| workload.googleapis.com/elasticsearch.node.operations.completed       | operation        | The type of operation.         | index, delete, get, query, fetch, scroll, suggest, merge, refresh, flush, warmer |
| workload.googleapis.com/elasticsearch.node.operations.time            | operation        | The type of operation.         | index, delete, get, query, fetch, scroll, suggest, merge, refresh, flush, warmer |
| workload.googleapis.com/elasticsearch.node.thread_pool.threads        | thread_pool_name | The name of the thread pool.   |                                                                                  |
| workload.googleapis.com/elasticsearch.node.thread_pool.tasks.queued   | thread_pool_name | The name of the thread pool.   |                                                                                  |
| workload.googleapis.com/elasticsearch.node.thread_pool.tasks.finished | thread_pool_name | The name of the thread pool.   |                                                                                  |
| workload.googleapis.com/elasticsearch.node.thread_pool.threads        | state            | The state of the thread.       | active, idle                                                                     |
| workload.googleapis.com/elasticsearch.node.thread_pool.tasks.finished | state            | The state of the task.         | rejected, completed                                                              |
| workload.googleapis.com/elasticsearch.node.documents                  | state            | The state of the document      | active, deleted                                                                  |

If `collect_jvm_metrics` is true, the following JVM metrics are collected:

| Metric                                               | Data Type          | Unit | Labels | Description                                                                   |
|------------------------------------------------------|--------------------|------|--------|-------------------------------------------------------------------------------|
| workload.googleapis.com/jvm.classes.loaded           | Gauge (INT64)      | 1    |        | The number of loaded classes                                                  |
| workload.googleapis.com/jvm.gc.collections.count     | Cumulative (INT64) | 1    | name   | The total number of garbage collections that have occurred                    |
| workload.googleapis.com/jvm.gc.collections.elapsed   | Cumulative (INT64) | ms   | name   | The approximate accumulated collection elapsed time                           |
| workload.googleapis.com/jvm.memory.heap.max          | Gauge (INT64)      | By   |        | The maximum amount of memory can be used for the heap                         |
| workload.googleapis.com/jvm.memory.heap.used         | Gauge (INT64)      | By   |        | The current heap memory usage                                                 |
| workload.googleapis.com/jvm.memory.heap.committed    | Gauge (INT64)      | By   |        | The amount of memory that is guaranteed to be available for the heap          |
| workload.googleapis.com/jvm.memory.nonheap.used      | Gauge (INT64)      | By   |        | The current non-heap memory usage                                             |
| workload.googleapis.com/jvm.memory.nonheap.committed | Gauge (INT64)      | By   |        | The amount of memory that is guaranteed to be available for non-heap purposes |
| workload.googleapis.com/jvm.memory.pool.max          | Gauge (INT64)      | By   | name   | The maximum amount of memory can be used for the memory pool                  |
| workload.googleapis.com/jvm.memory.pool.used         | Gauge (INT64)      | By   | name   | The current memory pool memory usage                                          |
| workload.googleapis.com/jvm.threads.count            | Gauge (INT64)      | 1    |        | The current number of threads                                                 |

Labels:
| Metric Name                                        | Label Name | Description                        | Values |
|----------------------------------------------------|------------|------------------------------------|--------|
| workload.googleapis.com/jvm.gc.collections.count   | name       | The name of the garbage collector. |        |
| workload.googleapis.com/jvm.gc.collections.elapsed | name       | The name of the garbage collector. |        |
| workload.googleapis.com/jvm.memory.pool.max        | name       | The name of the JVM memory pool.   |        |
| workload.googleapis.com/jvm.memory.pool.used       | name       | The name of the JVM memory pool.   |        |


If `collect_cluster_metrics` is true, the following cluster-level metrics are collected:
| Metric                                                   | Data Type     | Unit     | Labels | Description                                                                                                                                                                                    |
|----------------------------------------------------------|---------------|----------|--------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| workload.googleapis.com/elasticsearch.cluster.shards     | Gauge (INT64) | {shards} | state  | The number of shards in the cluster.                                                                                                                                                           |
| workload.googleapis.com/elasticsearch.cluster.data_nodes | Gauge (INT64) | {nodes}  |        | The number of data nodes in the cluster.                                                                                                                                                       |
| workload.googleapis.com/elasticsearch.cluster.nodes      | Gauge (INT64) | {nodes}  |        | The total number of nodes in the cluster.                                                                                                                                                      |
| workload.googleapis.com/elasticsearch.cluster.health     | Gauge (INT64) | {status} | status | The health status of the cluster. See [the Elasticsearch docs](https://www.elastic.co/guide/en/elasticsearch/reference/7.16/cluster-health.html#cluster-health-api-desc) for more information. |

Labels:
| Metric Name                                          | Label Name | Description                       | Values                                       |
|------------------------------------------------------|------------|-----------------------------------|----------------------------------------------|
| workload.googleapis.com/elasticsearch.cluster.shards | state      | The state of the shard.           | active, relocating, initializing, unassigned |
| workload.googleapis.com/elasticsearch.cluster.health | status     | The health status of the cluster. | green, yellow, red                           |

# `elasticsearch_json` and `elasticsearch_gc` Logging Receivers

Follow [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/elasticsearch)
for instructions to collect logs and metrics from this application using Ops Agent.

## Logs

JSON logs commonly contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field                      | Type                                                                                                                            | Description                                                                                                                 |
|----------------------------|---------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------|
| `jsonPayload.component`    | string                                                                                                                          | The component of Elasticsearch that emitted the log                                                                         |
| `jsonPayload.type`         | string                                                                                                                          | The type of log, indicating which log the record came from (e.g. `server` indicates this LogEntry came from the server log) |
| `jsonPayload.cluster.name` | string                                                                                                                          | The name of the cluster emitting the log record                                                                             |
| `jsonPayload.cluster.uuid` | string                                                                                                                          | The uuid of the cluster emitting the log record                                                                             |
| `jsonPayload.node.name`    | string                                                                                                                          | The name of the node emitting the log record                                                                                |
| `jsonPayload.node.uuid`    | string                                                                                                                          | The uuid of the node emitting the log record                                                                                |
| `jsonPayload.message`      | string                                                                                                                          | Log message                                                                                                                 |
| `severity`                 | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity))                       | Log entry level (translated)                                                                                                |
| `timestamp`                | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged                                                                                                   |

If the fields are absent on the original log record, the will not be present on the resultant LogEntry.

GC logs may contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field                 | Type                                                                                                                            | Description                      |
|-----------------------|---------------------------------------------------------------------------------------------------------------------------------|----------------------------------|
| `jsonPayload.gc_run`  | number                                                                                                                          | The run of the garbage collector |
| `jsonPayload.message` | string                                                                                                                          | Log message                      |
| `jsonPayload.type`    | string                                                                                                                          | The type of the log record       |
| `timestamp`           | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged        |

If the fields are absent on the original log record, the will not be present on the resultant LogEntry.
