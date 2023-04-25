#!/bin/bash

USAGE="Usage: ./mirror_content.sh <URL>

This will download the content at <URL> and upload it to:

https://storage.googleapis.com/ops-agents-public-buckets-vendored-deps/mirrored-content/<URL without http:// or https://>

This script can only be run by Googlers because release tests read from the
same bucket as presubmit tests, and we want release tests to be secure."

if [[ "$#" -ne 1 ]]; then
  echo "$USAGE"
  exit 1
fi

set -euxo pipefail

URL="$1"

SCRATCH="$(mktemp --directory)"

(cd "${SCRATCH}" && wget --force-directories "${URL}")

gsutil cp -r "${SCRATCH}/*" "gs://ops-agents-public-buckets-vendored-deps/mirrored-content/"

rm -r "${SCRATCH}"

set +x

STRIPPED_URL="$(echo "${URL}" | sed --regexp-extended 's#^https?://##')"
echo "Content should be available at https://storage.googleapis.com/ops-agents-public-buckets-vendored-deps/mirrored-content/${STRIPPED_URL}"
