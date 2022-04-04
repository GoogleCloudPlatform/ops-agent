# Varnish

Follow [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/varnish)
for instructions to collect logs and metrics from this application using Ops Agent.

# `varnish` Metrics Receiver

## Configuration

Following the guide for [Configuring the Ops Agent](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration#file-location), add the required elements for your varnish instance configuration.

To configure a receiver for your Varnish metrics, specify the following fields:

| Field                 | Required | Default | Description                                                                                                                                                                 |
|-----------------------|----------|---------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `type`                | required |         | Must be `varnish`.                                                                                                                                                          |
| `collection_interval` | required |         | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`.                                                                                      |
| `cache_dir`           | optional |         | Optional. This specifies the cache dir instance name to use when collecting metrics. If not specified, this will default to the host name.                                  |
| `exec_dir`            | optional |         | Optional. The directory where the varnishadm and varnishstat executables are located. If not provided, will default to relying on the executables being in the user's PATH. |

Example Configuration:

```yaml
metrics:
  receivers:
    varnish:
      type: varnish
  service:
    pipelines:
      varnish:
        receivers:
          - varnish
```

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
