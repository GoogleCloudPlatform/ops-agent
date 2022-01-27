# Memcache

Follow https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/memcached
for instructions to collect logs and metrics from this application using Ops Agent.

## Metrics

The following table provides the list of metrics that the Ops Agent collects from this application.

| Name | Description | Unit | Type | Attributes |
| ---- | ----------- | ---- | ---- | ---------- |
| memcached.bytes | Current number of bytes used by this server to store items. | By | Gauge | <ul> </ul> |
| memcached.commands | Commands executed. | 1 | Sum | <ul> <li>command</li> </ul> |
| memcached.connections.current | The current number of open connections. | connections | Gauge | <ul> </ul> |
| memcached.connections.total | Total number of connections opened since the server started running. | connections | Sum | <ul> </ul> |
| memcached.cpu.usage | Accumulated user and system time. | 1 | Sum | <ul> <li>state</li> </ul> |
| memcached.current_items | Number of items currently stored in the cache. | 1 | Gauge | <ul> </ul> |
| memcached.evictions | Cache item evictions. | 1 | Sum | <ul> </ul> |
| memcached.network | Bytes transferred over the network. | by | Sum | <ul> <li>direction</li> </ul> |
| memcached.operations | Operation counts. | 1 | Sum | <ul> <li>type</li> <li>operation</li> </ul> |
| memcached.threads | Number of threads used by the memcached instance. | 1 | Gauge | <ul> </ul> |

### Metrics labels

| Name | Description | Values |
| ---- | ----------- | ------ |
| command | The type of command. | <ul> <li>get</li> <li>set</li> <li>flush</li> <li>touch</li> </ul>
| direction | Direction of data flow. | <ul> <li>sent</li> <li>received</li> </ul>
| operation | The type of operation. | <ul> <li>increment</li> <li>decrement</li> <li>get</li> </ul> |
| state | The type of CPU usage. | <ul> <li>system</li> <li>user</li> </ul> |
| type | Result of cache request. | <ul> <li>hit</li> <li>miss</li> </ul> |
