# `apache httpd` Metrics Receiver

The httpd receiver can retrieve stats from your Apache web server using the `/server-status?auto` endpoint.


## Prerequisites

The receiver requires that you enable the [mod_status module](https://httpd.apache.org/docs/2.4/mod/mod_status.html) on your Apache web server.

 This requires adding the following lines to the server’s `httpd.conf` file:

```
<Location "/server-status">
    SetHandler server-status
    Require host example.com
</Location>
```

## Configuration

Following the guide for [Configuring the Ops Agent](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration#file-location), add the required elements for your httpd configuration.

To configure a receiver for your httpd metrics, specify the following fields:

| Field                 | Default                   | Description |
| ---                   | ---                       | ---         |
| `type`                | required                  | Must be `httpd`. |
| `server_status_url`     | `http://localhost:8080/server-status?auto` | The url exposed by the `mod_status` module. |
| `collection_interval` | `60s`                     | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |

Example Configuration:

```yaml
metrics:
  receivers:
    httpd_metrics:
      type: httpd
      server_status_url: http://localhost:8080/server-status?auto
      collection_interval: 30s
  service:
    pipelines:
      httpd_pipeline:
        receivers:
          - httpd_metrics
```

## Metrics

The Ops Agent collects the following metrics from your Apache web servers.

| Metric                                            | Data Type | Unit        | Labels              | Description |
| ---                                               | ---       | ---         | ---                 | ---         | 
| workload.googleapis.com/httpd.current_connections | gauge     | connections |      server_name        | The number of active connections currently attached to the HTTP server.  |
| workload.googleapis.com/httpd.uptime              | sum       | s           |     server_name     | The amount of time that the server has been running in seconds.  |
| workload.googleapis.com/httpd.requests            | sum       | 1    |    server_name         | Total requests serviced by the HTTP server.  |
| workload.googleapis.com/httpd.workers             | gauge     | connections | server_name, workers_state     | The number of workers currently attached to the HTTP server |
| workload.googleapis.com/httpd.scoreboard          | gauge     | scoreboard  | server_name, scoreboard_state  | Apache HTTP server scoreboard values. |
| workload.googleapis.com/httpd.traffic             | sum       | By |     server_name     | Total HTTP server traffic. |