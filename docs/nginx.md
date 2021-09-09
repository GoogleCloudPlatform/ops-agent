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
| workload.googleapis.com/nginx.requests             | sum       | requests    |        | Total number of requests made to the server. |
| workload.googleapis.com/nginx.connections_accepted | sum       | connections |        | Total number of accepted client connections. |
| workload.googleapis.com/nginx.connections_handled  | sum       | connections |        | Total number of handled connections. |
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
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Severity based on HTTP status code class ERROR (5xx), WARNING (4xx), NOTICE (3xx), INFO (2xx). |

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

## Monitoring request count by status code based on system logs based metric

Google provides some [system defined logs-based metrics](https://cloud.google.com/logging/docs/alerting/monitoring-logs#system-defined-logs-based-metrics) including
a measure of log entry count broken down by severity. Because the NGINX
access log has one entry per request with severity corresponding to the HTTP
status code class, you can use the metric 
`logging.googleapis.com/log_entry_count` with its `log` label filtered to 
`nginx_access` and grouped by `severity` to see the request count by response
class assuming you have enabled NGINX logging for your requests.

To make the metric easier to visualize you can use the
[Monitoring Query Language](https://cloud.google.com/monitoring/mql) to remap
the log severity to a more easily readable HTTP status code class, for
example by using the following query:

```
# Macro to map NGINX access log severity to HTTP status class
def severity_to_status_class(severity) =
  if($severity = 'ERROR', '5xx',
    if($severity = 'WARNING', '4xx',
      if($severity = 'NOTICE', '3xx',
        if($severity = 'INFO', '2xx', if($severity = 'DEBUG', '1xx', '-')))));

# MQL query to query the percentage of NGINX access log entries (and thus
# request, assuming one entry per requests) that are of 2xx/3xx/4xx/5xx status
# classes.
fetch gce_instance
| metric 'logging.googleapis.com/log_entry_count'
| filter metric.log = 'nginx_access'
| align rate(1m)
| group_by [metric.severity], .sum()
| map [status_class: @severity_to_status_class(metric.severity)]
| group_by [status_class], .sum()
| { ident; ident | group_by [], .sum() }
| join
| div
| value scale(val(), '%')
| every 1m
```

## Collecting a Latency metric from NGINX Logs and a Logs-based-metric

By default NGINX logs don't include latency, but the Ops Agent logs parser can
include latency in the logs if you include it at the end of the log entry
with an `'s'` suffix (to provide time duration units).

This is done using the NINGX `log_format` configuration option. For example:

```
log_format timed_combined '\$remote_addr - \$remote_user [\$time_local] '
      '"\$request" \$status \$body_bytes_sent '
      '"\$http_referer" "\$http_user_agent" '
      '\$request_time' 's';
server {
   ...
   access_log /var/log/nginx/access.log timed_combined;
}
```

You can then create a [logs based distribution metric](https://cloud.google.com/logging/docs/logs-based-metrics/distribution-metrics) to enable you to chart and
alert on NGINX latency opercentiles.

Here's a `gcloud` command you can run to create such a metric (change the
`PROJECT_ID` assignment if you want to use a different project that what
`gcloud` is currently configured with):

```
PROJECT_ID="$(gcloud config get-value project)"
tee /tmp/nginx_latencies_metric_config.yaml << EOF
name: nginx.latencies
description: NGINX Request Latency Histogram
valueExtractor: REGEXP_EXTRACT(httpRequest.latency, "(\\\\d|\\\\.)+s")
filter: logName="projects/$PROJECT_ID/logs/nginx_access" resource.type="gce_instance"
metricDescriptor:
  description: NGINX Request Latency Histogram
  metricKind: DELTA
  type: logging.googleapis.com/user/nginx.latencies
  unit: s
  valueType: DISTRIBUTION
bucketOptions:
  exponentialBuckets:
    growthFactor: 1.21
    numFiniteBuckets: 64
    scale: 0.001
EOF
gcloud logging metrics create nginx.latencies --config-from-file /tmp/nginx_latencies_metric_config.yaml --project $PROJECT_ID
rm /tmp/nginx_latencies_metric_config.yaml
```
