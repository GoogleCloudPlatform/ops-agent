# `wildfly` Metrics Receiver

The `wildfly` metrics receiver can fetch stats from a WildFly server's Java Virtual Machine (JVM) via [JMX](https://www.oracle.com/java/technologies/javase/javamanagement.html).

## Prerequisites
To expose the JMX endpoint remotely, you should also set the `jboss.bind.address.management` system property. By default, this property is set in WildFly's configuration. The default WildFly installation requires no JMX authentication with JMX exposed locally on 127.0.0.1:9990.

In addition, in order for the session metrics to be active, statistics need to be enabled on the undertow subsystem. One way of doing this is through using the JBoss CLI and running this command `/subsystem=undertow:write-attribute(name=statistics-enabled,value=true)`

## Configuration

| Field                 | Default            | Description |
| ---                   | ---                | ---         |
| `type`                | required           | Must be `wildfly`. |
| `endpoint`            | `service:jmx:remote+http://localhost:9990` | The [JMX Service URL](https://docs.oracle.com/javase/8/docs/api/javax/management/remote/JMXServiceURL.html) or host and port used to construct the Service URL. Must be in the form of `host:port`. Values in `host:port` form will be used to create a Service URL of `service:jmx:remote+http://<host>:<port>/jmxrmi`. |
| `username`            | not set by default | The configured username if JMX is configured to require authentication. |
| `password`            | not set by default | The configured password if JMX is configured to require authentication. |
| `collection_interval` | `60s`              | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |
| `additional_jars`     | `/opt/wildfly/bin/client/jboss-client.jar` | This should point to the jboss-client.jar which is required in order to monitor WildFly through JMX.

Example Configuration:

```yaml
metrics:
  receivers:
    wildfly:
      type: wildfly
      collection_interval: 60s
  service:
    pipelines:
      wildfly:
        receivers:
          - wildfly
```

## Metrics
The Ops Agent collects the following metrics from WildFly.
Note: In order for the session metrics to be active, statistics need to be enabled on the undertow subsystem.

| Metric                                               | Data Type      | Unit        | Labels                         | Description |
| ---                                                  | ---            | ---         | ---                            | ---         | 
| workload.googleapis.com/wildfly.session.count        | Cumulative     | sessions    | deployment                     | The number of sessions created. |
| workload.googleapis.com/wildfly.session.active       | Gauge          | sessions    | deployment                     | The number of currently active sessions. |
| workload.googleapis.com/wildfly.session.expired      | Cumulative     | sessions    | deployment                     | The number of sessions that have expired. |
| workload.googleapis.com/wildfly.session.rejected     | Cumulative     | sessions    | deployment                     | The number of sessions that have rejected. |
| workload.googleapis.com/wildfly.request.count        | Cumulative     | requests    | server, listener               | The number of requests received. |
| workload.googleapis.com/wildfly.request.time         | Cumulative     | ns          | server, listener               | The total amount of time spent on requests. |
| workload.googleapis.com/wildfly.request.server_error | Cumulative     | requests    | server, listener               | The number of requests that have resulted in a 5xx response. |
| workload.googleapis.com/wildfly.network.io           | Cumulative     | by          | server, listener, state        | The number of bytes transmitted. |
| workload.googleapis.com/wildfly.jdbc.connection.open | Gauge          | connections | data_source, state             | The number of open jdbc connections. |
| workload.googleapis.com/wildfly.jdbc.request.wait    | Cumulative     | requests    | data_source                    | The number of jdbc connections that had to wait before opening. |
| workload.googleapis.com/wildfly.jdbc.transaction.count | Cumulative   | transactions |                               | The number of transactions created. |
| workload.googleapis.com/wildfly.jdbc.rollback.count    | Cumulative   | transactions | cause                         | The number of transactions rolled back. |

# `wildfly_system` Logging Receiver

## Configuration

To configure a receiver for your wildfly server logs, specify the following fields:

| Field                 | Default                           | Description |
| ---                   | ---                               | ---         |
| `type`                | required                          | Must be `wildfly_system`. |
| `include_paths`       | `[/opt/wildfly/standalone/log/server.log, /opt/wildfly/domain/servers/*/log/server.log]` | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `/var/log/wildfly*/*.log`.
| `exclude_paths`       | `[]`                              | A list of filesystem path patterns to exclude from the set matched by `include_paths`.
| `wildcard_refresh_interval` | `60s` | The interval at which wildcard file paths in include_paths are refreshed. Given as a time duration, for example 30s, 2m. This property might be useful under high logging throughputs where log files are rotated faster than the default interval. Must be a multiple of 1s.|

Example Configuration:

```yaml
logging:
  receivers:
    wildfly_system:
      type: wildfly_system
  service:
    pipelines:
      wildfly_system:
        receivers:
          - wildfly_system
```

## Logs

Server logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received |
| `jsonPayload.thread` | string | Thread where the log originated |
| `jsonPayload.source` | string | Source where the log originated |
| `jsonPayload.messageCode` | string | Wildfly specific message code preceding the log, where applicable |
| `jsonPayload.message` | string | Log message |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |

Any fields that are blank or missing will not be present in the log entry.
