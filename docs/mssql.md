# MS SQL Server

Follow the [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/mssql) for instructions to collect metrics from this application using Ops Agent.

## Prerequisites

The `mssql` metrics receiver can fetch stats from a SQL Server via the [Windows Performance Counters](https://docs.microsoft.com/en-us/windows/win32/perfctrs/performance-counters-portal).

Because this `mssql` receiver makes use of the windows performance counters, it is required to run locally on the same machine with the SQL Server installation.

## Configuration

| Field                 | Default            | Description |
| ---                   | ---                | ---         |
| `type`                | required           | Must be `mssql`. |
| `collection_interval` | `60s`              | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |
| `receiver_version`    | `1`                | Determines which set of metrics will be retrieved. The only versions are `1` and `2`. |

Example Configuration:

```yaml
metrics:
  receivers:
    mssql:
      type: mssql
      collection_interval: 60s
      receiver_version: 2
  service:
    pipelines:
      mssql:
        receivers:
          - mssql
```

## Metrics

The following table provides the list of metrics that the Ops Agent collects from this application for the different versions of this receiver.
Note: In order for any metrics to be retrieved, the receiver must run locally on the same server as the SQL Server installation.

### v1
| Metric                                            | Data Type | Unit | Labels | Description |
| ---                                               | ---       | ---  | ---    | ---         | 
| agent.googleapis.com/mssql/connections/user       | gauge     | 1    |        | Currently open connections to SQL server. |
| agent.googleapis.com/mssql/transaction_rate       | gauge     | 1/s  |        | SQL server total transactions per second. |
| agent.googleapis.com/mssql/write_transaction_rate | gauge     | 1/s  |        | SQL server write transactions per second. |

### v2
| Metric                                               | Data Type      | Unit        | Labels      | Description |
| ---                                                  | ---            | ---         | ---         | ---         | 
| workload.googleapis.com/sqlserver.user.connection.count | Gauge       | connections |             | Number of users connected to the SQL Server. |
| workload.googleapis.com/sqlserver.lock.wait_time.avg | Gauge          | ms          |             | Average wait time for all lock requests that had to wait. |
| workload.googleapis.com/sqlserver.lock.wait.rate     | Gauge          | requests/s  |             | Number of lock requests resulting in a wait per second. |
| workload.googleapis.com/sqlserver.batch.request.rate | Gauge          | requests/s  |             | Number of batch requests received by SQL Server per second. |
| workload.googleapis.com/sqlserver.batch.sql_compilation.rate | Gauge  | compilations/s |          | Number of SQL compilations needed per second. |
| workload.googleapis.com/sqlserver.batch.sql_recompilation.rate | Gauge | compilations/s |         | Number of SQL recompilations needed per second. |
| workload.googleapis.com/sqlserver.page.buffer_cache.hit_ratio | Gauge | %           |             | Percent of pages found in the buffer pool without having to read from disk. |
| workload.googleapis.com/sqlserver.page.life_expectancy | Gauge        | s           |             | Time a page will stay in the buffer pool. |
| workload.googleapis.com/sqlserver.page.split.rate    | Gauge          | pages/s     |             | Number of pages split as a result of overflowing index pages per second. |
| workload.googleapis.com/sqlserver.page.lazy_write.rate | Gauge        | writes/s    |             | Number of lazy writes moving dirty pages to disk per second. |
| workload.googleapis.com/sqlserver.page.checkpoint.flush.rate | Gauge  | pages/s     |             | Number of pages flushed by operations requiring dirty pages to be flushed per second. |
| workload.googleapis.com/sqlserver.page.operation.rate | Gauge         | operation/s | type        | Number of physical database page operations issued per second. |
| workload.googleapis.com/sqlserver.transaction_log.growth.count | Cumulative | growths | database  | Total number of transaction log expansions for a database. |
| workload.googleapis.com/sqlserver.transaction_log.shrink.count | Cumulative | shrinks | database  | Total number of transaction log shrinks for a database. |
| workload.googleapis.com/sqlserver.transaction_log.percent_used | Gauge       | %           | database    | Percent of transaction log space used. |
| workload.googleapis.com/sqlserver.transaction_log.flush.wait.rate | Gauge | commits/s | database  | Number of commits waiting for a transaction log flush per second. |
| workload.googleapis.com/sqlserver.transaction_log.flush.rate | Gauge  | flushes/s   | database    | Number of log flushes per second. |
| workload.googleapis.com/sqlserver.transaction_log.flush.data.rate | Gauge | By/s    | database    | Total number of log bytes flushed per second. |
| workload.googleapis.com/sqlserver.transaction.rate   | Gauge          | transactions/s | database | Number of transactions started for the database (not including XTP-only transactions) per second. |
| workload.googleapis.com/sqlserver.transaction.write.rate | Gauge      | transactions/s | database | Number of transactions that wrote to the database and committed per second. |
