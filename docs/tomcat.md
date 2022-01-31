# `tomcat` Metrics Receiver

The `tomcat` metrics receiver can fetch stats from a Tomcat server's Java Virtual Machine (JVM) via [JMX](https://www.oracle.com/java/technologies/javase/javamanagement.html).

## Prerequisites

In order to expose a JMX endpoint, you must set the `com.sun.management.jmxremote.port` system property. It is recommended to also set the `com.sun.management.jmxremote.rmi.port` system property to the same port. To expose JMX endpoint remotely, you must also set the `java.rmi.server.hostname` system property. By default, these properties are set in a Tomcat deployment's tomcat-env.sh file and the default Tomcat installation requires no JMX authentication with JMX exposed locally on 127.0.0.1:8050.

## Configuration

| Field                 | Default            | Description |
| ---                   | ---                | ---         |
| `type`                | required           | Must be `tomcat`. |
| `endpoint`            | `localhost:8050`   | The [JMX Service URL](https://docs.oracle.com/javase/8/docs/api/javax/management/remote/JMXServiceURL.html) or host and port used to construct the Service URL. Must be in the form of `service:jmx:<protocol>:<sap>` or `host:port`. Values in `host:port` form will be used to create a Service URL of `service:jmx:rmi:///jndi/rmi://<host>:<port>/jmxrmi`. |
| `collect_jvm_metrics` | true               | Should the set of support [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics) also be collected |
| `username`            | not set by default | The configured username if JMX is configured to require authentication. |
| `password`            | not set by default | The configured password if JMX is configured to require authentication. |
| `collection_interval` | `60s`              | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |


Example Configuration:

```yaml
metrics:
  receivers:
    tomcat_metrics:
      type: tomcat
      collection_interval: 60s
  service:
    pipelines:
      tomcat_pipeline:
        receivers:
          - tomcat_metrics
```

## Metrics
In addition to Tomcat specific metrics, by default Tomcat will also report [JVM metrics](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/docs/jvm.md#metrics)

| Metric                                               | Data Type      | Unit        | Labels                         | Description |
| ---                                                  | ---            | ---         | ---                            | ---         | 
| workload.googleapis.com/tomcat.sessions              | Gauge          | sessions    |                                | The number of active sessions. |
| workload.googleapis.com/tomcat.errors                | Cumulative     | errors      | proto_handler                  | The number of errors encountered. |
| workload.googleapis.com/tomcat.processing_time       | Cumulative     | ms          | proto_handler                  | The total processing time. |
| workload.googleapis.com/tomcat.traffic               | Cumulative     | by          | proto_handler, direction       | The number of bytes transmitted and received. |
| workload.googleapis.com/tomcat.threads               | Gauge          | threads     | proto_handler, state           | The number of threads. |
| workload.googleapis.com/tomcat.max_time              | Gauge          | ms          | proto_handler                  | Maximum time to process a request. |
| workload.googleapis.com/tomcat.request_count         | Cumulative     | requests    | proto_handler                  | The total requests. |



# `tomcat_system` and `tomcat_access` Logging Receivers

## Configuration

To configure a receiver for your tomcat system logs, specify the following fields:

| Field                 | Default                           | Description |
| ---                   | ---                               | ---         |
| `type`                | required                          | Must be `tomcat_system`. |
| `include_paths`       | `[/var/log/tomcat*/catalina.out, /opt/tomcat/logs/catalina.out]` | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `/var/log/apache*/*.log`.
| `exclude_paths`       | `[]`                              | A list of filesystem path patterns to exclude from the set matched by `include_paths`.

To configure a receiver for your tomcat access logs, specify the following fields:

| Field                 | Default                                         | Description |
| ---                   | ---                                             | ---         |
| `type`                | required                                        | Must be `tomcat_access`. |
| `include_paths`       | `[/var/log/tomcat*/localhost_access_log.*.txt, /opt/tomcat/logs/localhost_access_log.*.txt]` | The log files to read. |
| `exclude_paths`       | `[]`                                            | Log files to exclude (if `include_paths` contains a glob or directory). |

Example Configuration:

```yaml
logging:
  receivers:
    tomcat_system:
      type: tomcat_system
    tomcat_access:
      type: tomcat_access
  service:
    pipelines:
      tomcat:
        receivers:
          - tomcat_system
          - tomcat_access
```

## Logs

System logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received |
| `jsonPayload.module` | string | Module of tomcat where the log originated |
| `jsonPayload.source` | string | source of where the log originated |
| `jsonPayload.message` | string | Log message, including detailed stacktrace where provided |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |

Any fields that are blank or missing will not be present in the log entry.

Access logs contain the [`httpRequest` field](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#httprequest):

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