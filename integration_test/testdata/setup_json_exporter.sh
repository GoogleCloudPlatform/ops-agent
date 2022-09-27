set -e

wget -O json_exporter.tar.gz \
    https://github.com/prometheus-community/json_exporter/releases/download/v0.5.0/json_exporter-0.5.0.linux-amd64.tar.gz
tar -xvf json_exporter.tar.gz -C $WORKDIR/

pythonCmd="cd $WORKDIR/ && python3 -m http.server 8000"
exporterCmd="cd $WORKDIR/ && ./json_exporter-0.5.0.linux-amd64/json_exporter --config.file $WORKDIR/json_exporter_config.yaml"

if command -v tmux &> /dev/null
then
    tmux new-session -d -s python-server "$pythonCmd" 
    tmux new-session -d -s json-exporter "$exporterCmd"
else
    screen -dm -S python-server bash -c "$pythonCmd" 
    screen -dm -S json-exporter bash -c "$exporterCmd"
fi

