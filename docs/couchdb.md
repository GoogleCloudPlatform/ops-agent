# `couchdb` Metrics Receiver

The couchdb receiver can retrieve stats from your couchdb server using the `/_node/_local/_stats/couchdb` endpoint.

## Configuration

Following the guide for [Configuring the Ops Agent](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration#file-location), add the required elements for your Apache web server configuration.

To configure a receiver for your Apache web server metrics, specify the following fields:

| Field                 | Default                 | Description                                                                            |
|-----------------------|-------------------------|----------------------------------------------------------------------------------------|
| `type`                | required                | Must be `couchdb`.                                                                     |
| `endpoint`            | `http://localhost:5984` | The url exposed by couchdb                                                             |
| `username`            | not set by default      | The username used to connect to the server.                                            |
| `password`            | not set by default      | The password used to connect to the server.                                            |
| `collection_interval` | `60s`                   | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |

Example Configuration:

```yaml
metrics:
  receivers:
    couchdb:
      type: couchdb
      endpoint: http://localhost:5984
      username: otelu
      password: otelp
  service:
    pipelines:
      couchdb:
        receivers:
          - couchdb
```

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
