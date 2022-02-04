# Apache Tomcat

Follow [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/tomcat)
for instructions to collect logs and metrics from this application using Ops Agent.

## Metrics
In addition to Tomcat specific metrics, by default Tomcat will also report [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics)

| Metric                                               | Data Type      | Unit        | Labels                         | Description |
| ---                                                  | ---            | ---         | ---                            | ---         | 
| workload.googleapis.com/tomcat.sessions              | Gauge          | sessions    |                                | The number of active sessions. |
| workload.googleapis.com/tomcat.errors                | Cumulative     | errors      | proto_handler                  | The number of errors encountered. |
| workload.googleapis.com/tomcat.processing_time       | Cumulative     | ms          | proto_handler                  | The total processing time. |
| workload.googleapis.com/tomcat.traffic               | Cumulative     | by          | proto_handler, direction       | The number of bytes transmitted and received. |
| workload.googleapis.com/tomcat.threads               | Gauge          | threads     | proto_handler, state           | The number of threads. |
| workload.googleapis.com/tomcat.max_time              | Gauge          | ms          | proto_handler                  | Maximum time to process a request. |
| workload.googleapis.com/tomcat.request_count         | Cumulative     | requests    | proto_handler                  | The total requests. |

## Logs

System logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received |
| `jsonPayload.module` | string | Module of tomcat where the log originated |
| `jsonPayload.source` | string | source of where the log originated |
| `jsonPayload.message` | string | Log message, including detailed stacktrace where provided |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |

Any fields that are blank or missing will not be present in the log entry.

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
