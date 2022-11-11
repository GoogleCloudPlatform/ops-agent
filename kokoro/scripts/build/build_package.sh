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

. VERSION
export_to_sponge_config "PACKAGE_VERSION" "${PKG_VERSION}"

# From https://cloud.google.com/compute/docs/troubleshooting/known-issues#keyexpired-2
# to fix issues like b/227486796.
curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -

docker images || echo images failed

# Install Docker.
# https://docs.docker.com/engine/install/ubuntu/#install-using-the-repository
curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
  | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt-get -y update
sudo apt-get -y install docker-ce docker-ce-cli containerd.io

sudo docker images

ARTIFACT_REGISTRY="us-docker.pkg.dev"
sudo gcloud auth configure-docker "${ARTIFACT_REGISTRY}"
CACHE_LOCATION="${ARTIFACT_REGISTRY}/stackdriver-test-143416/google-cloud-ops-agent-build-cache/ops-agent-cache:${DISTRO}"

sudo DOCKER_BUILDKIT=1 docker build . \
  --cache-from="${CACHE_LOCATION}" \
  --build-arg BUILDKIT_INLINE_CACHE=1 \
  --target "${DISTRO}-build" \
  -t build_image

# Tell our continuous build to update the cache. Our other builds do not
# write to any kind of cache, for example a per-PR cache, because the
# push takes a few minutes and adds little value over just using the continuous
# build's cache.
if [[ "${KOKORO_ROOT_JOB_TYPE}" == "CONTINUOUS_INTEGRATION" ]]; then
  sudo docker image tag build_image "${CACHE_LOCATION}"
  sudo docker push "${CACHE_LOCATION}"
fi

SIGNING_DIR="$(pwd)/kokoro/scripts/build/signing"
if [[ "${PKGFORMAT}" == "rpm" && "${SKIP_SIGNING}" != "true" ]]; then
  RPM_SIGNING_KEY="${KOKORO_KEYSTORE_DIR}/71565_rpm-signing-key"
  cp "${RPM_SIGNING_KEY}" "${SIGNING_DIR}/signing-key"
fi

sudo docker run \
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

exit 3