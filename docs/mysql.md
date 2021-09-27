# `mysql` Metrics Receiver

The mysql receiver can retrieve stats from your mysql instance by connecting as a monitoring user.


## Prerequisites

It is recommended that you create a dedicated monitoring user. The monitoring user must be granted the following permissions:

```
PROCESS ON *.*
SELECT ON INFORMATION_SCHEMA.INNODB_METRICS
```


## Configuration

Following the guide for [Configuring the Ops Agent](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration#file-location), add the required elements for your mysql configuration.

To configure a receiver for your mysql metrics, specify the following fields:

| Field                 | Default                   | Description |
| ---                   | ---                       | ---         |
| `type`                | required                  | Must be `mysql`. |
| `endpoint`     | `localhost:3306`          | The url exposed by mysql |
| `collection_interval` | `60s`                     | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |
| `username`            |                           | The username used to connect to the server.
| `password`            |                           | The password used to connect to the server.

Example Configuration:

```yaml
metrics:
  receivers:
    mysql_metrics:
      type: mysql
      endpoint: localhost:3306
      collection_interval: 30s
      password: pwd
      username: usr
  service:
    pipelines:
      mysql_pipeline:
        receivers:
          - mysql_metrics
```

## Metrics

The Ops Agent collects the following metrics from your mysql instances.

| Metric                                               | Data Type | Unit        | Labels                  | Description    |
| ---                                                  | ---       | ---         | ---                     | ---            | 
| workload.googleapis.com/mysql.buffer_pool_pages      | gauge     | 1           | buffer_pool_pages       | Buffer pool page count. |
| workload.googleapis.com/mysql.buffer_pool_operations | sum       | 1           | buffer_pool_operations  | Buffer pool operation count. |
| workload.googleapis.com/mysql.buffer_pool_size       | gauge     | 1           | buffer_pool_size        | Buffer pool size.     |
| workload.googleapis.com/mysql.commands               | sum       | 1           | command                 | MySQL command count. |
| workload.googleapis.com/mysql.handlers               | sum       | 1           | handler                 | MySQL handler count. |
| workload.googleapis.com/mysql.double_writes          | sum       | 1           | double_writes           | InnoDB doublewrite buffer count. |
| workload.googleapis.com/mysql.log_operations         | sum       | 1           | log_operations          | InndoDB log operation count. |
| workload.googleapis.com/mysql.operations             | sum       | 1           | operations              | InndoDB operation count. |
| workload.googleapis.com/mysql.page_operations        | sum       | 1           | page_operations         | InndoDB page operation count. |
| workload.googleapis.com/mysql.row_locks              | sum       | 1           | row_locks               | InndoDB row lock count. |
| workload.googleapis.com/mysql.row_operations         | sum       | 1           | row_operations          | InndoDB row operation count. |
| workload.googleapis.com/mysql.locks                  | sum       | 1           | locks                   | MySQL lock count. |
| workload.googleapis.com/mysql.sorts                  | sum       | 1           | sorts                   | MySQL sort count. |
| workload.googleapis.com/mysql.threads                | gauge     | 1           | threads                 | Thread count. |