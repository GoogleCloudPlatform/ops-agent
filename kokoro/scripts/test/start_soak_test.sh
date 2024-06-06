#!/bin/bash

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
  export VM_NAME="soak-test-${RELEASE_ID}-${TARGET}-${ARCH//_/-}-${LABEL}"
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
