#!/bin/bash
# Copyright 2022 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


set -ex

. VERSION

echo 'Creating plugin'
ls .
PLUGIN_DIR=$1/plugin_dir/var/lib/google-guest-agent/plugins/ops-agent-plugin_$PKG_VERSION
mkdir -p ${PLUGIN_DIR}
mkdir -p ${PLUGIN_DIR}/subagents/opentelemetry-collector
mkdir -p ${PLUGIN_DIR}/subagents/fluent-bit/bin/
mkdir -p ${PLUGIN_DIR}/libexec
mkdir -p ${PLUGIN_DIR}/THIRD_PARTY_LICENSES

cp $1/opt/google-cloud-ops-agent/plugin ${PLUGIN_DIR}/plugin
cp $1/opt/google-cloud-ops-agent/libexec/google_cloud_ops_agent_wrapper ${PLUGIN_DIR}/libexec/google_cloud_ops_agent_wrapper
cp $1/opt/google-cloud-ops-agent/libexec/google_cloud_ops_agent_diagnostics ${PLUGIN_DIR}/libexec/google_cloud_ops_agent_diagnostics

cp $1/opt/google-cloud-ops-agent/subagents/opentelemetry-collector/otelopscol ${PLUGIN_DIR}/subagents/opentelemetry-collector/otelopscol
cp $1/opt/google-cloud-ops-agent/subagents/fluent-bit/bin/fluent-bit ${PLUGIN_DIR}/subagents/fluent-bit/bin/fluent-bit

tar -cvzf /google-cloud-ops-agent-plugin_${PKG_VERSION}.tar.gz -C $1/plugin_dir/ .
echo 'DONE creating plugin'
