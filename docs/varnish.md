# Varnish

Follow [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/varnish)
for instructions to collect logs and metrics from this application using Ops Agent.

# `varnish` Metrics Receiver

## Configuration

Following the guide for [Configuring the Ops Agent](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration#file-location), add the required elements for your varnish instance configuration.

To configure a receiver for your Varnish metrics, specify the following fields:

| Field                 | Required | Default | Description                                                                                                        |
|-----------------------|----------|---------|--------------------------------------------------------------------------------------------------------------------|
| `type`                | required |         | Must be `varnish`.                                                                                                 |
| `collection_interval` | required |         | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`.                             |
| `exec_dir`            | optional |         | The executable directory where the varnishd and varnishstat executable lies.                                       |
| `cache_dir`           | optional |         | This specifies the cache dir to use when collecting metrics. If not specified, this will default to the host name. |

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

| Metric                                                    | Data Type  | Unit          | Labels                              | Description                                                      |
|-----------------------------------------------------------|------------|---------------|-------------------------------------|------------------------------------------------------------------|
| workload.googleapis.com/varnish.backend.connections.count | cumulative | {connections} | cache_name, backend_connection_type | The backend connection type count                                |
| workload.googleapis.com/varnish.cache.operations.count    | cumulative | {operations}  | cache_name, cache_operations        | The cache operation type count                                   |
| workload.googleapis.com/varnish.thread.operations.count   | cumulative | {operations}  | cache_name, thread_operations       | The thread operation type count                                  |
| workload.googleapis.com/varnish.session.count             | cumulative | {sessions}    | cache_name, session_type            | The session connection type count                                |
| workload.googleapis.com/varnish.object.nuked.count        | cumulative | {objects}     | cache_name                          | The objects that have been forcefully evicted from storage count |
| workload.googleapis.com/varnish.object.moved.count        | cumulative | {objects}     | cache_name                          | The moved operations done on the LRU list count                  |
| workload.googleapis.com/varnish.object.expired.count      | cumulative | {objects}     | cache_name                          | The expired objects from old age count                           |
| workload.googleapis.com/varnish.object.count              | gauge      | {objects}     | cache_name                          | The HTTP objects in the cache count                              |
| workload.googleapis.com/varnish.client.requests.count     | cumulative | {requests}    | cache_name, state                   | The client request count                                         |
| workload.googleapis.com/varnish.backend.requests.count    | cumulative | {requests}    | cache_name                          | The backend requests count                                       |
