#!/bin/sh
set -x -e
DESTDIR=$1
mkdir -p "$DESTDIR/opt/google-cloud-ops-agent/libexec"
go build -buildvcs=false -o "$DESTDIR/opt/google-cloud-ops-agent/libexec/google_cloud_ops_agent_diagnostics" \
  github.com/GoogleCloudPlatform/ops-agent/cmd/google_cloud_ops_agent_diagnostics