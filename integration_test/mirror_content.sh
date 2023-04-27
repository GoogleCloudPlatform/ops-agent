#!/bin/bash

USAGE="Usage: ./mirror_content.sh [URL]...

This will download the content at the given URLs and upload it to:

https://storage.googleapis.com/ops-agents-public-buckets-vendored-deps/mirrored-content/<URL without http:// or https://>

This script can only be run by Googlers because release tests read from the
same bucket as presubmit tests, and we want release tests to be secure."

if [[ "$#" -eq 0 ]]; then
  echo "$USAGE"
  exit 1
fi

set -euxo pipefail

SCRATCH="$(mktemp --directory)"

(cd "${SCRATCH}" && wget --force-directories "$@")

gsutil cp -r "${SCRATCH}/*" "gs://ops-agents-public-buckets-vendored-deps/mirrored-content/"

set +x

echo "Content should be available at"
find "${SCRATCH}" -type f -printf 'https://storage.googleapis.com/ops-agents-public-buckets-vendored-deps/mirrored-content/%P\n'

rm -r "${SCRATCH}"
