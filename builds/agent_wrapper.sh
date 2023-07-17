# Copyright 2023 Google LLC
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
mkdir -p "$DESTDIR/opt/google-cloud-ops-agent/libexec"
go build -buildvcs=false -o "$DESTDIR/opt/google-cloud-ops-agent/libexec/google_cloud_ops_agent_wrapper" \
  github.com/GoogleCloudPlatform/ops-agent/cmd/agent_wrapper