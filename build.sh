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
if [ -z "$BUILD_DISTRO" ]; then
  release_version="$(lsb_release -rs)" #e.g. 9.13 for debian, 8.3.2011 for centos
  BUILD_DISTRO=$(lsb_release -is | tr '[:upper:]' '[:lower:]')${release_version%%.}
fi
if [ -z "$CODE_VERSION" ]; then
  CODE_VERSION=${PKG_VERSION}
fi
BUILD_INFO_IMPORT_PATH="github.com/GoogleCloudPlatform/ops-agent/internal/version"
BUILD_X1="-X ${BUILD_INFO_IMPORT_PATH}.BuildDistro=${BUILD_DISTRO}"
BUILD_X2="-X ${BUILD_INFO_IMPORT_PATH}.Version=${CODE_VERSION}"
LD_FLAGS="${BUILD_X1} ${BUILD_X2}"
set -x -e

export PATH=/usr/local/go/bin:$PATH

if [ -z "$DESTDIR" ]; then
  DESTDIR=$(mktemp -d)
fi

function build_otel() {
  cd submodules/opentelemetry-java-contrib
  mkdir -p "$DESTDIR$subagentdir/opentelemetry-collector/"
  ./gradlew --no-daemon :jmx-metrics:build
  cp jmx-metrics/build/libs/opentelemetry-jmx-metrics-*-SNAPSHOT.jar "$DESTDIR$subagentdir/opentelemetry-collector/opentelemetry-java-contrib-jmx-metrics.jar"

  # Rename LICENSE file because it causes issues with file hash consistency due to an unknown
  # issue with the debuild/rpmbuild processes. Something is unzipping the jar in a case-insensitive
  # environment and having a conflict between the LICENSE file and license/ directory, leading to a changed jar file
  mkdir ./META-INF
  unzip -j "$DESTDIR$subagentdir/opentelemetry-collector/opentelemetry-java-contrib-jmx-metrics.jar" "META-INF/LICENSE" -d ./META-INF
  zip -d "$DESTDIR$subagentdir/opentelemetry-collector/opentelemetry-java-contrib-jmx-metrics.jar" "META-INF/LICENSE"
  mv ./META-INF/LICENSE ./META-INF/LICENSE.renamed
  zip -u "$DESTDIR$subagentdir/opentelemetry-collector/opentelemetry-java-contrib-jmx-metrics.jar" "META-INF/LICENSE.renamed"

  cd ../opentelemetry-operations-collector
  # Using array assignment to drop the filename from the sha256sum output
  JAR_SHA_256=($(sha256sum "$DESTDIR$subagentdir/opentelemetry-collector/opentelemetry-java-contrib-jmx-metrics.jar"))
  go build -o "$DESTDIR$subagentdir/opentelemetry-collector/otelopscol" \
    -ldflags "-X github.com/open-telemetry/opentelemetry-collector-contrib/receiver/jmxreceiver.MetricsGathererHash=$JAR_SHA_256" \
    ./cmd/otelopscol
}

function build_fluentbit() {
  cd submodules/fluent-bit
  mkdir -p build
  cd build
  # CMAKE_INSTALL_PREFIX here will cause the binary to be put at
  # /usr/lib/google-cloud-ops-agent/bin/fluent-bit
  # Additionally, -DFLB_SHARED_LIB=OFF skips building libfluent-bit.so
  cmake .. -DCMAKE_INSTALL_PREFIX=$subagentdir/fluent-bit \
    -DFLB_HTTP_SERVER=ON -DFLB_DEBUG=OFF -DCMAKE_BUILD_TYPE=RelWithDebInfo \
    -DWITHOUT_HEADERS=ON -DFLB_SHARED_LIB=OFF -DFLB_STREAM_PROCESSOR=OFF \
    -DFLB_MSGPACK_TO_JSON_INIT_BUFFER_SIZE=1.5 -DFLB_MSGPACK_TO_JSON_REALLOC_BUFFER_SIZE=.10
  make -j8
  make DESTDIR="$DESTDIR" install
  # We don't want fluent-bit's service or configuration, but there are no cmake
  # flags to disable them. Prune after build.
  rm "${DESTDIR}/lib/systemd/system/fluent-bit.service"
  rm -r "${DESTDIR}${subagentdir}/fluent-bit/etc"
}

function build_opsagent() {
  mkdir -p "$DESTDIR$prefix/libexec"
  go build -o "$DESTDIR$prefix/libexec/google_cloud_ops_agent_engine" \
    -ldflags "$LD_FLAGS" \
    github.com/GoogleCloudPlatform/ops-agent/cmd/google_cloud_ops_agent_engine
}

function build_systemd() {
  function install_unit() {
    # $1 = source path; $2 = destination path relative to the unit directory
    sed "s|@PREFIX@|$prefix|g; s|@SYSCONFDIR@|$sysconfdir|g" "$1" > "$DESTDIR$systemdsystemunitdir/$2"
  }
  mkdir -p "$DESTDIR$systemdsystemunitdir"
  for f in systemd/*.service; do
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
