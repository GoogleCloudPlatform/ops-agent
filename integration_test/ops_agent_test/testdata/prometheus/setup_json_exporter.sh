set -e

declare -A ARCHMAP=([x86_64]="amd64" [aarch64]="arm64")
ARCH="${ARCHMAP[$(uname -m)]}"

# Copy JSON exporter from GCS and extract to /opt/
gcloud storage cp \
    "gs://ops-agents-public-buckets-vendored-deps/mirrored-content/github.com/prometheus-community/json_exporter/releases/download/v0.5.0/json_exporter-0.5.0.linux-${ARCH}.tar.gz" \
    json_exporter.tar.gz
sudo mkdir -p /opt/json_exporter
sudo tar -xzf json_exporter.tar.gz -C /opt/json_exporter --strip-components 1
sudo systemctl daemon-reload

# Start a Go http server serving files in /opt/go-http-server/ on port 8000
sudo systemctl enable http-server-for-prometheus-test
sudo systemctl restart http-server-for-prometheus-test

# Start the JSON exporter with uploaded config yaml in /opt/json_exporter/json_exporter_config.yaml 
sudo systemctl enable json-exporter-for-prometheus-test
sudo systemctl restart json-exporter-for-prometheus-test