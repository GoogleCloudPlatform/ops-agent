# `nginx` Metrics Receiver

The nginx receiver can retrieve stats from your nginx instance using the `mod_status` endpoint.


## Prerequisites

The receiver requires that you enable the [status module](http://nginx.org/en/docs/http/ngx_http_stub_status_module.html) on your nginx instance.

To enable the status module, complete the following steps:

1. Edit the `status.conf` file. You can find this file in the nginx configuration directory, typically found at `/etc/nginx/conf.d`.
2. Add the following lines to your configuration file:

   ```
   location /status {
       stub_status on;
   }
   ```

    1. Alternately, you can append these lines to your `nginx.conf` file, which is typically located in one of the following directories: `/etc/nginx`, `/usr/local/nginx/conf`, or `/usr/local/etc/nginx`.

   Your configuration file might look like the following example:
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

3. Reload the nginx configuration:

   ```
   sudo service nginx reload
   ```


## Configuration

Following the guide for [Configuring the Ops Agent](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration#file-location), add the required elements for your nginx configuration.

To configure a receiver for your nginx metrics, specify the following fields:

| Field                 | Default                   | Description |
| ---                   | ---                       | ---         |
| `type`                | required                  | Must be `nginx`. |
| `stub_status_url`     | `http://localhost/status` | The url exposed by the nginx stats module. |
| `collection_interval` | `60s`                     | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |

Example Configuration:

```yaml
metrics:
  receivers:
    nginx_metrics:
      type: nginx
      stub_status_url: http://localhost:80/status
      collection_interval: 30s
  service:
    pipelines:
      nginx_pipeline:
        receivers:
          - nginx_metrics
```

## Metrics

The Ops Agent collects the following metrics from your nginx instances.

| Metric                                           | Data Type | Unit        | Labels | Description |
| ---                                              | ---       | ---         | ---    | ---         | 
| workload.googleapis.com/nginx.requests             | cumulative       | requests    |        | Total number of requests made to the server. |
| workload.googleapis.com/nginx.connections_accepted | cumulative       | connections |        | Total number of accepted client connections. |
| workload.googleapis.com/nginx.connections_handled  | cumulative       | connections |        | Total number of handled connections. |
| workload.googleapis.com/nginx.connections_current  | gauge     | connections | state  | Current number of connections. |

# `nginx_access` and `nginx_error` Logging Receivers

## Configuration

To configure a receiver for your nginx access logs, specify the following fields:

| Field                 | Default                       | Description |
| ---                   | ---                           | ---         |
| `type`                | required                      | Must be `nginx_access`. |
| `include_paths`       | `[/var/log/nginx/access.log]` | The log files to read. |
| `exclude_paths`       | `[]`                          | Log files to exclude (if `include_paths` contains a glob or directory). |

To configure a receiver for your nginx error logs, specify the following fields:

| Field                 | Default                      | Description |
| ---                   | ---                          | ---         |
| `type`                | required                     | Must be `nginx_error`. |
| `include_paths`       | `[/var/log/nginx/error.log]` | The log files to read. |
| `exclude_paths`       | `[]`                         | Log files to exclude (if `include_paths` contains a glob or directory). |

Example Configuration:

```yaml
logging:
  receivers:
    nginx_default_access:
      type: nginx_access
    nginx_default_error:
      type: nginx_error
  service:
    pipelines:
      nginx:
        receivers:
          - nginx_default_access
          - nginx_default_error
```

## Logs

Access logs contain the [`httpRequest` field](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#httprequest):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `httpRequest.protocol` | string | Protocol used for the request |
| `httpRequest.referer` | string | Contents of the `Referer` header |
| `httpRequest.remoteIp` | string | Client IP address |
| `httpRequest.requestMethod` | string | HTTP method |
| `httpRequest.requestUrl` | string | Request URL (typically just the path part of the URL) |
| `httpRequest.responseSize` | string (`int64`) | Response size |
| `httpRequest.status` | number | HTTP status code |
| `httpRequest.userAgent` | string | Contents of the `User-Agent` header |
| `jsonPayload.host` | string | Contents of the `Host` header (usually not reported by nginx) |
| `jsonPayload.user` | string | Authenticated username for the request |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received |

Any fields that are blank or missing will not be present in the log entry.

Error logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.level` | string | Log entry level |
| `jsonPayload.pid` | number | Process ID |
| `jsonPayload.tid` | number | Thread ID |
| `jsonPayload.connection` | number | Connection ID |
| `jsonPayload.message` | string | Log message |
| `jsonPayload.client` | string | Client IP address (optional) |
| `jsonPayload.server` | string | Nginx server name (optional) |
| `jsonPayload.request` | string | Original HTTP request (optional) |
| `jsonPayload.subrequest` | string | Nginx subrequest (optional) |
| `jsonPayload.upstream` | string | Upstream request URI (optional) |
| `jsonPayload.host` | string | Host header (optional) |
| `jsonPayload.referer` | string | Referer header (optional) |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged |
