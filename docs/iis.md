# IIS

Follow the [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/iis) for instructions to collect metrics from this application using Ops Agent.

## Metrics

The following table provides the list of metrics that the Ops Agent collects from this application.

| Metric                                                   | Data Type  | Unit | Labels | Description |
| ---                                                      | ---        | ---  | ---    | ---         | 
| agent.googleapis.com/iis/current_connections             | gauge      | 1    |        | Currently open connections to IIS. |
| agent.googleapis.com/iis/network/transferred_bytes_count | cumulative | By   |        | Network bytes transferred by IIS. |
| agent.googleapis.com/iis/new_connection_count            | cumulative | 1    |        | Connections opened to IIS. |
| agent.googleapis.com/iis/request_count                   | cumulative | 1    | state  | Requests made to IIS. |

