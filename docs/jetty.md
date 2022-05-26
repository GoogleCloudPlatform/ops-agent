# Jetty

Follow the [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/jetty) for instructions to collect metrics from this application using Ops Agent.

# `jetty` Metrics Receiver

The `jetty` metric receiver can fetch stats from a Jetty server's Java Virtual Machine (JVM) via [JMX](https://www.oracle.com/java/technologies/javase/javamanagement.html).
## Prerequisites

In order to expose a JMX endpoint, you must set the `com.sun.management.jmxremote.port` system property. It is recommended to also set the `com.sun.management.jmxremote.rmi.port` system property to the same port. To expose a JMX endpoint remotely, you must also set the `java.rmi.server.hostname` system property. You should set these properties when running the `start.jar` file when starting the server. Also, the `jmx` module needs to be enabled on the jetty server.

## Configuration

| Field                          | Default            | Description      |
| ---                            | ---                | ---              |
| `type`                         | required           | Must be `jetty`. |
| `endpoint`                     | `localhost:1099`   | The [JMX Service URL](https://docs.oracle.com/javase/8/docs/api/javax/management/remote/JMXServiceURL.html) or host and port used to construct the Service URL. Must be in the form of `service:jmx:<protocol>:<sap>` or `host:port`. Values in `host:port` form will be used to create a Service URL of `service:jmx:rmi:///jndi/rmi://<host>:<port>/jmxrmi`. |
| `collect_jvm_metrics`          | true               | Should the set of support [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics) also be collected |
| `username`                     | not set by default | The configured username if JMX is configured to require authentication. |
| `password`                     | not set by default | The configured password if JMX is configured to require authentication. |
| `collection_interval`          | `60s`              | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |

Example Configuration:

```yaml
metrics:
   receivers:
      jetty:
         type: jetty
   service:
      pipelines:
         jetty:
            receivers:
               - jetty
```

## Metrics
In addition to Jetty specific metrics, by default Jetty will also report [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics)

| Metric                     | Data Type  | Unit           | Labels       | Description |
| ---                        | ---        | ---            | ---          | ---   |  
| `jetty.select.count`       | cumulative | `{operations}` |              | The number of select calls. |
| `jetty.session.count`      | cumulative | `{sessions}`   | resource     | The number of sessions created. |
| `jetty.session.time.total` | gauge      | `s`            | resource     | The total time sessions have been active. |
| `jetty.session.time.max`   | gauge      | `s`            | resource     | The maximum amount of time a session has been active. |
| `jetty.thread.count`       | gauge      | `{threads}`    | state        | The current number of threads. |
| `jetty.thread.queue.count` | gauge      | `{threads}`    |              | The current number of threads in the queue. |


# `jetty_access` Logging Receiver

## Prerequisites
Access logs have to be configured to log. See [documentation](https://www.eclipse.org/jetty/documentation/jetty-9/index.html#configuring-jetty-logging)

## Configuration

To configure a receiver for your Jetty Access logs, specify the following fields:

| Field                 | Default                           | Description |
| ---                   | ---                               | ---         |
| `type`                | required                          | Must be `jetty_access`. |
| `include_paths`       | `['/opt/logs/*.request.log']` | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `/opt/logs/*.request.log`. |
| `exclude_paths`       | `[]`                              | A list of filesystem path patterns to exclude from the set matched by `include_paths`.
| `wildcard_refresh_interval` | `60s` | The interval at which wildcard file paths in `include_paths` are refreshed. Specified as a time interval parsable by [time.ParseDuration](https://pkg.go.dev/time#ParseDuration). Must be a multiple of 1s.|

Example Configuration:

```yaml
logging:
  receivers:
    jetty_access:
      type: jetty_access
  service:
    pipelines:
      jetty:
        receivers:
          - jetty_access
```

### Access Logs

Access logs contain the [`httpRequest` field](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#httprequest):

Any fields that are blank or missing will not be present in the log entry.

| Field | Type | Description |
| ---   | ---- | ----------- |
| `httpRequest.protocol` | string | Protocol used for the request |
| `httpRequest.referer` | string | Contents of the `Referer` header |
| `httpRequest.requestMethod` | string | HTTP method |
| `httpRequest.requestUrl` | string | Request URL (typically just the path part of the URL) |
| `httpRequest.responseSize` | string (`int64`) | Response size |
| `httpRequest.status` | number | HTTP status code |
| `httpRequest.userAgent` | string | Contents of the `User-Agent` header |
| `jsonPayload.host` | string | Contents of the `Host` header |
| `jsonPayload.user` | string | Authenticated username for the request |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received |

### System Logs

System logs are collected by default in the syslog receiver.