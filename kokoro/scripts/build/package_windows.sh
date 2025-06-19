#!/bin/bash

go install -trimpath -ldflags="-s -w" github.com/google/googet/v2/goopack@latest

# cd to the root of the git repo containing this script.
cd "$(readlink -f "$(dirname "$0")")"
cd "$(git rev-parse --show-toplevel)"

# Avoids "fatal: detected dubious ownership in repository" errors on Kokoro containers.
git config --global --add safe.directory "$(pwd)"

mkdir "${KOKORO_ARTIFACTS_DIR}/result"

mv "${KOKORO_GFILE_DIR}/result" "${KOKORO_ARTIFACTS_DIR}/result"

releaseName=$(awk -F "=" '/PKG_VERSION/ {print $2}' VERSION | tr -d '"')

"$GOPATH"/bin/goopack -output_dir "${KOKORO_ARTIFACTS_DIR}/result" \
  -var:PKG_VERSION="$releaseName" \
  -var:ARCH=x86_64 \
  -var:GOOS=windows \
  -var:GOARCH=amd64 \
  -var:FROM_DIR="${KOKORO_ARTIFACTS_DIR}/result/out" \
  pkg/goo/google-cloud-ops-agent.goospec

if [[ -n $_LOUHI_TAG_NAME ]]
then
  # Example value: louhi/2.46.0/abcdef/windows/x86_64/start
  IFS="/"
  read -ra louhi_tag_components <<< "$_LOUHI_TAG_NAME"
  ver="${louhi_tag_components[1]}"
  ref="${louhi_tag_components[2]}"
  target="${louhi_tag_components[3]}"
  arch="${louhi_tag_components[4]}"
  gcs_bucket="gs://${_STAGING_ARTIFACTS_PROJECT_ID}-ops-agent-releases/${ver}/${ref}/${target}/${arch}/"
  gsutil cp "${KOKORO_ARTIFACTS_DIR}"/result/*.goo  "${gcs_bucket}"
fi
