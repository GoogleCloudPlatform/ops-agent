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
