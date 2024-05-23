#!/bin/bash
# Copyright 2024 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -x -e
DESTDIR=$1
mkdir -p $DESTDIR
fluent_bit_dir=/opt/google-cloud-ops-agent/subagents/fluent-bit

CMAKE_ARGS=`cat $(dirname $0)/fluent_bit_cmake_args`

cd submodules/fluent-bit
mkdir -p build
cd build
# CMAKE_INSTALL_PREFIX here will cause the binary to be put at
# /usr/lib/google-cloud-ops-agent/bin/fluent-bit
# Additionally, -DFLB_SHARED_LIB=OFF skips building libfluent-bit.so
cmake .. -DCMAKE_INSTALL_PREFIX=$fluent_bit_dir $CMAKE_ARGS
make -j8
make DESTDIR="$DESTDIR" install

# We don't want fluent-bit's service or configuration, but there are no cmake
# flags to disable them. Prune after build.
rm "${DESTDIR}/lib/systemd/system/fluent-bit.service" || true
rm "${DESTDIR}/usr/lib/systemd/system/fluent-bit.service" || true
rm -r "${DESTDIR}${fluent_bit_dir}/etc"
