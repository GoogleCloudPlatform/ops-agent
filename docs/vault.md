# Elasticsearch
# `elasticsearch` Metrics Receiver

The `elasticsearch` metrics receiver fetches node and cluster level stats from Elasticsearch nodes. The receiver is meant to be run 

## Configurations
To configure a receiver for your Vault metrics, specify the following fields:

| Field                   | Required | Default           | Description |
| ---                     | ---      | ---               | ---         |
| `type`                  | required |                   | Must be `vault`. |
| `endpoint`              | optional | `localhost:8200`  | hostname:port of vault instance to be monitored. |
| `metrics_path`          | optional | `/v1/sys/metrics` | the path for metrics collection. |
| `token`                 | optional |                   | Token used for authentication. |
| `collection_interval`   | optional |                   | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |
| `insecure`              | optional | true              | Signals whether to use a secure TLS connection or not. If insecure is true TLS will not be enabled. |
| `insecure_skip_verify`  | optional | false             | Whether to skip verifying the certificate or not. A false value of insecure_skip_verify will not be used if insecure is true as the connection will not use TLS at all. |
| `cert_file`             | optional |                   | Path to the TLS cert to use for mTLS required connections. |
| `key_file`              | optional |                   | Path to the TLS key to use for mTLS required connections. |
| `ca_file`               | optional |                   | Path to the CA cert. As a client this verifies the server certificate. If empty, uses system root CA. |


Example Configuration:

```yaml
metrics:
  receivers:
    elasticsearch:
      type: vault
  service:
    pipelines:
      vault:
        receivers:
          - vault
```

## Metrics

The Ops Agent collects the following metrics from your Elasticsearch nodes:

| Metric                                        | Data Type          | Unit          | Labels                  | Description                                                                              |
|-----------------------------------------------|--------------------|---------------|-------------------------|------------------------------------------------------------------------------------------|
| workload.googleapis.com/vault.node.cache.memory.usage              | Gauge (INT64)      | By            | cache_name              | The size in bytes of the cache.                                     |
| workload.googleapis.com/vault.node.cache.evictions                 | Cumulative (INT64) | {evictions}   | cache_name              | The number of evictions from the cache.                             |


Labels:

| Metric Name                                        | Label Name | Description                        | Values |
|----------------------------------------------------|------------|------------------------------------|--------|
| workload.googleapis.com/jvm.gc.collections.count   | name       | The name of the garbage collector. |        |
| workload.googleapis.com/jvm.gc.collections.elapsed | name       | The name of the garbage collector. |        |
| workload.googleapis.com/jvm.memory.pool.max        | name       | The name of the JVM memory pool.   |        |
| workload.googleapis.com/jvm.memory.pool.used       | name       | The name of the JVM memory pool.   |        |