# Redis

Follow [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/redis)
for instructions to collect logs and metrics from this application using Ops Agent.

## Metrics

The Ops Agent collects the following metrics from your redis servers.

| Metric                                                              | Data Type | Unit    | Labels | Description |
| ---                                                                 | ---       | ---     | ---    | ---         |
| workload.googleapis.com/redis.cpu.time                              | sum       | s       | state  | System CPU consumed by the Redis server in seconds since server start |
| workload.googleapis.com/redis.clients.connected                     | gauge     | 1       |        | Number of client connections (excluding connections from replicas) |
| workload.googleapis.com/redis.clients.max_input_buffer              | gauge     | 1       |        | Biggest input buffer among current client connections |
| workload.googleapis.com/redis.clients.max_output_buffer             | gauge     | 1       |        | Longest output list among current client connections |
| workload.googleapis.com/redis.clients.blocked                       | gauge     | 1       |        | Number of clients pending on a blocking call |
| workload.googleapis.com/redis.keys.expired                          | sum       | 1       |        | Total number of key expiration events |
| workload.googleapis.com/redis.keys.evicted                          | sum       | 1       |        | Number of evicted keys due to maxmemory limit |
| workload.googleapis.com/redis.connections.received                  | sum       | 1       |        | Total number of connections accepted by the server |
| workload.googleapis.com/redis.connections.rejected                  | sum       | 1       |        | Number of connections rejected because of maxclients limit |
| workload.googleapis.com/redis.memory.used                           | gauge     | by      |        | Total number of bytes allocated by Redis using its allocator |
| workload.googleapis.com/redis.memory.peak                           | gauge     | by      |        | Peak memory consumed by Redis (in bytes) |
| workload.googleapis.com/redis.memory.rss                            | gauge     | by      |        | Number of bytes that Redis allocated as seen by the operating system |
| workload.googleapis.com/redis.memory.lua                            | gauge     | by      |        | Number of bytes used by the Lua engine |
| workload.googleapis.com/redis.memory.fragmentation_ratio            | gauge     | 1       |        | Ratio between used_memory_rss and used_memory |
| workload.googleapis.com/redis.rdb.changes_since_last_save           | gauge     | 1       |        | Number of changes since the last dump |
| workload.googleapis.com/redis.commands                              | gauge     | {ops}/s |        | Number of commands processed per second |
| workload.googleapis.com/redis.commands.processed                    | sum       | 1       |        | Total number of commands processed by the server |
| workload.googleapis.com/redis.net.input                             | sum       | by      |        | The total number of bytes read from the network |
| workload.googleapis.com/redis.net.output                            | sum       | by      |        | The total number of bytes written to the network |
| workload.googleapis.com/redis.keyspace.hits                         | sum       | 1       |        | Number of successful lookup of keys in the main dictionary |
| workload.googleapis.com/redis.keyspace.misses                       | sum       | 1       |        | Number of failed lookup of keys in the main dictionary |
| workload.googleapis.com/redis.latest_fork                           | guage     | us      |        | Duration of the latest fork operation in microseconds |
| workload.googleapis.com/redis.slaves.connected                      | gauge     | 1       |        | Number of connected replicas |
| workload.googleapis.com/redis.replication.backlog_first_byte_offset | gauge     | 1       |        | The master offset of the replication backlog buffer |
| workload.googleapis.com/redis.replication.offset                    | gauge     | 1       |        | The server's current replication o

## Logs
<!-- TODO: Add these config options to public docs -->
<!-- 
insecure	            true	Sets whether or not to use a secure TLS connection. If set to false, then TLS is enabled.
insecure_skip_verify	false	Sets whether or not to skip verifying the certificate. If insecure is set to true, then the insecure_skip_verify value is not used.
cert_file		                Path to the TLS certificate to use for TLS-required connections.
key_file		                Path to the TLS key to use for TLS-required connections.
ca_file		                    Path to the CA certificate. As a client, this verifies the server certificate. If empty, the receiver uses the system root CA. -->


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
