# Apache Web Server (httpd)

Follow [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/apache)
for instructions to collect logs and metrics from this application using Ops Agent.

## Metrics

The following table provides the list of metrics that the Ops Agent collects from this application.

| Metric                                            | Data Type | Unit        | Labels              | Description |
| ---                                               | ---       | ---         | ---                 | ---         | 
| workload.googleapis.com/apache.current_connections | gauge     | connections |      server_name        | The number of active connections currently attached to the HTTP server.  |
| workload.googleapis.com/apache.requests            | sum       | 1    |    server_name         | Total requests serviced by the HTTP server.  |
| workload.googleapis.com/apache.workers             | gauge     | connections | server_name, workers_state     | The number of workers currently attached to the HTTP server |
| workload.googleapis.com/apache.scoreboard          | gauge     | scoreboard  | server_name, scoreboard_state  | Apache HTTP server scoreboard values. |
| workload.googleapis.com/apache.traffic             | sum       | byte |     server_name     | Total HTTP server traffic. |

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
