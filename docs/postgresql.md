# `postgresql` Metrics Receiver

The postgresql receiver can retrieve stats from your postgresql instance by connecting as a monitoring user.

## Prerequisites

The `postgresql` receiver defaults to connecting to a local postgresql server using a Unix socket and Unix authentication as the `postgres` user.

## Configuration

Following the guide for [Configuring the Ops Agent](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration#file-location), add the required elements for your postgresql configuration.

To configure a receiver for your postgresql metrics, specify the following fields:

| Field                   | Default                         | Description |
| ---                     | ---                             | ---         |
| `type`                  | required                        | Must be `postgresql`. |
| `endpoint`              | `/var/run/postgresql/.s.PGSQL.5432`   | The hostname:port or socket path used to connect to postgresql |
| `collection_interval`   | `60s`                           | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |
| `username`              | `postgres`                          | The username used to connect to the server. |
| `password`              |                                 | The password used to connect to the server. |
| `insecure`              | true                            | Signals whether to use a secure TLS connection or not. If insecure is true TLS will not be enabled. |
| `insecure_skip_verify`  | true                            | Whether to skip verifying the certificate or not. A false value of insecure_skip_verify will not be used if insecure is true as the connection will not use TLS at all. |
| `cert_file`             |                             | Path to the TLS cert to use for TLS required connections. |
| `key_file`              |                             | Path to the TLS key to use for TLS required connections. |
| `ca_file`               |                             | Path to the CA cert. As a client this verifies the server certificate. If empty, uses system root CA. |

Example Configuration:

```yaml
metrics:
  receivers:
    postgresql_metrics:
      type: postgresql
      username: usr
      password: pwd
  service:
    pipelines:
      postgresql_pipeline:
        receivers:
          - postgresql_metrics
```

TCP connection with a username and password:

```yaml
metrics:
  receivers:
    postgresql_metrics:
      type: postgresql 
      endpoint: localhost:3306
      collection_interval: 30s
      password: pwd
      username: usr
  service:
    pipelines:
      postgresql_pipeline:
        receivers:
          - postgresql_metrics
```

TCP connection with a username and password and TLS:

```yaml
metrics:
  receivers:
    postgresql_metrics:
      type: postgresql 
      endpoint: localhost:3306
      collection_interval: 30s
      password: pwd
      username: usr
      insecure: false
      insecure_skip_verify: false
      cert_file: /path/to/cert
      ca_file: /path/to/ca
  service:
    pipelines:
      postgresql_pipeline:
        receivers:
          - postgresql_metrics
```

## Metrics

The Ops Agent collects the following metrics from your postgresql instances.

| Metric                                                 | Data Type | Unit        | Labels                          | Description    |
| ---                                                    | ---       | ---         | ---                             | ---            | 
| workload.googleapis.com/postgresql.backends            | gauge     | 1           | database                        | The number of backends. |
| workload.googleapis.com/postgresql.blocks_read         | sum       | 1           | database, table, source         | The number of blocks read. |
| workload.googleapis.com/postgresql.commits             | sum       | 1           | database                        | The number of commits.     |
| workload.googleapis.com/postgresql.db_size             | gauge     | By          | database                        | The database disk usage. |
| workload.googleapis.com/postgresql.operations          | sum       | 1           | database, table, operation      | The number of db row operations. |
| workload.googleapis.com/postgresql.rollbacks           | sum       | 1           | database                        | The number of rollbacks. |
| workload.googleapis.com/postgresql.rows                | sum       | 1           | database, table, state          | The number of rows in the database. |



# `postgresql_general` Logging Receiver

## Configuration

To configure a receiver for your postgresql general logs, specify the following fields:

| Field                 | Default                      | Description |
| ---                   | ---                          | ---         |
| `type`                | required                     | Must be `postgresql_general`. |
| `include_paths`       | `[/var/log/postgresql/postgresql*.log]` | The log files to read. |
| `exclude_paths`       | `[]`                         | Log files to exclude (if `include_paths` contains a glob or directory). |


Example Configuration:

```yaml
logging:
  receivers:
    postgresql_general:
      type: postgresql_general
  service:
    pipelines:
      postgresql:
        receivers:
          - postgresql_general
```

## Logs

General Query logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.tid` | number | Thread ID where the log originated |
| `jsonPayload.role` | string | Authenticated role for the action being logged when relevant |
| `jsonPayload.user` | string | Authenticated user for the action being logged when relevant |
| `jsonPayload.type` | string | Type of log - may be severity, or may be type of database interaction depending on log |
| `jsonPayload.message` | string | Log of the database action |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged |
