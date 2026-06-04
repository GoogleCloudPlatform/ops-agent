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

otel_dir=/opt/google-cloud-ops-agent/subagents/opentelemetry-collector
DESTDIR="${1}${otel_dir}"

mkdir -p $DESTDIR

LDFLAGS=""



if [ -z "${GO_BIN}" ]; then
    GO_BIN=/usr/local/go/bin/go
fi
ARCH=$($GO_BIN env GOARCH)

cd submodules/opentelemetry-operations-collector/otelopscol

BUILDARCH="$ARCH" \
TARGETARCH="$ARCH" \
GO_BIN="${GO_BIN}" \
COLLECTOR_LD_FLAGS="$LDFLAGS" \
COLLECTOR_BUILDVCS="false" \
COLLECTOR_BUILD_TAGS="gpu" \
    make build
cp ./otelopscol "$DESTDIR/otelopscol"
