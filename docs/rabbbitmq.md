# `rabbitmq` Metrics Receiver

This receiver fetches stats from a RabbitMQ node using the [RabbitMQ Management Plugin](https://www.rabbitmq.com/management.html).

## Prerequisites

This receiver supports RabbitMQ versions `3.8` and `3.9`.

The RabbitMQ Management Plugin must be enabled by following the [official instructions](https://www.rabbitmq.com/management.html#getting-started).

Also, a user with at least [monitoring](https://www.rabbitmq.com/management.html#permissions) level permissions must be used for monitoring.
## Configuration

Following the guide for [Configuring the Ops Agent](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration#file-location), add the required elements for your rabbitmq configuration.

To configure a receiver for your rabbitmq metrics, specify the following fields:

| Field                   | Required | Default                         | Description |
| ---                     | ---      | ---                             | ---         |
| `type`                  | required |                                 | Must be `rabbitmq`. |
| `endpoint`              | optional | `http://localhost:15672`        | URL of node to be monitored |
| `collection_interval`   | required |                                 | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |
| `username`              | required |                                 | The username used to connect to the server. |
| `password`              | required |                                 | The password used to connect to the server. |
| `insecure`              | optional | true                            | Signals whether to use a secure TLS connection or not. If insecure is true TLS will not be enabled. |
| `insecure_skip_verify`  | optional |                                 | Whether to skip verifying the certificate or not. A false value of insecure_skip_verify will not be used if insecure is true as the connection will not use TLS at all. |
| `cert_file`             | optional |                                 | Path to the TLS cert to use for TLS required connections. |
| `key_file`              | optional |                                 | Path to the TLS key to use for TLS required connections. |
| `ca_file`               | optional |                                 | Path to the CA cert. As a client this verifies the server certificate. If empty, uses system root CA. |

Example Configuration:


```yaml
metrics:
  receivers:
    rabbitmq:
      type: rabbitmq 
      endpoint: http://localhost:15672
      password: pwd
      username: usr
  service:
    pipelines:
      rabbitmq:
        receivers:
          - rabbitmq
```

TCP connection with a username and password and TLS:

```yaml
metrics:
  receivers:
    rabbitmq:
      type: rabbitmq 
      endpoint: http://localhost:15672
      collection_interval: 60s
      password: pwd
      username: usr
      insecure: false
      insecure_skip_verify: false
      cert_file: /path/to/cert
      ca_file: /path/to/ca
  service:
    pipelines:
      rabbitmq:
        receivers:
          - rabbitmq
```

## Metrics

The Ops Agent collects the following metrics from your rabbitmq instances.

| Metric                                                 | Data Type | Unit        | Labels                          | Description    |
| ---                                                    | ---       | ---         | ---                             | ---            | git 
| rabbitmq.consumer.count | gauge | {consumers} | <ul> </ul>  | The number of consumers currently reading from the queue. |
| rabbitmq.message.acknowledged | cumulative | {messages} | <ul> </ul>  | The number of messages acknowledged by consumers. |
| rabbitmq.message.current | gauge | {messages} | <ul> <li>state</li> </ul>  | The total number of messages currently in the queue. |
| rabbitmq.message.delivered | cumulative | {messages} | <ul> </ul>  | The number of messages delivered to consumers. |
| rabbitmq.message.dropped | cumulative | {messages} | <ul> </ul>  | The number of messages dropped as unroutable. |
| rabbitmq.message.published | cumulative | {messages} | <ul> </ul>  | The number of messages published to a queue. |

## Labels

| Name | Description |
| ---- | ----------- |
| state | The state of messages in a queue. |