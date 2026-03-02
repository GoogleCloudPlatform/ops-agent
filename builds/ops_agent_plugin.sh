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
source VERSION && echo $PKG_VERSION  > "$DESTDIR/OPS_AGENT_VERSION"
mkdir -p "$DESTDIR/opt/google-cloud-ops-agent"
go build -buildvcs=false -ldflags "-s -w" -o "$DESTDIR/opt/google-cloud-ops-agent/ops_agent" \
  github.com/GoogleCloudPlatform/ops-agent/cmd/ops_agent_uap_plugin