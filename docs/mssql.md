# MS SQL Server

Follow the [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/mssql) for instructions to collect metrics from this application using Ops Agent.

## Metrics

The following table provides the list of metrics that the Ops Agent collects from this application.

| Metric                                            | Data Type | Unit | Labels | Description |
| ---                                               | ---       | ---  | ---    | ---         | 
| agent.googleapis.com/mssql/connections/user       | gauge     | 1    |        | Currently open connections to SQL server. |
| agent.googleapis.com/mssql/transaction_rate       | gauge     | 1/s  |        | SQL server total transactions per second. |
| agent.googleapis.com/mssql/write_transaction_rate | gauge     | 1/s  |        | SQL server write transactions per second. |

