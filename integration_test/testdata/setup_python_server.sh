set -e

# Start a python http server in the WORKDIR
# In order to have the server runs in the background, redirect all stdin, stdout and sederr
nohup bash -c "cd $WORKDIR && python3 -u -m http.server 8000" \
    >$WORKDIR/python-server.log 2>$WORKDIR/python-server.err </dev/null &