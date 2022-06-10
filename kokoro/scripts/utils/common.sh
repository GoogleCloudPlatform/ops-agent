#!/bin/bash

set -e
set -x
set -o pipefail

CUSTOM_SPONGE_CONFIG="${KOKORO_ARTIFACTS_DIR}/custom_sponge_config.csv"

# A convenience function to store a key/value pair in the Sponge invocation
# configuration. To propagate the key/value pair from Sponge to a Rapid custom
# field, also add the key to sponge_config_keys in the workflow spec.
function export_to_sponge_config() {
  local key="${1}"
  local value="${2}"
  printf "${key},${value}\n" >> "${CUSTOM_SPONGE_CONFIG}"
}

# A convenience function to extract the HEAD commit hash of a given repo.
function extract_git_hash() {
  local repo="${1}"
  git -C "$(basename "${repo}")" rev-parse HEAD
}

# A convenience function to store the HEAD commit hash in the Sponge invocation
# configuration. To propagate the hash from Sponge to a Rapid custom field, also
# add the repo_var to sponge_config_keys in the workflow spec.
function git_track_hash() {
  local repo="${1}"
  local repo_var="${2}"
  hash="$(extract_git_hash "${repo}")"
  export_to_sponge_config "${repo_var}" "${hash}"
}

# Track Flakiness for Non-Google3 projects by exporting the vars described
# in go/ng3-flakiness to Sponge.
function track_flakiness()
{
  # Inherit job type from root job in job chains/groups.
  local job_type="${KOKORO_ROOT_JOB_TYPE:-$KOKORO_JOB_TYPE}"

  if [[ "${job_type}" == "PRESUBMIT_GITHUB" ]]; then
    # Exit early, we don't track flakiness for presubmits.
    return
  elif [[ "${job_type}" == "CONTINUOUS_INTEGRATION" ]]; then
    export_to_sponge_config "ng3_job_type" "POSTSUBMIT"
  elif [[ "${job_type}" == "RELEASE" ]]; then
    export_to_sponge_config "ng3_job_type" "PERIODIC"
  fi
  export_to_sponge_config "ng3_project_id" "cloud-ops-agent"
  export_to_sponge_config "ng3_commit" "${KOKORO_GIT_COMMIT_unified_agents}"
  export_to_sponge_config "ng3_cl_target_branch" "master"
  export_to_sponge_config "ng3_test_type" "INTEGRATION"
  export_to_sponge_config "ng3_sponge_url" "https://fusion2.corp.google.com/invocations/${KOKORO_BUILD_ID}"
}
