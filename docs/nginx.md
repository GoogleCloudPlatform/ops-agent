# nginx Receiver

The nginx receiver can retrieve stats from your nginx instance using the `mod_status` endpoint.


## Prerequisites

The receiver requires that you enable the [status module](http://nginx.org/en/docs/http/ngx_http_stub_status_module.html) on your nginx instance.

To enable the status module, complete the following steps:

1. Edit the `status.conf` file. You can find this file in the nginx configuration directory, typically found at `/etc/nginx/conf.d`.
2. Add the following lines to your configuration file:

   ```
   location /status {
       stub_status on;
   }
   ```

    1. Alternately, you can append these lines to your `nginx.conf` file, which is typically located in one of the following directories: `/etc/nginx`, `/usr/local/nginx/conf`, or `/usr/local/etc/nginx`.

   Your configuration file might look like the following example:
   ```
   server {
       listen 80;
       server_name mynginx.domain.com; 
       location /status {
           stub_status on;
       }
       location / {
           root /dev/null;  
       }
   }
   ```

3. Reload the nginx configuration:

   ```
   sudo service nginx reload
   ```


## Configuration

Following the guide for [Configuring the Ops Agent](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration#file-location), add the required elements for your nginx configuration.

To configure a receiver for your nginx metrics, specify the following fields:

| Field                 | Default                   | Description |
| ---                   | ---                       | ---         |
| `type`                | required                  | Must be `nginx`. |
| `stub_status_url`     | `http://localhost/status` | The url exposed by the nginx stats module. |
| `collection_interval` | `60s`                     | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |

Example Configuration:

```yaml
metrics:
  receivers:
    nginx_metrics:
      type: nginx
      stub_status_url: http://localhost:80/status
      collection_interval: 30s
  service:
    pipelines:
      nginxpipeline:
        receivers:
          - nginx_metrics
```

## Metrics

The Ops Agent collects the following metrics from your nginx instances.

| Metric                                           | Data Type | Unit        | Labels | Description |
| ---                                              | ---       | ---         | ---    | ---         | 
| workload.googleapis.com/nginx.requests             | sum       | requests    |        | Total number of requests made to the server. |
| workload.googleapis.com/nginx.connections_accepted | sum       | connections |        | Total number of accepted client connections. |
| workload.googleapis.com/nginx.connections_handled  | sum       | connections |        | Total number of handled connections. |
| workload.googleapis.com/nginx.connections_current  | gauge     | connections | state  | Current number of connections. |
