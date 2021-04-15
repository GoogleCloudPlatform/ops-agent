#!/bin/bash

# Copyright 2020 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Package build scripts can use this by setting $DESTDIR before launching the
# script.

prefix=/opt/google-cloud-ops-agent
sysconfdir=/etc
systemdsystemunitdir=$(pkg-config systemd --variable=systemdsystemunitdir)
systemdsystempresetdir=$(pkg-config systemd --variable=systemdsystempresetdir)
subagentdir=$prefix/subagents

. VERSION

set -x -e

export PATH=/usr/local/go/bin:$PATH

if [ -z "$DESTDIR" ]; then
  DESTDIR=$(mktemp -d)
fi

function build_otel() {
  cd submodules/opentelemetry-operations-collector
  mkdir -p "$DESTDIR$subagentdir/opentelemetry-collector"
  go build -o "$DESTDIR$subagentdir/opentelemetry-collector/otelopscol" ./cmd/otelopscol
}

function build_fluentbit() {
  cd submodules/fluent-bit
  mkdir -p build
  cd build
  cmake .. -DCMAKE_INSTALL_PREFIX=$subagentdir/fluent-bit -DFLB_HTTP_SERVER=On -DCMAKE_BUILD_TYPE=RELWITHDEBINFO
  make -j8
  make DESTDIR="$DESTDIR" install
  # We don't want fluent-bit's service
  rm "$DESTDIR/lib/systemd/system/fluent-bit.service"
  rm -r "$DESTDIR$subagentdir/fluent-bit/etc"
}

function build_opsagent() {
  mkdir -p "$DESTDIR$prefix/libexec"
  go build -o "$DESTDIR$prefix/libexec/google_cloud_ops_agent_engine" github.com/GoogleCloudPlatform/ops-agent/cmd/google_cloud_ops_agent_engine
}

function build_systemd() {
  function install_unit() {
    # $1 = source path; $2 = destination path relative to the unit directory
    sed "s|@PREFIX@|$prefix|g; s|@SYSCONFDIR@|$sysconfdir|g" "$1" > "$DESTDIR$systemdsystemunitdir/$2"
  }
  mkdir -p "$DESTDIR$systemdsystemunitdir"
  for f in systemd/*.service systemd/*.target; do
    install_unit "$f" "$(basename "$f")"
  done
  if [ "$(systemctl --version | grep -Po '^systemd \K\d+')" -lt 240 ]; then
    for d in systemd/*.service.d; do
      mkdir "$DESTDIR$systemdsystemunitdir/$(basename "$d")"
      for f in "$d"/*.conf; do
        install_unit "$f" "$(basename "$d")/$(basename "$f")"
      done
    done
  fi
  mkdir -p "$DESTDIR$systemdsystempresetdir"
  for f in systemd/*.preset; do
    cp "$f" "$DESTDIR$systemdsystempresetdir/$(basename "$f")"
  done
}

(build_otel)
(build_fluentbit)
(build_opsagent)
(build_systemd)
# TODO: Build sample config file
mkdir -p "$DESTDIR/$sysconfdir/google-cloud-ops-agent/"
cp "confgenerator/default-config.yaml" "$DESTDIR/$sysconfdir/google-cloud-ops-agent/config.yaml"

# N.B. Don't include $DESTDIR itself in the tarball, since mktemp -d will create it mode 0700.
(cd "$DESTDIR" && tar -czf /tmp/google-cloud-ops-agent.tgz *)
