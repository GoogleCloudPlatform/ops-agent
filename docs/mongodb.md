# `mongodb` Metrics Receiver

The `mongodb` metrics receiver can fetch server stats and database stats from a MongoDB instance.

## Prerequisites

If authentication is required, the supplied user must have [clusterMonitor](https://docs.mongodb.com/manual/reference/built-in-roles/#mongodb-authrole-clusterMonitor) permissions.

## Configuration

| Field                  | Required | Default                 | Description                                                                                                                                                             |
|------------------------|----------|-------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `type`                 | required |                         | Must be `mongodb`.                                                                                                                                                |
| `collection_interval`  | optional | `60s`                   | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`.                                                                                  |
| `endpoint`             | optional | `http://localhost:27017` | Either a hostname, IP address, or UNIX domain socket. A port can be specified like \<hostname\>:\<port\>. If no port is specified the default 27017 will be used.                                                                                                                                  |
| `username`             | optional |                         | Username for authentication with the MongoDB instance. Required if `password` is set.                                                                                          |
| `password`             | optional |                         | Password for authentication with MongoDB instance. Required if `username` is set.                                                                                          |                                                                      |
| `insecure`             | optional | true                    | Signals whether to use a secure TLS connection or not. If insecure is true TLS will not be enabled.                                                                     |
| `insecure_skip_verify` | optional | false                   | Whether to skip verifying the certificate or not. A false value of insecure_skip_verify will not be used if insecure is true as the connection will not use TLS at all. |
| `cert_file`            | optional |                         | Path to the TLS cert to use for mTLS required connections.                                                                                                              |
| `key_file`             | optional |                         | Path to the TLS key to use for mTLS required connections.                                                                                                               |
| `ca_file`              | optional |                         | Path to the CA cert. As a client this verifies the server certificate. If empty, uses system root CA.                                                                   |
|


Example Configuration:

```yaml
metrics:
  receivers:
    mongodb:
      type: mongodb
  service:
    pipelines:
      mongodb:
        receivers:
          - mongodb
```

# `mongodb` Logging Receiver

## Configuration

To configure a receiver for your mongodb logs, specify the following fields:

| Field                 | Default                       | Description |
| ---                   | ---                           | ---         |
| `type`                | required                      | Must be `mongodb`. |
| `include_paths`       | `[/var/log/mongodb/mongod.log*]` | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `/var/log/mongodb/*.log`.
| `exclude_paths`       | `[]`                          | A list of filesystem path patterns to exclude from the set matched by `include_paths`.
| `wildcard_refresh_interval` | `60s` | The interval at which wildcard file paths in include_paths are refreshed. Specified as a time interval parsable by [time.ParseDuration](https://pkg.go.dev/time#ParseDuration). Must be a multiple of 1s.|


Example Configuration:

```yaml
logging:
  receivers:
    mongodb:
      type: mongodb
  service:
    pipelines:
      mongodb:
        receivers: [mongodb]
```

## Logs

MongoDB logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.component` | string | Categorization of the log message. A full list can be found [here](https://docs.mongodb.com/manual/reference/log-messages/#std-label-log-message-components) |
| `jsonPayload.ctx` | string | The name of the thread issuing the log statement |
| `jsonPayload.id` | number | Log ID |
| `jsonPayload.message` | string | Log message |
| `jsonPayload.attributes` | object (optional) | Object containing one or more key-value pairs for any additional attributes provided |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged |
