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
prefix=
sysconfdir=/config
subagentdir=$prefix/bin

. VERSION
if [ -z "$BUILD_DISTRO" ]; then
  BUILD_DISTRO=windows-ltsc2019
fi
if [ -z "$CODE_VERSION" ]; then
  CODE_VERSION=${PKG_VERSION}
fi
BUILD_INFO_IMPORT_PATH="github.com/GoogleCloudPlatform/ops-agent/internal/version"
BUILD_X1="-X ${BUILD_INFO_IMPORT_PATH}.BuildDistro=${BUILD_DISTRO}"
BUILD_X2="-X ${BUILD_INFO_IMPORT_PATH}.Version=${CODE_VERSION}"
LD_FLAGS="${BUILD_X1} ${BUILD_X2}"
set -x -e

if [ -z "$DESTDIR" ]; then
  DESTDIR=$(mktemp -d)
fi

function build_otel() {
  cd submodules/opentelemetry-operations-collector
  GOOS=windows go build -o $DESTDIR$subagentdir/google-cloud-metrics-agent_windows_amd64.exe ./cmd/otelopscol
}

function build_fluentbit() {
  cd submodules/fluent-bit
  mkdir -p build
  cd build

  HOST=x86_64-w64-mingw32
  CC=${HOST}-gcc

  TOP_SRCDIR=$(pwd)
  mkdir -p _build_aux

  # Build libtool
  LIBTOOL_DIR="${TOP_SRCDIR}/_build_aux/_libtool"
  pushd _build_aux
  if [ -d "_libtool" ]; then
    echo "Assuming that libtool is already built, because _libtool exists."
  else
    curl -LO http://ftpmirror.gnu.org/libtool/libtool-2.4.6.tar.gz
    tar xzf libtool-2.4.6.tar.gz
    cd libtool-2.4.6
    ./configure --host="$HOST" --prefix="${LIBTOOL_DIR}"
    make
    make install
  fi
  export PATH="${LIBTOOL_DIR}/bin:${PATH}"
  export LDFLAGS="-L${LIBTOOL_DIR}/bin -L${LIBTOOL_DIR}/lib ${LDFLAGS}"
  popd

  cmake .. -DCMAKE_TOOLCHAIN_FILE=../../pkg/goo/ubuntu-mingw64.cmake -DCMAKE_INSTALL_PREFIX=$subagentdir/fluent-bit -DFLB_HTTP_SERVER=On -DCMAKE_BUILD_TYPE=RELWITHDEBINFO
  make #-j8
  make DESTDIR="$DESTDIR" install
  # We don't want fluent-bit's service
  rm "$DESTDIR/lib/systemd/system/fluent-bit.service"
  rm -r "$DESTDIR$subagentdir/fluent-bit/etc"
}

function build_opsagent() {
  GOOS=windows go build -o "$DESTDIR$prefix/bin/google_cloud_ops_agent" -ldflags "$LD_FLAGS" github.com/GoogleCloudPlatform/ops-agent/cmd/ops_agent_windows
}

(build_otel)
(build_fluentbit)
(build_opsagent)
# TODO: Build sample config file
mkdir -p "$DESTDIR$sysconfdir/google-cloud-ops-agent/"
cp "confgenerator/default-config.yaml" "$DESTDIR/$sysconfdir/google-cloud-ops-agent/config.yaml"

# N.B. Don't include $DESTDIR itself in the tarball, since mktemp -d will create it mode 0700.
(cd "$DESTDIR" && tar -czf /tmp/google-cloud-ops-agent.tgz *)

set -ex

cd pkg/goo

$GOPATH/bin/goopack -output_dir / \
  -var:PKG_VERSION=$PKG_VERSION \
  -var:ARCH=x86_64 \
  -var:GOOS=windows \
  -var:GOARCH=amd64 \
  google-cloud-ops-agent.goospec

