# `redis` Metrics Receiver

The redis receiver can retrieve stats from your redis server through the [INFO](https://redis.io/commands/info) command. 


## Configuration

Following the guide for [Configuring the Ops Agent](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration#file-location), add the required elements for your redis configuration.

To configure a receiver for your redis metrics, specify the following fields:

| Field                 | Default                   | Description |
| ---                   | ---                       | ---         |
| `type`                | required                  | Must be `redis`. |
| `endpoint`            | `localhost:6379`          | The url exposed by redis |
| `collection_interval` | `60s`                     | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |
| `password`            |                           | The password used to connect to the server.

Example Configuration:

```yaml
metrics:
  receivers:
    redis_metrics:
      type: redis
      endpoint: localhost:6379
      collection_interval: 30s
      password: pwd
  service:
    pipelines:
      redis_pipeline:
        receivers:
          - redis_metrics
```

## Metrics

The Ops Agent collects the following metrics from your redis servers.

| Metric                                | Data Type | Unit | Labels  | Description    |
| ---                                   | ---       | ---  | ---     | ---            | 
| workload.googleapis.com/redis/uptime	                              | sum	  | s		   |         | Number of seconds since Redis server start |
| workload.googleapis.com/redis/cpu/time	                            | sum	  | s	       | state:  | System CPU consumed by the Redis server in seconds since server start |
| workload.googleapis.com/redis/clients/connected	                    | sum	  | 1		   |         | Number of client connections (excluding connections from replicas) |
| workload.googleapis.com/redis/clients/max_input_buffer	            | gauge	| 1		   |         | Biggest input buffer among current client connections |
| workload.googleapis.com/redis/clients/max_output_buffer	            | gauge	| 1		   |         | Longest output list among current client connections |
| workload.googleapis.com/redis/clients/blocked	                      | sum	  | 1		   |         | Number of clients pending on a blocking call |
| workload.googleapis.com/redis/keys/expired	                        | sum	  | 1		   |         | Total number of key expiration events |
| workload.googleapis.com/redis/keys/evicted	                        | sum	  | 1		   |         | Number of evicted keys due to maxmemory limit |
| workload.googleapis.com/redis/connections/received	                | sum	  | 1		   |         | Total number of connections accepted by the server |
| workload.googleapis.com/redis/connections/rejected	                | sum	  | 1		   |         | Number of connections rejected because of maxclients limit |
| workload.googleapis.com/redis/memory/used	                          | gauge	| by	   |         | Total number of bytes allocated by Redis using its allocator |
| workload.googleapis.com/redis/memory/peak	                          | gauge	| by	   |         | Peak memory consumed by Redis (in bytes) |
| workload.googleapis.com/redis/memory/rss	                          | gauge	| by	   |         | Number of bytes that Redis allocated as seen by the operating system |
| workload.googleapis.com/redis/memory/lua	                          | gauge	| by	   |         | Number of bytes used by the Lua engine |
| workload.googleapis.com/redis/memory/fragmentation_ratio	          | gauge	| 1		   |         | Ratio between used_memory_rss and used_memory |
| workload.googleapis.com/redis/rdb/changes_since_last_save	          | sum	  | 1		   |         | Number of changes since the last dump |
| workload.googleapis.com/redis/commands	                            | gauge	| {ops}/s  |         | Number of commands processed per second |
| workload.googleapis.com/redis/commands/processed	                  | sum	  | 1		   |         | Total number of commands processed by the server |
| workload.googleapis.com/redis/net/input	                            | sum	  | by	   |         | The total number of bytes read from the network |
| workload.googleapis.com/redis/net/output	                          | sum	  | by	   |         | The total number of bytes written to the network |
| workload.googleapis.com/redis/keyspace/hits	                        | sum	  | 1		   |         | Number of successful lookup of keys in the main dictionary |
| workload.googleapis.com/redis/keyspace/misses	                      | sum	  | 1	       |         | Number of failed lookup of keys in the main dictionary |
| workload.googleapis.com/redis/latest_fork	                          | guage	| us	   |         | Duration of the latest fork operation in microseconds |
| workload.googleapis.com/redis/slaves/connected	                    | sum	  | 1		   |         | Number of connected replicas |
| workload.googleapis.com/redis/replication/backlog_first_byte_offset	| gauge	| 1		   |         | The master offset of the replication backlog buffer |
| workload.googleapis.com/redis/replication/offset	                  | gauge	| 1		   |         | The server's current replication offset |
				
        
# `redis` Logging Receiver

## Configuration

To configure a receiver for your redis logs, specify the following fields:

| Field                 | Default                       | Description |
| ---                   | ---                           | ---         |
| `type`                | required                      | Must be `redis`. |
| `include_paths`       | `[/var/log/redis/redis-server.log, /var/log/redis_6379.log, /var/log/redis/redis.log, /var/log/redis/default.log, /var/log/redis/redis_6379.log]` | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `/var/log/redis/*.log`.
| `exclude_paths`       | `[]`                          | A list of filesystem path patterns to exclude from the set matched by `include_paths`.


Example Configuration:

```yaml
logging:
  receivers:
    redis_default:
      type: redis
  service:
    pipelines:
      apache:
        receivers:
        - redis_default
```

## Logs

Redis logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.roleChar` | string | redis role character (X, C, S, M) |
| `jsonPayload.role` | string | translated from redis role character (sentinel, RDB/AOF_writing_child, slave, master) |
| `jsonPayload.level` | string | Log entry level |
| `jsonPayload.pid` | number | Process ID |
| `jsonPayload.message` | string | Log message |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged |