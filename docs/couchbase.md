# Couchbase

Supported telemetry types: metrics and logs

## Metrics

The `couchbase` integration uses the builtin [Prometheus exporter](https://docs.couchbase.com/cloud-native-database/prometheus-overview.html) running on Couchbase 7.0 by default. The metrics are retrieved from this endpoint and then will be transformed to be ingested by Google Cloud.

### Configuration

| Field                 | Default          | Description                                                                              |
| --------------------- | ---------------- | ---------------------------------------------------------------------------------------- |
| `type`                | required         | Must be `couchbase`.                                                                     |
| `endpoint`            | `localhost:8091` | The address of the couchbase node that exposes the prometheus exporter metrics endpoint. |
| `username`            | required         | The configured username to authenticate to couchbase.                                    |
| `password`            | required         | The configured password to authenticate to couchbase.                                    |
| `collection_interval` | `60s`            | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`.   |

Example Configuration:

```yaml
metrics:
  receivers:
    couchbase:
      type: couchbase
      username: opsuser
      password: password
  service:
    pipelines:
      couchbase:
        receivers:
          - couchbase
```

## Logs

There are three types of logs monitored by the couchbase integration:

- couchbase_general
- couchbase_http_access
- couchbase_xdcr

### Couchbase General Logs

General logs are primarily found in these filepaths on linux:

- /opt/couchbase/var/lib/couchbase/logs/couchdb.log
- /opt/couchbase/var/lib/couchbase/logs/info.log
- /opt/couchbase/var/lib/couchbase/logs/debug.log
- /opt/couchbase/var/lib/couchbase/logs/error.log
- /opt/couchbase/var/lib/couchbase/logs/babysitter.log

These logs are generally useful for diagnosiing overall activity of the couchbase cluster and bucket actions.

| LogEntry Field      | Example Value                                                       |
| ------------------- | ------------------------------------------------------------------- |
| severity            | error                                                               |
| timestamp           | 2021-12-13T13:35:44.135Z                                            |
| jsonPayload.node    | cb.local                                                            |
| jsonPayload.node    | ns_1                                                                |
| jsonPayload.type    | ns_server                                                           |
| jsonPayload.source  | <0.23294.248>:compaction_daemon:spawn_scheduled_views_compactor:548 |
| jsonPayload.message | Start compaction of indexes for bucket test_bucket with config:     |

```yaml
logs:
  receivers:
    couchbase_general:
      type: couchbase_general
  service:
    pipelines:
      couchbase_general:
        receivers:
          - couchbase_general
```

### Couchbase HTTP Access Logs

HTTP Access Logs are generally found here on linux:

- /opt/couchbase/var/lib/couchbase/logs/http_access.log
- /opt/couchbase/var/lib/couchbase/logs/http_access_ionternal.log

These logs contain information about a lot of RESTful activity when it comes to couchbase and provides `LogEntry`'s that look similar to this.

| LogEntry Field      | Example Value                                                                                                                                                              |
| ------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| jsonPayload.host    | 10.66.8.2                                                                                                                                                                  |
| jsonPayload.user    | otelu                                                                                                                                                                      |
| timestamp           | 21/Dec/2021:13:49:17 +0000                                                                                                                                                 |
| method              | GET                                                                                                                                                                        |
| jsonPayload.path    | /pools/default?etag=95600158&waitChange=10000                                                                                                                              |
| jsonPayload.client  | Go-http-client/1.1                                                                                                                                                         |
| jsonPayload.code    | 200                                                                                                                                                                        |
| jsonPayload.size    | 7887                                                                                                                                                                       |
| jsonPayload.message | "http://10.33.120.15:8091/ui/index.html" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.110 Safari/537.36" 10007 |

Example Configuration:

```yaml
logs:
  receivers:
    couchbase_http_access:
      type: couchbase_http_access
  service:
    pipelines:
      couchbase:
        receivers:
          - couchbase_http_access
```

### Couchbase Cross Datacenter Logs

Cross Datacenter Logs are generally found here on linux:

- /opt/couchbase/var/lib/couchbase/logs/goxdcr.log

Usually containing information about classes used in cross datacenter actions.

| LogEntry Field       | Example Value                                                                                                                                                                                                                       |
| -------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| severity             | INFO                                                                                                                                                                                                                                |
| timestamp            | 2022-01-03T17:43:58.178Z                                                                                                                                                                                                            |
| jsonPayload.log_type | GOXDCR.ResourceMgr                                                                                                                                                                                                                  |
| jsonPayload.message  | Resource Manager State = overallTP: 0 highTP: 0 highExist: false lowExist: false backlogExist: false maxTP: 0 highTPNeeded: 0 highTokens: 0 maxTokens: 0 lowTPLimit: 0 calibration: None dcpAction: Reset processCpu: 0 idleCpu: 85 |

```yaml
logs:
  receivers:
    couchbase_xdcr:
      type: couchbase_xdcr
  service:
    pipelines:
      couchbase:
        receivers:
          - couchbase_xdcr
```

Example with all 3 configured

```yaml
logging:
  receivers:
    couchbase_general:
      type: couchbase_general
    couchbase_http_access:
      type: couchbase_http_access
    couchbase_goxdcr:
      type: couchbase_goxdcr
  service:
    pipelines:
      couchbase:
        receivers:
          - couchbase_general
          - couchbase_http_access
          - couchbase_goxdcr
```
