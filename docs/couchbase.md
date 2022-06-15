# Couchbase

Supported telemetry types: metrics

## Metrics

The couchbase integration uses the builtin [prometheus exporter](https://docs.couchbase.com/cloud-native-database/prometheus-overview.html) running on Couchbase 7.0 by default. The metrics are retrieved from this endpoint and then will be transformed to be ingested by Google Cloud.

## Configuration

| Field                 | Default            | Description |
| ---                   | ---                | ---         |
| `type`                | required           | Must be `couchbase`. |
| `endpoint`            | `localhost:8091`  | The address of the couchbase node that exposes the prometheus exporter metrics endpoint. |
| `username`            | required | The configured username to authenticate to couchbase. |
| `password`            | required | The configured password to authenticate to couchbase. |
| `collection_interval` | `60s`              | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |

Example Configuration:

```yaml
metrics:
  receivers:
    couchbase:
      type: couchbase
      username: opsuser
      password: password
  service:
    pipelines:
      couchbase:
        receivers:
          - couchbase
```
