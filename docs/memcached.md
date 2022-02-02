# Memcached

Follow [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/memcached)
for instructions to collect logs and metrics from this application using Ops Agent.

## Metrics

The following table provides the list of metrics that the Ops Agent collects from this application.

| Name | Description | Unit | Type | Attributes |
| ---- | ----------- | ---- | ---- | ---------- |
| memcached.bytes | Current number of bytes used by this server to store items. | By | Gauge | |
| memcached.commands | Commands executed. | 1 | Sum | command |
| memcached.connections.current | The current number of open connections. | connections | Gauge | |
| memcached.connections.total | Total number of connections opened since the server started running. | connections | Sum | |
| memcached.cpu.usage | Accumulated user and system time. | 1 | Sum | state |
| memcached.current_items | Number of items currently stored in the cache. | 1 | Gauge | |
| memcached.evictions | Cache item evictions. | 1 | Sum | |
| memcached.network | Bytes transferred over the network. | by | Sum | direction |
| memcached.operations | Operation counts. | 1 | Sum | type, operation |
| memcached.threads | Number of threads used by the memcached instance. | 1 | Gauge | |

### Metrics labels

| Name | Description | Values |
| ---- | ----------- | ------ |
| command | The type of command. | get, set, flush, touch |
| direction | Direction of data flow. | sent, received |
| operation | The type of operation. | increment, decrement, get |
| state | The type of CPU usage. | system, user |
| type | Result of cache request. | hit, miss |
