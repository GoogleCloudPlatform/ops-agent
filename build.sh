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
# dummy change
set -x -e
prefix=/opt/google-cloud-ops-agent

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

function build_opsagentengine() {
  if [[ ! -f /work/google_cloud_ops_agent_engine ]]; then
  go build -buildvcs=false -o "/work/google_cloud_ops_agent_engine" \
    -ldflags "$LD_FLAGS" \
    github.com/GoogleCloudPlatform/ops-agent/cmd/google_cloud_ops_agent_engine
  fi
  mkdir -p "$DESTDIR$prefix/libexec"
  cp /work/google_cloud_ops_agent_engine "$DESTDIR$prefix/libexec/google_cloud_ops_agent_engine"
}

(build_opsagentengine)

# Copy the cached compilations from docker to the destination
cp -r /work/cache/* $DESTDIR

# N.B. Don't include $DESTDIR itself in the tarball, since mktemp -d will create it mode 0700.
(cd "$DESTDIR" && tar -czf /tmp/google-cloud-ops-agent.tgz *)
