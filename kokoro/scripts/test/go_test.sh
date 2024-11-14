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


# This script needs the following environment variables to be defined:
# 1. various KOKORO_* variables
# 2. TEST_SUITE_NAME: name of the test file, minus the .go suffix. For example,
#    ops_agent_test or third_party_apps_test.
#
# And also the following, documented at the top of gce_testing.go and
# $TEST_SUITE_NAME.go:
# 1. PROJECT
# 2. ZONES
# 3. TRANSFERS_BUCKET
#
# If TEST_SOURCE_PIPER_LOCATION is defined, this script will look for test
# sources in there, otherwise it will look in GitHub.
#
# In addition, the following test suites need additional env variables:
# install_scripts_test:
#   * AGENTS_TO_TEST: comma-separated list of agents to test.
#   * SCRIPTS_DIR: path to installation scripts to test.
# os_config_test and gcloud_policies_test:
#   * GCLOUD_LITE_BLAZE_PATH: path to just-built copy of gcloud_lite to use for
#     testing.
# ops_agent_policies_test:
#   * POLICIES_DIR: path to policy .yaml files to test.
#   * ZONE: a single zone to run tests in (ZONES is ignored).

set -e
set -u
set -x
set -o pipefail

# cd to the root of the git repo containing this script ($0).
cd "$(readlink -f "$(dirname "$0")")"
cd ../../../

# Source the common utilities.
source kokoro/scripts/utils/common.sh
source kokoro/scripts/utils/louhi.sh

track_flakiness

# Avoids "fatal: detected dubious ownership in repository" errors on Kokoro containers.
git config --global --add safe.directory "$(pwd)"

