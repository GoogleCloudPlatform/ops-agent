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


# cd to the root of the git repo containing this script.
cd "$(readlink -f "$(dirname "$0")")"
cd "$(git rev-parse --show-toplevel)"

# Source the common scripts.
source "kokoro/scripts/utils/common.sh"

set -e
set -x
set -o pipefail

RESULT_DIR=${RESULT_DIR:-"${KOKORO_ARTIFACTS_DIR}/result"}
export RESULT_DIR

git_track_hash . "OPS_AGENT_REPO_HASH"
OPS_AGENT_REPO_HASH="$(extract_git_hash .)"
# Submodules aren't cloned by kokoro for github repos.
git submodule update --init --recursive

# Debugging why we are not getting Docker cache hits for presubmits.
ls -Al submodules/opentelemetry-operations-collector/go.* || echo ls failed
stat submodules/opentelemetry-operations-collector/go.* || echo stat failed

. VERSION
export_to_sponge_config "PACKAGE_VERSION" "${PKG_VERSION}"

ARTIFACT_REGISTRY="us-docker.pkg.dev"
sudo docker-credential-gcr configure-docker --registries="${ARTIFACT_REGISTRY}"
function get_cache_location() {
  echo "${ARTIFACT_REGISTRY}/stackdriver-test-143416/google-cloud-ops-agent-build-cache/ops-agent-cache$1:${DISTRO}"
}
function get_target() {
  echo "${DISTRO}-build$1"
}

DOCKER_TARGET_SUFFIXES=("" "-diagnostics" "-fluent-bit" "-otel")
DOCKER_CACHE_FROM=""
for suffix in "${DOCKER_TARGET_SUFFIXES[@]}"
do
  DOCKER_CACHE_FROM+="$(get_cache_location $suffix), "
done


function docker_build(){
  # ,$get_cache_location(-diagnostics),$get_cache_location(-fluent-bit),$get_cache_location(-otel)
  DOCKER_BUILDKIT=1 docker build . \
    --cache-from="$(get_cache_location $1),$(get_cache_location -diagnostics),$(get_cache_location -fluent-bit),$(get_cache_location -otel)" \
    --build-arg BUILDKIT_INLINE_CACHE=1 \
    --progress=plain \
    --target "$(get_target $1)" \
    -t $2
}

for suffix in "${DOCKER_TARGET_SUFFIXES[@]}"
do
  docker pull "$(get_cache_location $suffix)"
done
(docker_build "" "build_image")


docker history --no-trunc build_image

# Tell our continuous build to update the cache. Our other builds do not
# write to any kind of cache, for example a per-PR cache, because the
# push takes a few minutes and adds little value over just using the continuous
# build's cache.
function update_cache() {
  docker_build "$1" "$(get_cache_location $1)"
  docker push "$(get_cache_location $1)"
}

time (update_cache "-diagnostics")
time (update_cache "-fluent-bit")
time (update_cache  "-otel")

if [[ "${KOKORO_ROOT_JOB_TYPE}" == "CONTINUOUS_INTEGRATION" ]]; then
  for suffix in "${DOCKER_TARGET_SUFFIXES[@]}"
  do
    (update_cache "$suffix")
  done
fi

SIGNING_DIR="$(pwd)/kokoro/scripts/build/signing"
if [[ "${PKGFORMAT}" == "rpm" && "${SKIP_SIGNING}" != "true" ]]; then
  RPM_SIGNING_KEY="${KOKORO_KEYSTORE_DIR}/71565_rpm-signing-key"
  cp "${RPM_SIGNING_KEY}" "${SIGNING_DIR}/signing-key"
fi

docker run \
  -i \
  -v "${RESULT_DIR}":/artifacts \
  -v "${SIGNING_DIR}":/signing \
  build_image \
  bash <<EOF
    cp /google-cloud-ops-agent*.${PKGFORMAT} /artifacts

    if [[ "${PKGFORMAT}" == "rpm" && "${SKIP_SIGNING}" != "true" ]]; then
      bash /signing/sign.sh /artifacts/*.rpm
    fi
EOF
