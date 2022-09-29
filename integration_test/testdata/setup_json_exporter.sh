set -e

wget -O json_exporter.tar.gz \
    https://github.com/prometheus-community/json_exporter/releases/download/v0.5.0/json_exporter-0.5.0.linux-amd64.tar.gz
tar -xvf json_exporter.tar.gz -C $WORKDIR/

nohup bash -c "cd $WORKDIR && python3 -u -m http.server 8000" >python-server.log 2>python-server.err </dev/null &
echo "Python server started."
nohup $WORKDIR/json_exporter-0.5.0.linux-amd64/json_exporter --config.file $WORKDIR/json_exporter_config.yaml >json-exporter.log 2>json-exporter.err </dev/null & 
echo "JSON exporter started."
