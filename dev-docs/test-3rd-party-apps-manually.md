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
curl https://raw.githubusercontent.com/GoogleCloudPlatform/ops-agent/master/integration_test/third_party_apps_data/applications/nginx/debian_ubuntu/install > install.sh
sudo bash install.sh
```

### 4. Configure the application to expose metrics

This step might be a no-op for some applications.

The commands should be available at `https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/applications/{{APPLICATION_NAME}}/exercise` (e.g. https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/applications/nginx/exercise). To browse all supported applications, go to https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/applications.

Take Nginx for example:
```
curl https://raw.githubusercontent.com/GoogleCloudPlatform/ops-agent/master/integration_test/third_party_apps_data/applications/nginx/exercise > exercise.sh
sudo bash exercise.sh
```

### 5. Configure and restart Ops Agent

The commands should be available at `https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/agent/ops-agent/linux/enable_{{APPLICATION_NAME}}` (e.g. https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/agent/ops-agent/linux/enable_nginx). To browse all supported applications, go to https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/agent/ops-agent/linux.

Take Nginx for example:
```
curl https://raw.githubusercontent.com/GoogleCloudPlatform/ops-agent/master/integration_test/third_party_apps_data/applications/nginx/enable > enable.sh
sudo bash enable.sh
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

