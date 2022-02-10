# Couchdb

Follow [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/couchdb)
for instructions to collect logs and metrics from this application using Ops Agent.


## Metrics

The Ops Agent collects the following metrics from your couchdb server.

| Metric                                               | Data Type | Unit         | Labels                      | Description                                  |
|------------------------------------------------------|-----------|--------------|-----------------------------|----------------------------------------------|
| workload.googleapis.com/couchdb.average_request_time | gauge     | ms           | node_name                   | The average duration of a served request.    |
| workload.googleapis.com/couchdb.httpd.bulk_requests  | sum       | {requests}   | node_name                   | The number of bulk requests.                 |
| workload.googleapis.com/couchdb.httpd.requests       | sum       | {requests}   | node_name, http.method      | The number of HTTP requests by method.       |
| workload.googleapis.com/couchdb.httpd.responses      | sum       | {responses}  | node_name, http.status_code | The number of HTTP responses by status code. |
| workload.googleapis.com/couchdb.httpd.views          | sum       | {views}      | node_name, view             | The number of views read.                    |
| workload.googleapis.com/couchdb.database.open        | gauge     | {databases}  | node_name                   | The number of open databases.                |
| workload.googleapis.com/couchdb.file_descriptor.open | gauge     | {files}      | node_name                   | The number of open file descriptors.         |
| workload.googleapis.com/couchdb.database.operations  | sum       | {operations} | node_name, operation        | The number of database operations.           |

## Logs

Access logs contain the [`httpRequest` field](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#httprequest)
Error logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry)
Any fields that are blank or missing will not be present in the log entry.

| Field                       | Type                                                                                                                            | Description                            |
|-----------------------------|---------------------------------------------------------------------------------------------------------------------------------|----------------------------------------|
| `httpRequest.serverIp`      | string                                                                                                                          | Server IP address                      |
| `httpRequest.remoteIp`      | string                                                                                                                          | Client IP address                      |
| `httpRequest.requestMethod` | string                                                                                                                          | HTTP method                            |
| `httpRequest.responseSize`  | string (`int64`)                                                                                                                | Response size                          |
| `httpRequest.status`        | number                                                                                                                          | HTTP status code                       |
| `jsonPayload.remote_user`   | string                                                                                                                          | Authenticated username for the request |
| `jsonPayload.pid`           | number                                                                                                                          | Process ID                             |
| `jsonPayload.message`       | string                                                                                                                          | Log message                            |
| `jsonPayload.node`          | string                                                                                                                          | node instance name                     |
| `jsonPayload.host`          | string                                                                                                                          | host instance name                     |
| `jsonPayload.path`          | string                                                                                                                          | request path                           |
| `jsonPayload.remote_user`   | string                                                                                                                          | user id (optional)                     |
| `severity`      | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity))                       | Log entry level (translated)           |
| `timestamp`                 | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged              |
