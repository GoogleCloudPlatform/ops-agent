#!/bin/bash

set -e
set -u
set -x
set -o pipefail

for GIT_ALIAS in git github; do
  CANDIDATE_LOCATION="${KOKORO_ARTIFACTS_DIR}/${GIT_ALIAS}/unified_agents/integration_test/soak_test/cmd"
  if [[ -d "${CANDIDATE_LOCATION}" ]]; then
    cd "${CANDIDATE_LOCATION}"
    break
  fi
done

LOG_RATE=${LOG_RATE-100000} \
LOG_SIZE_IN_BYTES=${LOG_SIZE_IN_BYTES-1000} \
VM_NAME="${VM_NAME:-soak-test-${KOKORO_BUILD_NUMBER}-$(date +%F)}" \
DISTRO="${DISTRO:-ubuntu-2004-lts}" \
TTL="${TTL:-30m}" \
  go run . \
  -tags=integration_test \
  -timeout=1h

