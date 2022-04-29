# IIS

Follow the [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/iis) for instructions to collect metrics from this application using Ops Agent.

Additionally, specify `version` as `v2` to enable new metrics gathering. 

Example `v2` Configuration
```yaml
metrics:
  receivers:
    iis:
      type: iis
      receiver_version: 2
  service:
    pipelines:
      iispipeline:
        receivers:
          - iis
```

## Metrics

The following tables provide the lists of metrics that the Ops Agent collects from this application for each supported version of this receiver.

### v1
| Metric                                                   | Data Type  | Unit | Labels | Description |
| ---                                                      | ---        | ---  | ---    | ---         | 
| agent.googleapis.com/iis/current_connections             | gauge      | 1    |        | Currently open connections to IIS. |
| agent.googleapis.com/iis/network/transferred_bytes_count | cumulative | By   |        | Network bytes transferred by IIS. |
| agent.googleapis.com/iis/new_connection_count            | cumulative | 1    |        | Connections opened to IIS. |
| agent.googleapis.com/iis/request_count                   | cumulative | 1    | state  | Requests made to IIS. |


### v2
| Metric                                                   | Data Type  | Unit | Labels | Description |
| ---                                                      | ---        | ---  | ---    | ---         | 
| iis.connection.active | gauge | {connections} | <ul> </ul>  |Number of active connections. |
| iis.connection.anonymous | cumulative | {connections} | <ul> </ul>  |Number of connections established anonymously. |
| iis.connection.attempt.count | cumulative | {attempts} | <ul> </ul>  |Total number of attempts to connect to the server. |
| iis.network.blocked | cumulative | By | <ul> </ul>  |Number of bytes blocked due to bandwidth throttling. |
| iis.network.file.count | cumulative | {files} | <ul> <li>direction</li> </ul>  |Number of transmitted files. |
| iis.network.io | cumulative | By | <ul> <li>direction</li> </ul>  |Total amount of bytes sent and received. |
| iis.request.count | cumulative | {requests} | <ul> <li>request</li> </ul>  |Total number of requests of a given type. |
| iis.request.queue.age.max | gauge | ms | <ul> </ul>  |Age of oldest request in the queue. |
| iis.request.queue.count | gauge | {requests} | <ul> </ul>  |Current number of requests in the queue. |
| iis.request.rejected | cumulative | {requests} | <ul> </ul>  |Total number of requests rejected. |
| iis.thread.active | gauge | {threads} | <ul> </ul>  |Current number of active threads. |
| iis.uptime | gauge | s | <ul> </ul>  |The amount of time the server has been up. |

# `iis_access` Logging Receiver

## Configuration

To configure a receiver for your IIS system logs, specify the following fields:

| Field                 | Default                           | Description |
| ---                   | ---                               | ---         |
| `type`                | required                          | Must be `iis_access`. |
| `include_paths`       | `['C:\inetpub\logs\LogFiles\W3SVC1\u_ex*']` | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `C:\inetpub\logs\LogFiles\W3SVC1\u_ex*`. |
| `exclude_paths`       | `[]`                              | A list of filesystem path patterns to exclude from the set matched by `include_paths`.
| `wildcard_refresh_interval` | `60s` | The interval at which wildcard file paths in `include_paths` are refreshed. Specified as a time interval parsable by [time.ParseDuration](https://pkg.go.dev/time#ParseDuration). Must be a multiple of 1s.|

This receiver currently only supports the default W3C format.

Example Configuration:

```yaml
logging:
  receivers:
    iis_access:
      type: iis_access
  service:
    pipelines:
      iis:
        receivers:
          - iis_access
```

## Logs

Access logs contain the [`httpRequest` field](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#httprequest):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `httpRequest.referer` | string | Contents of the `Referer` header |
| `httpRequest.requestMethod` | string | HTTP method |
| `httpRequest.serverIp` | string | The server's IP and port that was requested |
| `httpRequest.remoteIp` | string | IP of the client that made the request |
| `httpRequest.requestUrl` | string | Request URL (typically just the path part of the URL) |
| `httpRequest.status` | number | HTTP status code |
| `httpRequest.userAgent` | string | Contents of the `User-Agent` header |
| `jsonPayload.user` | string | Authenticated username for the request |
| `jsonPayload.sc_substatus` | number | The substatus error code |
| `jsonPayload.sc_win32_status` | number | The Windows status code |
| `jsonPayload.time_taken` | number | The length of time that the action took, in milliseconds |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received |

Any fields that are blank or missing will not be present in the log entry.