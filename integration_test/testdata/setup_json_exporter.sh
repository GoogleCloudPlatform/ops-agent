set -e

wget -O json_exporter.tar.gz \
    https://github.com/prometheus-community/json_exporter/releases/download/v0.5.0/json_exporter-0.5.0.linux-amd64.tar.gz
tar -xvf json_exporter.tar.gz -C $WORKDIR/

echo 'cd $WORKDIR && python3 -m http.server 8000 &> python-server.log' | at now
echo "Python server started."
echo '$WORKDIR/json_exporter-0.5.0.linux-amd64/json_exporter --config.file $WORKDIR/json_exporter_config.yaml &> json-exporter.log &' | at now
echo "JSON exporter started."
