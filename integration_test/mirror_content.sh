#!/bin/bash

USAGE="Usage: ./mirror_content.sh <app> <URL>

This will download the content at <URL> and upload it to:

gs://ops-agents-public-buckets-vendored-deps/app-installers/<app>/<last part of URL>

This script can only be run by Googlers because release tests read from the
same bucket as presubmit tests, and we want release tests to be secure."

if [[ "$#" -ne 2 ]]; then
  echo "$USAGE"
  exit 1
fi

set -euxo pipefail

APP="$1"
URL="$2"

SCRATCH="$(mktemp --directory)"

(cd "${SCRATCH}" && curl -LO --fail-with-body "${URL}")

gsutil cp "${SCRATCH}/*" "gs://ops-agents-public-buckets-vendored-deps/app-installers/${APP}/"