# A helper function for joining a bash array.
# Ex. join_by , a b c -> a,b,c
function join_by() {
  delim="$1"
  for (( i = 2; i <= $#; i++)); do
    printf "${!i}"  # The ith positional argument
    if [[ $i -ne $# ]]; then
      printf "${delim}"
    fi
  done
}

function set_image_specs() {
  # if IMAGE_SPECS is defined, do nothing
  if [[ -n "${IMAGE_SPECS:-}" ]]; then
    return 0
  fi
  populate_env_vars_from_louhi_tag_if_present
  # if TARGET is not set, return an error
  if [[ -z "${TARGET:-}" ]]; then
    echo "At least one of TARGET/IMAGE_SPECS must be set." 1>&2
    return 1
  fi
  # if ARCH is not set, return an error
  if [[ -z "${ARCH:-}" ]]; then
    echo "If TARGET is set, ARCH must be as well." 1>&2
    return 1
  fi
  # At minimum, IMAGE_SPECS will be the images from "representative" for TARGET/ARCH in projects.yaml.
  local image_specs
  image_specs=$(yaml project.yaml "['targets']['${TARGET}']['architectures']['${ARCH}']['test_distros']['representative']")
  # If not a presubmit job, add the exhaustive list of test distros.
  if [[ "${TEST_EXHAUSTIVE_DISTROS:-}" == "1" ]]; then
    # ['test_distros']['exhaustive'] is an optional field.
    exhaustive_image_specs=$(yaml project.yaml "['targets']['${TARGET}']['architectures']['${ARCH}']['test_distros']['exhaustive']") || true
    if [[ -n "${exhaustive_image_specs:-}" ]]; then
      image_specs="${image_specs},${exhaustive_image_specs}"
    fi
  fi
  IMAGE_SPECS="${image_specs}"
  export IMAGE_SPECS
}

export_to_sponge_config "TARGET" "${TARGET:-}"
export_to_sponge_config "ARCH" "${ARCH:-}"

# Note: if we ever need to change regions, we will need to set up a new
# Cloud Router and Cloud NAT gateway for that region. This is because
# we use --no-address on Kokoro, because of b/169084857.
# The new Cloud NAT gateway must have "Minimum ports per VM instance"
# set to 512 as per this article:
# https://cloud.google.com/knowledge/kb/sles-unable-to-fetch-updates-when-behind-cloud-nat-000004450
function set_zones() {
   # if ZONES is defined, do nothing
  if [[ -n "${ZONES:-}" ]]; then
    return 0
  fi
  if [[ "${ARCH:-}" == "x86_64" ]]; then
    zone_list=(
      us-central1-a=3
      us-central1-b=3
      us-central1-c=3
      us-central1-f=3
      us-east1-b=2
      us-east1-c=2
      us-east1-d=2
    )
  # T2A machines are only available on us-central1-{a,b,f}.
  # See warning above about changing regions.
  elif [[ "${ARCH:-}" == "aarch64" ]]; then
    zone_list=(
      us-central1-a
      us-central1-b
      us-central1-f
    )
  else
    zone_list=(
      invalid_zone
    )
  fi
  zones=$(join_by , "${zone_list[@]}")
  export ZONES=$zones
}

# Temporary compatibility shim for old PLATFORMS variable.
if [[ -n "${PLATFORMS:-}" ]]; then
  IMAGE_SPECS="${PLATFORMS}"
fi

set_image_specs
set_zones

# If a built agent was passed in from Kokoro directly, use that.
if compgen -G "${KOKORO_GFILE_DIR}/result/google-cloud-ops-agent*" > /dev/null; then
  # Upload the agent packages to GCS.
  AGENT_PACKAGES_IN_GCS="gs://${TRANSFERS_BUCKET}/agent_packages/${KOKORO_BUILD_ID}"
  gsutil cp -r "${KOKORO_GFILE_DIR}/result/*" "${AGENT_PACKAGES_IN_GCS}/"

  # AGENT_PACKAGES_IN_GCS is used to tell Ops Agent integration tests
  # (https://github.com/GoogleCloudPlatform/ops-agent/tree/master/integration_test)
  # to install and use this custom build of the agent instead.
  export AGENT_PACKAGES_IN_GCS
fi

LOGS_DIR="${KOKORO_ARTIFACTS_DIR}/logs"
mkdir -p "${LOGS_DIR}"

if [[ -n "${TEST_SOURCE_PIPER_LOCATION-}" ]]; then
  if [[ -n "${SCRIPTS_DIR-}" ]]; then
    SCRIPTS_DIR="${KOKORO_PIPER_DIR}/${SCRIPTS_DIR}"
    export SCRIPTS_DIR
  fi
  if [[ -n "${POLICIES_DIR-}" ]]; then
    POLICIES_DIR="${KOKORO_PIPER_DIR}/${POLICIES_DIR}"
    export POLICIES_DIR
  fi

  cd "${KOKORO_PIPER_DIR}/${TEST_SOURCE_PIPER_LOCATION}/${TEST_SUITE_NAME}"

  # Make a module containing the latest dependencies from GitHub.
  go mod init "${TEST_SUITE_NAME}"
  go get "github.com/GoogleCloudPlatform/ops-agent@${BRANCH_TO_TEST_FROM_PIPER:-master}"
  go mod tidy -compat=1.17
else
  cd "integration_test/${TEST_SUITE_NAME}"
fi

if [[ "${TEST_SUITE_NAME}" == "os_config_test" || "${TEST_SUITE_NAME}" == "gcloud_policies_test" ]]; then
  GCLOUD_TO_TEST="${KOKORO_BLAZE_DIR}/${GCLOUD_LITE_BLAZE_PATH}"
  export GCLOUD_TO_TEST
fi

# Boost the max number of open files from 1024 to 1 million.
ulimit -n 1000000

# Set up some command line flags for "gotestsum".
gotestsum_args=(
  --packages=./...
  --format=standard-verbose
  --junitfile="${LOGS_DIR}/sponge_log.xml"
)
if [[ -n "${GOTESTSUM_RERUN_FAILS:-}" ]]; then
  gotestsum_args+=( "--rerun-fails=${GOTESTSUM_RERUN_FAILS}" )
fi

# Set up some command line flags for "go test".
go_test_args=(
  -test.parallel=1000
  -tags=integration_test
  -timeout=3h
)
if [[ "${SHORT:-false}" == "true" ]]; then
  go_test_args+=( "-test.short" )
fi
if [[ -n "${TEST_SELECTOR:-}" ]]; then
  go_test_args+=( "-test.run=${TEST_SELECTOR}" )
fi

TEST_UNDECLARED_OUTPUTS_DIR="${LOGS_DIR}" \
  gotestsum "${gotestsum_args[@]}" \
  -- "${go_test_args[@]}"
