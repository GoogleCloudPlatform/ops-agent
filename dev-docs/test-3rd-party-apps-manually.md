## Test collecting logs and metrics from a 3rd party application manually

### 1. Create a VM and SSH into it

```
gcloud compute instances create --zone us-central1-a --image-project debian-cloud --image-family debian-10 test-app
gcloud compute ssh test-app
```

### 2. Install Ops Agent

The following commands are from https://cloud.devsite.corp.google.com/monitoring/agent/ops-agent/installation#agent-install-latest-linux

```
curl -sSO https://dl.google.com/cloudagents/add-google-cloud-ops-agent-repo.sh
sudo bash add-google-cloud-ops-agent-repo.sh --also-install
```

### 3. Install the application

The commands should be available at `https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/applications/{{APPLICATION_NAME}}/{{DISTRO_NAME}}/install` (e.g. https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/applications/nginx/debian_ubuntu/install). To browse all supported applications, go to https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/applications.

Take Nginx for example:
```
sudo apt update
sudo apt install -y nginx
```

### 4. Configure the application to expose metrics

This step might be a no-op for some applications.

The commands should be available at `https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/applications/{{APPLICATION_NAME}}/{{DISTRO_NAME}}/post` (e.g. https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/applications/nginx/debian_ubuntu/post). To browse all supported applications, go to https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/applications.

Take Nginx for example:
```
sudo tee /etc/nginx/conf.d/status.conf > /dev/null << EOF
server {
    listen 80;
    server_name 127.0.0.1;
    location /nginx_status {
        stub_status on;
        access_log off;
        allow 127.0.0.1;
        deny all;
    }
    location / {
        root /dev/null;
    }
}
EOF
sudo service nginx reload
curl http://127.0.0.1:80/nginx_status
```

output
```
Active connections: 1
server accepts handled requests
 9 9 9
Reading: 0 Writing: 1 Waiting: 0
```

### 5. Configure and restart Ops Agent

The commands should be available at `https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/agent/ops-agent/linux/enable_{{APPLICATION_NAME}}` (e.g. https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/agent/ops-agent/linux/enable_nginx). To browse all supported applications, go to https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/agent/ops-agent/linux.

Take Nginx for example:
```
sudo tee /etc/google-cloud-ops-agent/config.yaml > /dev/null << EOF
logging:
  receivers:
    nginx_access:
      type: nginx_access
    nginx_error:
      type: nginx_error
  service:
    pipelines:
      nginx:
        receivers:
          - nginx_access
          - nginx_error
metrics:
  receivers:
    nginx_metrics:
      type: nginx
      stub_status_url: http://127.0.0.1:80/nginx_status
      collection_interval: 30s
  service:
    pipelines:
      nginx:
        receivers:
          - nginx_metrics
EOF
sudo service google-cloud-ops-agent restart
```

### 6. Verify metrics

Go to [Metrics Explorer](https://console.cloud.google.com/monitoring/metrics-explorer) and use a query similar to the following in the `MQL` tab. A sample metrics name for a specific application can be found at `https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/applications/{{APPLICATION_NAME}}/metric_name.txt` (e.g. https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/applications/nginx/metric_name.txt). To browse all supported applications, go to https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/applications.


Take Nginx for example:
```
fetch gce_instance
| metric 'workload.googleapis.com/nginx.requests'
| align rate(1m)
| every 1m
```

### 7. Verify logs

Go to the [Log Viewer](https://console.cloud.google.com/logs/viewer) and use a query like the following to query for an application's logs. A sample `log_name` for a specific application can be found at `https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/applications/{{APPLICATION_NAME}}/expected_logs.yaml` (e.g. https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/applications/nginx/expected_logs.yaml). To browse all supported applications, go to https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/applications.

Take Nginx for example:
```
resource.type="gce_instance"
logName=("projects/{PROJECT_ID}/logs/nginx_error" OR "projects/{PROJECT_ID}/logs/nginx_access")
```

