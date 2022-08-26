# Varnish

Follow [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/varnish)
for instructions to collect logs and metrics from this application using Ops Agent.

## Metrics

The Ops Agent collects the following metrics from your varnish instance.

| Metric                                                     | Data Type  | Unit          | Labels                              | Description                                                      |
|------------------------------------------------------------|------------|---------------|-------------------------------------|------------------------------------------------------------------|
| workload.googleapis.com/varnish.backend.connection.count   | cumulative | {connections} | cache_name, backend_connection_type | The backend connection type count                                |
| workload.googleapis.com/varnish.cache.operation.count      | cumulative | {operations}  | cache_name, cache_operations        | The cache operation type count                                   |
| workload.googleapis.com/varnish.thread.operation.count     | cumulative | {operations}  | cache_name, thread_operations       | The thread operation type count                                  |
| workload.googleapis.com/varnish.session.count              | cumulative | {sessions}    | cache_name, session_type            | The session connection type count                                |
| workload.googleapis.com/varnish.object.nuked               | cumulative | {objects}     | cache_name                          | The objects that have been forcefully evicted from storage count |
| workload.googleapis.com/varnish.object.moved               | cumulative | {objects}     | cache_name                          | The moved operations done on the LRU list count                  |
| workload.googleapis.com/varnish.object.expired             | cumulative | {objects}     | cache_name                          | The expired objects from old age count                           |
| workload.googleapis.com/varnish.object.count               | gauge      | {objects}     | cache_name                          | The HTTP objects in the cache count                              |
| workload.googleapis.com/varnish.client.request.count       | cumulative | {requests}    | cache_name, state                   | The client request count                                         |
| workload.googleapis.com/varnish.client.request.error.count | cumulative | {requests}    | cache_name, http.status_code        | The client requests errors received by status code.              |
| workload.googleapis.com/varnish.backend.request.count      | cumulative | {requests}    | cache_name                          | The backend requests count                                       |

## Logs

Varnish logs contain the [`httpRequest` field](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#httprequest):

| Field                       | Type                                                                                                                            | Description                                                   |
|-----------------------------|---------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------|
| `httpRequest.protocol`      | string                                                                                                                          | Protocol used for the request                                 |
| `httpRequest.referer`       | string                                                                                                                          | Contents of the `Referer` header                              |
| `httpRequest.remoteIp`      | string                                                                                                                          | Client IP address                                             |
| `httpRequest.requestMethod` | string                                                                                                                          | HTTP method                                                   |
| `httpRequest.requestUrl`    | string                                                                                                                          | Request URL (typically just the path part of the URL)         |
| `httpRequest.responseSize`  | string (`int64`)                                                                                                                | Response size                                                 |
| `httpRequest.status`        | number                                                                                                                          | HTTP status code                                              |
| `httpRequest.userAgent`     | string                                                                                                                          | Contents of the `User-Agent` header                           |
| `jsonPayload.host`          | string                                                                                                                          | Contents of the `Host` header (usually not reported by nginx) |
| `jsonPayload.user`          | string                                                                                                                          | Authenticated username for the request                        |
| `timestamp`                 | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received                                 |
