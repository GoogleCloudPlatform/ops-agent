#!/bin/bash

# See go/sdi-new-distro-detector for details on how this script is being used.
# A short summary is that it is run periodically by GCB to file bugs when
# the list of default distro families changes.
set -ex
set -o pipefail

PATTERNS_FILE="irrelevant_family_patterns.txt"
echo '^cos-
^fedora-
^rhel-
^rocky-linux-[0-9]+-optimized-gcp
^sql-
^ubuntu-pro-
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
gcloud compute images list --sort-by=FAMILY --format='value(FAMILY)' --standard-images --project=windows-cloud \
  | sort --unique \
  | strip_irrelevant_families \
  | tee current_families.txt

# Fetch the list of relevant families as of the last run.
LAST_KNOWN_LIST="gs://stackdriver-test-143416-new-distro-detector/list_of_families.txt"
gsutil -q cp "${LAST_KNOWN_LIST}" - \
  | tee known_families.txt

# If there is a difference, print the diff, and...
if ! diff known_families.txt current_families.txt; then
  # Print instructions for handling the error.
  echo 'See go/sdi-new-distro-detector#handling-errors for instructions on handling this error.",'
  # Upload the current list to the GCS bucket so that the next run passes.
  gsutil -q cp current_families.txt "${LAST_KNOWN_LIST}"
  # Report an error, which will result in a bug being filed.
  exit 1
fi
