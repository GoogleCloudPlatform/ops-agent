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

# Install Docker.
# https://docs.docker.com/engine/install/ubuntu/#install-using-the-repository
curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
  | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt-get -y update
sudo apt-get -y install docker-ce docker-ce-cli containerd.io

# Set up a BRANCH_NAME to use to tag the docker image with for the cache.
# On Kokoro, `git rev-parse --abbrev-ref HEAD` just prints "HEAD", so we
# need to use the pull request number instead of the actual branch name.
if [[ -n "${KOKORO_GITHUB_PULL_REQUEST_NUMBER}" ]]; then
  BRANCH_NAME="pr-${KOKORO_GITHUB_PULL_REQUEST_NUMBER}"
else
  BRANCH_NAME="branch-$(git rev-parse --abbrev-ref HEAD)"
fi

ARTIFACT_REGISTRY_LOCATION="us-docker.pkg.dev"
sudo gcloud auth configure-docker "${ARTIFACT_REGISTRY_LOCATION}"

CACHE_LOCATION="${ARTIFACT_REGISTRY_LOCATION}/stackdriver-test-143416/google-cloud-ops-agent-build-cache/ops-agent-cache"
CACHE_LOCATION_MASTER="${CACHE_LOCATION}:${DISTRO}-master"
CACHE_LOCATION_BRANCH="${CACHE_LOCATION}:${DISTRO}-${BRANCH_NAME}"

# Let's see if this is necessary
# TODO: if unnecessary, remember to inline CACHE_LOCATION_MASTER and CACHE_LOCATION_BRANCH
sudo docker pull "${CACHE_LOCATION_BRANCH}" || \
  sudo docker pull "${CACHE_LOCATION_MASTER}" || \
  true

# Create a driver so that we can use the --cache-{from,to} flags below.
# https://docs.docker.com/build/building/drivers/
sudo docker buildx create \
  --name container-driver \
  --driver=docker-container

# The --cache-from and --cache-to arguments are following the recommendations
# at https://docs.docker.com/build/building/cache/backends/#command-syntax.
# --load is necessary because of:
# https://docs.docker.com/build/building/drivers/docker-container/#loading-to-local-image-store
sudo DOCKER_BUILDKIT=1 docker buildx build . \
  --builder=container-driver \
  --cache-from="${CACHE_LOCATION_MASTER}" \
  --cache-from="${CACHE_LOCATION_BRANCH}" \
  --cache-to="type=registry,ref=${CACHE_LOCATION_BRANCH},mode=max" \
  --load \
  --target "${DISTRO}-build" \
  -t build_image

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
