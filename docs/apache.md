# `apache httpd` Metrics Receiver

The Apache httpd receiver can retrieve stats from your Apache web server using the `/server-status?auto` endpoint.


## Prerequisites

The receiver requires that you enable the [mod_status module](https://httpd.apache.org/docs/2.4/mod/mod_status.html) on your Apache web server.

 This requires adding the following lines to the server’s `httpd.conf` file:

```
<Location "/server-status">
    SetHandler server-status
    Require host localhost
</Location>
```

## Configuration

Following the guide for [Configuring the Ops Agent](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration#file-location), add the required elements for your Apache web server configuration.

To configure a receiver for your Apache web server metrics, specify the following fields:

| Field                 | Default                   | Description |
| ---                   | ---                       | ---         |
| `type`                | required                  | Must be `apache`. |
| `server_status_url`     | `http://localhost:8080/server-status?auto` | The url exposed by the `mod_status` module. |
| `collection_interval` | `60s`                     | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |

Example Configuration:

```yaml
metrics:
  receivers:
    apache_metrics:
      type: apache
      server_status_url: http://localhost:8080/server-status?auto
      collection_interval: 30s
  service:
    pipelines:
      apache_pipeline:
        receivers:
          - apache_metrics
```

## Metrics

The Ops Agent collects the following metrics from your Apache web servers.

| Metric                                            | Data Type | Unit        | Labels              | Description |
| ---                                               | ---       | ---         | ---                 | ---         | 
| workload.googleapis.com/apache.current_connections | gauge     | connections |      server_name        | The number of active connections currently attached to the HTTP server.  |
| workload.googleapis.com/apache.requests            | sum       | 1    |    server_name         | Total requests serviced by the HTTP server.  |
| workload.googleapis.com/apache.workers             | gauge     | connections | server_name, workers_state     | The number of workers currently attached to the HTTP server |
| workload.googleapis.com/apache.scoreboard          | gauge     | scoreboard  | server_name, scoreboard_state  | Apache HTTP server scoreboard values. |
| workload.googleapis.com/apache.traffic             | sum       | byte |     server_name     | Total HTTP server traffic. |

# `apache_access` and `apache_error` Logging Receivers

## Configuration

To configure a receiver for your apache access logs, specify the following fields:

| Field                 | Default                       | Description |
| ---                   | ---                           | ---         |
| `type`                | required                      | Must be `apache_access`. |
| `include_paths`       | `[/var/log/apache2/access.log]` | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `/var/log/apache*/*.log`.
| `exclude_paths`       | `[]`                          | A list of filesystem path patterns to exclude from the set matched by `include_paths`.

To configure a receiver for your apache error logs, specify the following fields:

| Field                 | Default                      | Description |
| ---                   | ---                          | ---         |
| `type`                | required                     | Must be `apache_error`. |
| `include_paths`       | `[/var/log/apache2/error.log]` | The log files to read. |
| `exclude_paths`       | `[]`                         | Log files to exclude (if `include_paths` contains a glob or directory). |

Example Configuration:

```yaml
logging:
  receivers:
    apache_default_access:
      type: apache_access
    apache_default_error:
      type: apache_error
  service:
    pipelines:
      apache:
        receivers:
          - apache_default_access
          - apache_default_error
```

## Logs

Access logs contain the [`httpRequest` field](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#httprequest):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `httpRequest.protocol` | string | Protocol used for the request |
| `httpRequest.referer` | string | Contents of the `Referer` header |
| `httpRequest.requestMethod` | string | HTTP method |
| `httpRequest.requestUrl` | string | Request URL (typically just the path part of the URL) |
| `httpRequest.responseSize` | string (`int64`) | Response size |
| `httpRequest.status` | number | HTTP status code |
| `httpRequest.userAgent` | string | Contents of the `User-Agent` header |
| `jsonPayload.host` | string | Contents of the `Host` header |
| `jsonPayload.user` | string | Authenticated username for the request |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received |

Any fields that are blank or missing will not be present in the log entry.

Error logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.errorCode` | string | apache error code |
| `jsonPayload.level` | string | Log entry level |
| `jsonPayload.module` | string | apache module where the log originated |
| `jsonPayload.pid` | number | Process ID |
| `jsonPayload.tid` | number | Thread ID |
| `jsonPayload.message` | string | Log message |
| `jsonPayload.client` | string | Client IP address (optional) |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged |
