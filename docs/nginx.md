# Nginx

Follow [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/nginx)
for instructions to collect logs and metrics from this application using Ops Agent.

## Metrics

The Ops Agent collects the following metrics from your nginx instances.

| Metric                                           | Data Type | Unit        | Labels | Description |
| ---                                              | ---       | ---         | ---    | ---         | 
| workload.googleapis.com/nginx.requests             | cumulative       | requests    |        | Total number of requests made to the server. |
| workload.googleapis.com/nginx.connections_accepted | cumulative       | connections |        | Total number of accepted client connections. |
| workload.googleapis.com/nginx.connections_handled  | cumulative       | connections |        | Total number of handled connections. |
| workload.googleapis.com/nginx.connections_current  | gauge     | connections | state  | Current number of connections. |

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
