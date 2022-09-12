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

# Build .rpms
rpmbuild --define "_source_filedigest_algorithm md5" \
  --define "package_version $PKG_VERSION" \
  --define "_sourcedir $(pwd)" \
  --define "_rpmdir $(pwd)" \
  -ba pkg/rpm/google-cloud-ops-agent.spec
cp $(uname -m)/google-cloud-ops-agent*.rpm /
