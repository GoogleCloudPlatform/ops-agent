# Nginx Receiver

The `nginx` receiver can fetch stats from a NGINX instance using the mod_status endpoint.


## Prerequisites

[ngx_http_stub_status_module](http://nginx.org/en/docs/http/ngx_http_stub_status_module.html) must be enabled on the NGINX instance. This may require editing the NGINX configuration.


## Configuration

| Field                 | Default                   | Description |
| ---                   | ---                       | ---         |
| `type`                | required                  | Must be `nginx`. |
| `endpoint`            | `http://localhost/status` | The url exposed by the NGINX stats module. |
| `collection_interval` | `60s`                     | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |

Example Configuration:

```yaml
metrics:
  receivers:
    mynginxmetrics:
      type: nginx
      endpoint: http://localhost:80/status
      collection_interval: 30s
  service:
    pipelines:
      nginxpipeline:
        receivers:
          - nginxmetrics
```

## Metrics

| Metric                     | Data Type | Unit        | Labels | Description |
| ---                        | ---       | ---         | ---    | ---         | 
| nginx.requests             | sum       | requests    |        | Total number of requests made to the server. |
| nginx.connections_accepted | sum       | connections |        | Total number of accepted client connections. |
| nginx.connections_handled  | sum       | connections |        | Total number of handled connections. |
| nginx.connections_current  | gauge     | connections | state  | Current number of connections. |