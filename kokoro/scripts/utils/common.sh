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
  export_to_sponge_config "ng3_commit" "${KOKORO_GIT_COMMIT_unified_agents-${KOKORO_GIT_COMMIT}}"
  export_to_sponge_config "ng3_cl_target_branch" "master"
  export_to_sponge_config "ng3_test_type" "INTEGRATION"
  export_to_sponge_config "ng3_sponge_url" "https://fusion2.corp.google.com/invocations/${KOKORO_BUILD_ID}"
}

# A helper for parsing YAML files.
# Ex: VALUE=$(yaml ~/my_yaml_file.yaml "['a_key']")
function yaml() {
  python3 -c "import yaml;f=yaml.safe_load(open('$1'))$2;print(','.join(str(i) for i in f) if type(f)==list else f);"
}

# This function expects to be run at the root of the git repo.
function set_platforms() {
  # if PLATFORMS is defined, do nothing
  if [ -n "${PLATFORMS}" ]; then
    return 0
  fi
  # if _LOUHI_TAG_NAME is defined, set TARGET and ARCH env vars by parsing it.
  # Example value: louhi/2.46.0/foobar/windows/x86_64/start
  if [ -n "${_LOUHI_TAG_NAME}" ]; then
    local split_tag=(${IN//;/ })
    TARGET=${arrIN[3]}
    ARCH=${arrIN[4]}
  fi
  # if TARGET is not set, return an error
  if [ -z "${TARGET}" ]; then
    echo "At least one of TARGET/PLATFORMS must be set." 1>&2
    return 1
  fi
  # if ARCH is not set, return an error
  if [ -z "${ARCH}" ]; then
    echo "If TARGET is set, ARCH must be as well." 1>&2
    return 1
  fi
  # At minimum, PLATFORMS will be the distros from "representative" for TARGET/ARCH in projects.yaml.
  local platforms=$(yaml project.yaml "['targets']['${TARGET}']['architectures']['${ARCH}']['representative']")
  # If not a presubmit job, add the exhaustive list of test distros.
  if ["${KOKORO_ROOT_JOB_TYPE:-$KOKORO_JOB_TYPE}" != PRESUBMIT_*]; then
    platforms="${platforms},$(yaml project.yaml "['targets']['${TARGET}']['architectures']['${ARCH}']['exhaustive'])" | true
  fi
  PLATFORMS=${platforms}
}