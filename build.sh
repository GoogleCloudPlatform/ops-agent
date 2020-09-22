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
# TODO: Get version number from packaging
if [ -z "$version" ]; then
  version=0.1
fi

set -x -e

if [ -z "$DESTDIR" ]; then
  DESTDIR=$(mktemp -d)
fi

function build_collectd() {
  cd vendor/collectd
  autoreconf -f -i
  ./configure --prefix=$prefix/lib/collectd \
    --with-useragent="stackdriver_ops_agent_collectd/$version" \
    --with-data-max-name-len=256 \
    --disable-all-plugins \
    --disable-static \
    --disable-perl --without-libperl  --without-perl-bindings \
    --enable-cpu \
    --enable-df \
    --enable-disk \
    --enable-load \
    --enable-logfile \
    --enable-logging-metrics \
    --enable-memory \
    --enable-swap \
    --enable-syslog \
    --enable-interface \
    --enable-tcpconns \
    --enable-write_http \
    --enable-aggregation \
    --enable-csv \
    --enable-nginx \
    --enable-apache \
    --enable-memcached \
    --enable-mysql \
    --enable-protocols \
    --enable-plugin_mem \
    --enable-processes \
    --enable-python \
    --enable-ntpd \
    --enable-nfs \
    --enable-stackdriver_agent \
    --enable-exec \
    --enable-tail \
    --enable-statsd \
    --enable-network \
    --enable-match_regex --enable-target_set \
    --enable-target_replace --enable-target_scale \
    --enable-match_throttle_metadata_keys \
    --enable-write_log \
    --enable-unixsock \
    --enable-java --with-java=/usr/lib/jvm/default-java \
    --enable-redis --with-libhiredis \
    --enable-curl \
    --enable-curl_json \
    --enable-write_gcm \
    --enable-debug \
    --enable-docker
  make -j8
  make DESTDIR="$DESTDIR" install
}

function build_fluentbit() {
  cd vendor/fluent-bit
  mkdir -p build
  cd build
  cmake .. -DCMAKE_INSTALL_PREFIX=$prefix/lib/fluent-bit
  make -j8
  make DESTDIR="$DESTDIR" install
  # We don't want fluent-bit's service
  rm "$DESTDIR/lib/systemd/system/fluent-bit.service"
  rm -r "$DESTDIR/$prefix/lib/fluent-bit/etc"
}

function build_opsagent() {
  mkdir -p "$DESTDIR$prefix/libexec"
  go build -o "$DESTDIR$prefix/libexec/generate_config" github.com/Stackdriver/unified_agents/cmd/generate_config
}

function build_systemd() {
  mkdir -p "$DESTDIR$systemdsystemunitdir"
  for i in systemd/*; do
    sed "s|@PREFIX@|$prefix|g; s|@SYSCONFDIR@|$sysconfdir|g" $i > "$DESTDIR$systemdsystemunitdir/$(basename "$i")"
  done
}

(build_collectd)
(build_fluentbit)
(build_opsagent)
(build_systemd)
# TODO: Build sample config file
mkdir -p "$DESTDIR/$sysconfdir/google-cloud-ops-agent/"
touch "$DESTDIR/$sysconfdir/google-cloud-ops-agent/config.yaml"

# N.B. Don't include $DESTDIR itself in the tarball, since mktemp -d will create it mode 0700.
(cd "$DESTDIR" && tar -czf /tmp/google-cloud-ops-agent.tgz *)
