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

export PATH=/usr/local/go/bin:$PATH

if [ -z "$DESTDIR" ]; then
  DESTDIR=$(mktemp -d)
fi

function build_otel() {
  cd submodules/opentelemetry-java-contrib
  mkdir -p "$DESTDIR$subagentdir/"
  ./gradlew --no-daemon :jmx-metrics:build
  cp jmx-metrics/build/libs/opentelemetry-jmx-metrics-*-SNAPSHOT.jar "$DESTDIR$subagentdir/opentelemetry-java-contrib-jmx-metrics.jar"

  # Rename LICENSE file because it causes issues with file hash consistency due to an unknown
  # issue with the debuild/rpmbuild processes. Something is unzipping the jar in a case-insensitive
  # environment and having a conflict between the LICENSE file and license/ directory, leading to a changed jar file
  mkdir ./META-INF
  unzip -j "$DESTDIR$subagentdir/opentelemetry-java-contrib-jmx-metrics.jar" "META-INF/LICENSE" -d ./META-INF
  zip -d "$DESTDIR$subagentdir/opentelemetry-java-contrib-jmx-metrics.jar" "META-INF/LICENSE"
  mv ./META-INF/LICENSE ./META-INF/LICENSE.renamed
  zip -u "$DESTDIR$subagentdir/opentelemetry-java-contrib-jmx-metrics.jar" "META-INF/LICENSE.renamed"

  cd ../opentelemetry-operations-collector
  # Using array assignment to drop the filename from the sha256sum output
  JAR_SHA_256=($(sha256sum "$DESTDIR$subagentdir/opentelemetry-java-contrib-jmx-metrics.jar"))
  GOOS=windows go build -o $DESTDIR$subagentdir/google-cloud-metrics-agent_windows_amd64.exe \
    -ldflags "-X github.com/open-telemetry/opentelemetry-collector-contrib/receiver/jmxreceiver.MetricsGathererHash=$JAR_SHA_256" \
    ./cmd/otelopscol
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

  export OPENSSL_ROOT_DIR=/usr/share/mingw-w64 OPENSSL_CRYPTO_LIBRARY=/usr/lib/mingw-w64/libcrypto.a

  cmake .. -DCMAKE_TOOLCHAIN_FILE=../../pkg/goo/ubuntu-mingw64.cmake -DCMAKE_INSTALL_PREFIX=$prefix/ \
    -DFLB_HTTP_SERVER=On -DCMAKE_BUILD_TYPE=RelWithDebInfo
  #cat build/CMakeFiles/CMakeOutput.log
  #cat >&2 build/CMakeFiles/CMakeError.log
  make #-j8
  make DESTDIR="$DESTDIR" install
  # We don't want fluent-bit's service or configuration, but there are no cmake
  # flags to disable them. Prune after build.
  rm "${DESTDIR}/lib/systemd/system/fluent-bit.service"
}

function build_opsagent() {
  GOOS=windows go build -o "$DESTDIR$prefix/bin/google-cloud-ops-agent.exe" \
    -ldflags "$LD_FLAGS" \
    ./cmd/ops_agent_windows
}

(build_fluentbit)
(build_otel)
(build_opsagent)

# TODO: Build sample config file
mkdir -p "$DESTDIR$sysconfdir/"
cp "confgenerator/default-config.yaml" "$DESTDIR$sysconfdir/config.yaml"
mkdir -p "$DESTDIR/pkg/goo/"
cp "pkg/goo/maint.ps1" "$DESTDIR/pkg/goo/"

# N.B. Don't include $DESTDIR itself in the tarball, since mktemp -d will create it mode 0700.
(cd "$DESTDIR" && tar -czf /tmp/google-cloud-ops-agent.tgz *)

set -ex

TOP_SRCDIR=$(pwd)
(cd "$DESTDIR" && \
 $(go env GOPATH)/bin/goopack -output_dir / \
   -var:PKG_VERSION=$PKG_VERSION \
   -var:ARCH=x86_64 \
   -var:GOOS=windows \
   -var:GOARCH=amd64 \
   "${TOP_SRCDIR}/pkg/goo/google-cloud-ops-agent.goospec")

