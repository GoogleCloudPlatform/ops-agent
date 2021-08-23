# Nginx Receiver

The `nginx` receiver can fetch stats from a Nginx instance using the mod_status endpoint.


## Prerequisites

The receiver requires that you enable the [status module](http://nginx.org/en/docs/http/ngx_http_stub_status_module.html) on your Nginx instance.

You can enable the status module by adding the following to your Nginx configuration:

```
location /status {
    stub_status on;
}
```

This can be done by including the above in a `status.conf` file in the Nginx configuration directory (normally `/etc/nginx/conf.d`).

Alternately, you can append to your `nginx.conf`, normally located in one of the following directories: `/etc/nginx`, `/usr/local/nginx/conf`, or `/usr/local/etc/nginx`.

In the context of other configuration setting, this addition might look something like the following:
```
server {
    listen 80;
    server_name mynginx.domain.com;
    location /status {
        stub_status on;
    }
    location / {
        root /dev/null;
    }
}
```

Reload the Nginx configuration by running: `sudo service nginx reload`


## Configuration

| Field                 | Default                   | Description |
| ---                   | ---                       | ---         |
| `type`                | required                  | Must be `nginx`. |
| `endpoint`            | `http://localhost/status` | The url exposed by the Nginx stats module. |
| `collection_interval` | `60s`                     | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |

Example Configuration:

```yaml
metrics:
  receivers:
    nginx_metrics:
      type: nginx
      endpoint: http://localhost:80/status
      collection_interval: 30s
  service:
    pipelines:
      nginxpipeline:
        receivers:
          - nginx_metrics
```

## Metrics

| Metric                                           | Data Type | Unit        | Labels | Description |
| ---                                              | ---       | ---         | ---    | ---         | 
| custom.googleapis.com/nginx.requests             | sum       | requests    |        | Total number of requests made to the server. |
| custom.googleapis.com/nginx.connections_accepted | sum       | connections |        | Total number of accepted client connections. |
| custom.googleapis.com/nginx.connections_handled  | sum       | connections |        | Total number of handled connections. |
| custom.googleapis.com/nginx.connections_current  | gauge     | connections | state  | Current number of connections. |