# `memcached` Metrics Receiver

The memcached receiver can retrieve stats from your memcached server through the . 


## Configuration

Following the guide for [Configuring the Ops Agent](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration#file-location), add the required elements for your memcached configuration.

To configure a receiver for your memcached metrics, specify the following fields:

| Field                 | Default                   | Description |
| ---                   | ---                       | ---         |
| `type`                | required                  | Must be `memcached`. |
| `endpoint`            | `localhost:6379`          | The url, or unix socket filepath, for your memcached server |
| `collection_interval` | `60s`                     | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |

Example Configuration:

```yaml
metrics:
  receivers:
    redis_metrics:
      type: memcached
      address: localhost:6379
      collection_interval: 30s
  service:
    pipelines:
      memcached_pipeline:
        receivers:
          - memcached_metrics
```

## Metrics

The Ops Agent collects the following metrics from your memcached servers.

| Name | Description | Unit | Type | Attributes |
| ---- | ----------- | ---- | ---- | ---------- |
| memcached.bytes | Current number of bytes used by this server to store items. | By | Gauge | <ul> </ul> |
| memcached.commands | Commands executed. | 1 | Sum | <ul> <li>command</li> </ul> |
| memcached.current_connections | The current number of open connections. | connections | Sum | <ul> </ul> |
| memcached.current_items | Number of items currently stored in the cache. | 1 | Sum | <ul> </ul> |
| memcached.evictions | Cache item evictions. | 1 | Sum | <ul> </ul> |
| memcached.network | Bytes transferred over the network. | by | Sum | <ul> <li>direction</li> </ul> |
| memcached.operation_hit_ratio | Hit ratio for operations, expressed as a percentage value between 0.0 and 100.0. | % | Gauge | <ul> <li>operation</li> </ul> |
| memcached.operations | Operation counts. | 1 | Sum | <ul> <li>type</li> <li>operation</li> </ul> |
| memcached.rusage | Accumulated user and system time. | 1 | Sum | <ul> <li>state</li> </ul> |
| memcached.threads | Number of threads used by the memcached instance. | 1 | Sum | <ul> </ul> |
| memcached.total_connections | Total number of connections opened since the server started running. | connections | Sum | <ul> </ul> |
        
# `memcached` Logging Receiver

Memcached logs are collected by the default syslog receiver.