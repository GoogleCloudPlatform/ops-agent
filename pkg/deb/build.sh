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

# Add changelog entry
dch --create -b --package google-cloud-ops-agent -M \
  --distribution $(lsb_release -cs) \
  -v $PKG_VERSION~$(lsb_release -is | tr '[:upper:]' '[:lower:]')$(lsb_release -rs) \
  "Automated build"

# Build .debs
debuild --preserve-envvar JAVA_HOME -us -uc -sa
