#!/bin/bash
# Copyright 2024 Google LLC
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


# See go/sdi-new-distro-detector for details on how this script is being used.
# A short summary is that it is run periodically by GCB to file bugs when
# the list of default distro families changes.
set -ex
set -o pipefail

PATTERNS_FILE="irrelevant_family_patterns.txt"
echo '^cos-
^fedora-
^rocky-linux-[0-9]+-optimized-gcp
^sql-
^ubuntu-pro-
^ubuntu-accelerator
^ubuntu-minimal-' > "${PATTERNS_FILE}"

# Remove families that require no action on our part. This consists of both
# a) unsupported families, namely cos-.*, and
# b) families already "covered" by other families on the list. For example,
#    support for rhel-.* follows immediately from support for the matching
#    version of rocky-linux, so we don't need to do anything special to
#    support RHEL.
function strip_irrelevant_families() {
  cat - | grep --extended-regexp --invert-match --file "${PATTERNS_FILE}"
}
# Fetch the list of families provided by default on GCE. Set
# --project=windows-cloud to avoid including images from the current
# project.  Don't worry, it will still include images from debian-cloud,
# suse-cloud, etc.
# sort --unique removes duplicates that appear in the list for some reason.
gcloud compute images list --sort-by=FAMILY --format='value(FAMILY)' --standard-images --preview-images --project=windows-cloud \
  | sort --unique \
  | strip_irrelevant_families \
  | tee current_families.txt

# Fetch the list of relevant families as of the last run.
LAST_KNOWN_LIST="gs://stackdriver-test-143416-new-distro-detector/list_of_families.txt"
gsutil -q cp "${LAST_KNOWN_LIST}" - \
  | tee known_families.txt

# If there is a difference, print the diff, and...
if ! diff --ignore-all-space --ignore-blank-lines known_families.txt current_families.txt; then
  # Print instructions for handling the error.
  # Set +x temporarily so that the banner is only printed once.
  set +x
  echo '
    ####################################################
    #  See go/sdi-new-distro-detector#handling-errors  #
    #  for instructions on handling this error.        #
    ####################################################
  '
  set -x
  # Upload the current list to the GCS bucket so that the next run passes.
  gsutil -q cp current_families.txt "${LAST_KNOWN_LIST}"
  # Report an error, which will result in a bug being filed.
  exit 1
fi
