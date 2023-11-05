#!/bin/bash
# Copyright 2023 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


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
