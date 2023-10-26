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

ARCH="$(docker info --format '{{.Architecture}}')"
ARTIFACT_REGISTRY="us-docker.pkg.dev"
docker-credential-gcr configure-docker --registries="${ARTIFACT_REGISTRY}"
CACHE_LOCATION="${ARTIFACT_REGISTRY}/stackdriver-test-143416/google-cloud-ops-agent-build-cache/ops-agent-cache:${DISTRO}_${ARCH}"

DOCKER_BUILDKIT=1 docker build . \
  --cache-from="${CACHE_LOCATION}" \
  --build-arg BUILDKIT_INLINE_CACHE=1 \
  --progress=plain \
  --target "${DISTRO}-build" \
  -t build_image

docker history --no-trunc build_image

# Tell our continuous build to update the cache. Our other builds do not
# write to any kind of cache, for example a per-PR cache, because the
# push takes a few minutes and adds little value over just using the continuous
# build's cache.
if [[ "${KOKORO_ROOT_JOB_TYPE}" == "CONTINUOUS_INTEGRATION" ]]; then
  docker image tag build_image "${CACHE_LOCATION}"
  docker push "${CACHE_LOCATION}"
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
  -v "${TMPDIR}":/transformation_test \
  build_image \
  bash <<EOF
    cp /google-cloud-ops-agent*.${PKGFORMAT} /artifacts

    if [[ "${PKGFORMAT}" == "rpm" && "${SKIP_SIGNING}" != "true" ]]; then
      bash /signing/sign.sh /artifacts/*.rpm
    fi
EOF

TMPDIR="$(mktemp --directory)"

docker run \
  -i \
  -v "${TMPDIR}":/transformation_test \
  build_image \
  bash <<EOF
    cp /work/cache/opt/google-cloud-ops-agent/subagents/fluent-bit/bin/fluent-bit /transformation_test
EOF

# install go
GO_VERSION="1.19"
gsutil cp "gs://stackdriver-test-143416-go-install/go${GO_VERSION}.linux-amd64.tar.gz" - | \
  sudo tar --directory /usr/local -xzf /dev/stdin

PATH=$PATH:/usr/local/go/bin

go install -v github.com/jstemmer/go-junit-report/v2@latest

args=(
  -test.parallel=1000
  -timeout=3h
  -flb="${TMPDIR}"
)

STDERR_STDOUT_FILE="${TMPDIR}/test_stderr_stdout.txt"
function produce_xml() {
  cat "${STDERR_STDOUT_FILE}" | "$(go env GOPATH)/bin/go-junit-report" > "${TMPDIR}/sponge_log.xml"
}
# Always run produce_xml on exit, whether the test passes or fails.
trap produce_xml EXIT

# run transformation tests
go test -v ./transformation_test \
"${args[@]}" \
  2>&1 \
  | tee "${STDERR_STDOUT_FILE}"
