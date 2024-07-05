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


set -e
set -u
set -x
set -o pipefail

# cd to the root of the git repo containing this script ($0).
cd "$(readlink -f "$(dirname "$0")")"
cd ../../../

# Source the common utilities.
source kokoro/scripts/utils/louhi.sh

# For soak tests run by Louhi
populate_env_vars_from_louhi_tag_if_present
# if TARGET & ARCH are set, retrieve the soak distro from project.yaml
if [[ -n "${TARGET:-}" && -n "${ARCH:-}" ]]; then
  DISTRO=$(yaml project.yaml "['targets']['${TARGET}']['architectures']['${ARCH}']['soak_distro']")
  export VM_NAME="soak-test-${_LOUHI_EXECUTION_ID}-${TARGET}-${ARCH//_/-}-${LABEL}"
  export DISTRO
fi

for GIT_ALIAS in git github; do
  CANDIDATE_LOCATION="${KOKORO_ARTIFACTS_DIR}/${GIT_ALIAS}/unified_agents/integration_test/soak_test/cmd/launcher"
  if [[ -d "${CANDIDATE_LOCATION}" ]]; then
    cd "${CANDIDATE_LOCATION}"
    break
  fi
done

LOG_RATE=${LOG_RATE-1000} \
LOG_SIZE_IN_BYTES=${LOG_SIZE_IN_BYTES-1000} \
VM_NAME="${VM_NAME:-github-soak-test-${KOKORO_BUILD_ID}}" \
TTL="${TTL:-30m}" \
  go run -tags=integration_test .
