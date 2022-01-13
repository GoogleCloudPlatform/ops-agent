# `memcached` Metrics Receiver

The memcached receiver can retrieve stats from your memcached server through the . 


## Configuration

Following the guide for [Configuring the Ops Agent](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration#file-location), add the required elements for your memcached configuration.

To configure a receiver for your memcached metrics, specify the following fields:

| Field                 | Default                   | Description |
| ---                   | ---                       | ---         |
| `type`                | required                  | Must be `memcached`. |
| `endpoint`            | `localhost:3306`          | The url, or unix socket file path, for your memcached server. |
| `collection_interval` | `60s`                     | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |

Example Configuration:

```yaml
metrics:
  receivers:
    memcached_metrics:
      type: memcached
      collection_interval: 60s
  service:
    pipelines:
      memcached_pipeline:
        receivers:
          - memcached_metrics
```


## Metrics

These are the metrics available for this scraper.

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

## Attributes

| Name | Description | Values |
| ---- | ----------- | ------ |
| command | The type of command. | <ul> <li>get</li> <li>set</li> <li>flush</li> <li>touch</li> </ul>
| direction | Direction of data flow. | <ul> <li>sent</li> <li>received</li> </ul>
| operation | The type of operation. | <ul> <li>increment</li> <li>decrement</li> <li>get</li> </ul> |
| state | The type of CPU usage. | <ul> <li>system</li> <li>user</li> </ul> |
| type | Result of cache request. | <ul> <li>hit</li> <li>miss</li> </ul> |

# `memcached` Logging Receiver

Memcached logs are collected by the default syslog receiver.