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
# TODO: Get version number from packaging
if [ -z "$version" ]; then
  version=0.1
fi

set -x -e

if [ -z "$DESTDIR" ]; then
  DESTDIR=$(mktemp -d)
fi

function build_collectd() {
  cd submodules/collectd
  autoreconf -f -i
  ./configure --prefix=$subagentdir/collectd \
    --with-useragent="google-cloud-ops-agent-metrics/$version" \
    --with-data-max-name-len=256 \
    --disable-all-plugins \
    --disable-static \
    --enable-cpu \
    --enable-df \
    --enable-disk \
    --enable-load \
    --enable-logfile \
    --enable-memory \
    --enable-swap \
    --enable-syslog \
    --enable-interface \
    --enable-tcpconns \
    --enable-aggregation \
    --enable-protocols \
    --enable-plugin_mem \
    --enable-processes \
    --enable-stackdriver_agent \
    --enable-network \
    --enable-match_regex --enable-target_set \
    --enable-target_replace --enable-target_scale \
    --enable-match_throttle_metadata_keys \
    --enable-write_log \
    --enable-unixsock \
    --enable-write_gcm \
    --enable-debug
  make -j8
  make DESTDIR="$DESTDIR" install
}

function build_fluentbit() {
  cd submodules/fluent-bit
  mkdir -p build
  cd build
  cmake .. -DCMAKE_INSTALL_PREFIX=$subagentdir/fluent-bit -DFLB_HTTP_SERVER=On
  make -j8
  make DESTDIR="$DESTDIR" install
  # We don't want fluent-bit's service
  rm "$DESTDIR/lib/systemd/system/fluent-bit.service"
  rm -r "$DESTDIR$subagentdir/fluent-bit/etc"
}

function build_opsagent() {
  mkdir -p "$DESTDIR$prefix/libexec"
  go build -o "$DESTDIR$prefix/libexec/generate_config" github.com/Stackdriver/unified_agents/cmd/generate_config
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
  if [ "$(systemctl --version | grep -Po '^systemd \K\d+$')" -lt 240 ]; then
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

(build_collectd)
(build_fluentbit)
(build_opsagent)
(build_systemd)
# TODO: Build sample config file
mkdir -p "$DESTDIR/$sysconfdir/google-cloud-ops-agent/"
cp "confgenerator/default-config.yaml" "$DESTDIR/$sysconfdir/google-cloud-ops-agent/config.yaml"

# N.B. Don't include $DESTDIR itself in the tarball, since mktemp -d will create it mode 0700.
(cd "$DESTDIR" && tar -czf /tmp/google-cloud-ops-agent.tgz *)
