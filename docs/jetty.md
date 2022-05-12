# Jetty

Follow the [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/jetty) for instructions to collect metrics from this application using Ops Agent.


# `jetty_access` Logging Receiver

## Prerequisites
Access logs have to be configured to log. See [documentation](https://www.eclipse.org/jetty/documentation/jetty-9/index.html#configuring-jetty-logging)

## Configuration

To configure a receiver for your Jetty Access logs, specify the following fields:

| Field                 | Default                           | Description |
| ---                   | ---                               | ---         |
| `type`                | required                          | Must be `jetty_access`. |
| `include_paths`       | `['/opt/logs/*.request.log']` | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `/opt/logs/*.request.log`. |
| `exclude_paths`       | `[]`                              | A list of filesystem path patterns to exclude from the set matched by `include_paths`.
| `wildcard_refresh_interval` | `60s` | The interval at which wildcard file paths in `include_paths` are refreshed. Specified as a time interval parsable by [time.ParseDuration](https://pkg.go.dev/time#ParseDuration). Must be a multiple of 1s.|

Example Configuration:

```yaml
logging:
  receivers:
    jetty_access:
      type: jetty_access
  service:
    pipelines:
      jetty:
        receivers:
          - jetty_access
```

### Access Logs

Access logs contain the [`httpRequest` field](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#httprequest):

Any fields that are blank or missing will not be present in the log entry.

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

### System Logs

System logs are collected by default in the syslog receiver.
