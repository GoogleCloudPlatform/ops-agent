# `postgresql` Metrics Receiver

The postgresql receiver can retrieve stats from your postgresql instance by connecting as a monitoring user.

## Prerequisites

The `postgresql` receiver defaults to connecting to a local postgresql server using a Unix socket and Unix authentication as the `root` user.

## Configuration

Following the guide for [Configuring the Ops Agent](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration#file-location), add the required elements for your postgresql configuration.

To configure a receiver for your postgresql metrics, specify the following fields:

| Field                   | Required | Default                         | Description |
| ---                     | ---      | ---                             | ---         |
| `type`                  | required |                      | Must be `postgresql`. |
| `endpoint`              | optional | `/var/run/postgresql/.s.PGSQL.5432`   | The hostname:port or socket path starting with `/` used to connect to postgresql |
| `collection_interval`   | required |                                 | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |
| `username`              | optional |                                 | The username used to connect to the server. |
| `password`              | optional |                                 | The password used to connect to the server. |
| `insecure`              | optional | true                            | Signals whether to use a secure TLS connection or not. If insecure is true TLS will not be enabled. |
| `insecure_skip_verify`  | optional | false                           | Whether to skip verifying the certificate or not. A false value of insecure_skip_verify will not be used if insecure is true as the connection will not use TLS at all. |
| `cert_file`             | optional |                             | Path to the TLS cert to use for TLS required connections. |
| `key_file`              | optional |                             | Path to the TLS key to use for TLS required connections. |
| `ca_file`               | optional |                             | Path to the CA cert. As a client this verifies the server certificate. If empty, uses system root CA. |

Example Configuration:


```yaml
metrics:
  receivers:
    postgresql_metrics:
      type: postgresql 
      endpoint: localhost:3306
      collection_interval: 60s
      password: pwd
      username: usr
  service:
    pipelines:
      postgresql_pipeline:
        receivers:
          - postgresql_metrics
```

TCP connection with a username and password and TLS:

```yaml
metrics:
  receivers:
    postgresql_metrics:
      type: postgresql 
      endpoint: localhost:3306
      collection_interval: 60s
      password: pwd
      username: usr
      insecure: false
      insecure_skip_verify: false
      cert_file: /path/to/cert
      ca_file: /path/to/ca
  service:
    pipelines:
      postgresql_pipeline:
        receivers:
          - postgresql_metrics
```

## Metrics

The Ops Agent collects the following metrics from your postgresql instances.

| Metric                                                 | Data Type | Unit        | Labels                          | Description    |
| ---                                                    | ---       | ---         | ---                             | ---            | git 
| rabbitmq.consumer.count | Sum(Int) | {consumers} | <ul> </ul>  | The number of consumers currently reading from the queue. |
| rabbitmq.message.acknowledged | Sum(Int) | {messages} | <ul> </ul>  | The number of messages acknowledged by consumers. |
| rabbitmq.message.current | Sum(Int) | {messages} | <ul> <li>message.state</li> </ul>  | The total number of messages currently in the queue. |
| rabbitmq.message.delivered | Sum(Int) | {messages} | <ul> </ul>  | The number of messages delivered to consumers. |
| rabbitmq.message.dropped | Sum(Int) | {messages} | <ul> </ul>  | The number of messages dropped as unroutable. |
| rabbitmq.message.published | Sum(Int) | {messages} | <ul> </ul>  | The number of messages published to a queue. |

## Attributes

| Name | Description |
| ---- | ----------- |
| message.state | The state of messages in a queue. |
| rabbitmq.node.name | The name of the RabbitMQ node. |
| rabbitmq.queue.name | The name of the RabbitMQ queue. |
| rabbitmq.vhost.name | The name of the RabbitMQ vHost. |