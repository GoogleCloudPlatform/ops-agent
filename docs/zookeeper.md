# `Zookeeper` Metrics Receiver

The Zookeeper receiver collects metrics from a Zookeeper instance, using the `mntr` command. The `mntr` 4 letter word command needs
to be enabled for the receiver to be able to collect metrics.

## Configuration

Following the guide for [Configuring the Ops Agent](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration#file-location), add the required elements for your Apache web server configuration.

To configure a receiver for your Zookeeper metrics, specify the following fields:

| Field                 | Default                 | Description                                                                            |
|-----------------------|-------------------------|----------------------------------------------------------------------------------------|
| `type`                | required                | Must be `zookeeper`.                                                                   |
| `endpoint`            | `localhost:2181`        | The url exposed by zookeeper                                                           |
| `collection_interval` | `60s`                   | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |

Example configuration.

```yaml
metrics:
  receivers:
    zookeeper:
      type: zookeeper
  service:
    pipelines:
      zookeeper:
        receivers:
          - zookeeper
```

## Metrics

The Ops Agent collects the following metrics from your Zookeeper instance.

| Metric                                               | Data Type | Unit       | Labels                      | Description                                  |
|------------------------------------------------------|-----------|------------|-----------------------------|----------------------------------------------|
| workload.googleapis.com/zookeeper.connection.active               | sum   | connections      |           | Number of active clients connected to a ZooKeeper server.                   |
| workload.googleapis.com/zookeeper.data_tree.ephemeral_node.count  | sum   | nodes            |           | Number of ephemeral nodes that a ZooKeeper server has in its data tree.     |
| workload.googleapis.com/zookeeper.data_tree.size                  | sum   | By               |           | Size of data in bytes that a ZooKeeper server has in its data tree.         |
| workload.googleapis.com/zookeeper.file_descriptor.limit           | gauge | file_descriptors |           | Maximum number of file descriptors that a ZooKeeper server can open.        |
| workload.googleapis.com/zookeeper.file_descriptor.open            | sum   | file_descriptors |           | Number of file descriptors that a ZooKeeper server has open.                |
| workload.googleapis.com/zookeeper.follower.count                  | sum   | followers        | state     | The number of followers. Only exposed by the leader.                        |
| workload.googleapis.com/zookeeper.fsync.exceeded_threshold.count  | sum   | events           |           | Number of times fsync duration has exceeded warning threshold.              |
| workload.googleapis.com/zookeeper.latency.avg                     | gauge | ms               |           | Average time in milliseconds for requests to be processed.                  |
| workload.googleapis.com/zookeeper.latency.max                     | gauge | ms               |           | Maximum time in milliseconds for requests to be processed.                  |
| workload.googleapis.com/zookeeper.latency.min                     | gauge | ms               |           | Minimum time in milliseconds for requests to be processed.                  |
| workload.googleapis.com/zookeeper.packet.count                    | sum   | packets          | direction | The number of ZooKeeper packets received or sent by a server.               |
| workload.googleapis.com/zookeeper.request.active                  | sum   | requests         |           | Number of currently executing requests.                                     |
| workload.googleapis.com/zookeeper.sync.pending                    | sum   | syncs            |           | The number of pending syncs from the followers. Only exposed by the leader. |
| workload.googleapis.com/zookeeper.watch.count                     | sum   | watches          |           | Number of watches placed on Z-Nodes on a ZooKeeper server.                  |
| workload.googleapis.com/zookeeper.znode.count                     | sum   | znodes           |           | Number of z-nodes that a ZooKeeper server has in its data tree.             |


# `zookeeper_general` Logging Receiver

## Configuration

To configure a receiver for your Zookeeper logs, specify the following fields:

| Field                 | Default                           | Description |
| ---                   | ---                               | ---         |
| `type`                | required                          | Must be `zookeeper_general`. |
| `include_paths`       | `[/opt/zookeeper/logs/zookeeper-*.out, /var/log/zookeeper/zookeeper.log]` | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `/var/log/zookeeper*/*.log`. |
| `exclude_paths`       | `[]`                              | A list of filesystem path patterns to exclude from the set matched by `include_paths`. |
| `wildcard_refresh_interval` | `60s` | The interval at which wildcard file paths in include_paths are refreshed. Specified as a time interval parsable by [time.ParseDuration](https://pkg.go.dev/time#ParseDuration). Must be a multiple of 1s.|

Example Configuration:

```yaml
logging:
  receivers:
    zookeeper:
      type: zookeeper_general
  service:
    pipelines:
      zookeeper:
        receivers:
          - zookeeper
```

## Logs

System logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received |
| `jsonPayload.line` | number | Line number from which the log was generated in source |
| `jsonPayload.source` | string | Source of where the log originated |
| `jsonPayload.thread` | string | Thread from which the log originated |
| `jsonPayload.myid` | number | Numeric ID of the Zookeeper instance |
| `jsonPayload.message` | string | Log message, including detailed stacktrace where provided |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |
