#!/bin/bash
# check_source_image.sh
# Checks if the latest public image is newer than the source of our last build and if we need a new build

set -euo pipefail

PROJECT_ID="${1}"
SOURCE_IMAGE_FAMILY="${2}"
SOURCE_IMAGE_PROJECT="${3}"
TARGET_IMAGE_FAMILY="${4}"
# Louhi set trigger type as either "cron-trigger" or "git-change-trigger"
LOUHI_TRIGGER_TYPE="${5}" 

echo "--- Checking for New Source Image ---"
LATEST_PUBLIC_IMAGE=$(gcloud compute images describe-from-family "${SOURCE_IMAGE_FAMILY}" --project="${SOURCE_IMAGE_PROJECT}" --format="value(name)")
echo "Latest available public image: ${LATEST_PUBLIC_IMAGE}"

LAST_CURATED_SOURCE_IMAGE=""
if gcloud compute images describe-from-family "${TARGET_IMAGE_FAMILY}" --project="${PROJECT_ID}" &> /dev/null; then
  LAST_CURATED_SOURCE_IMAGE=$(gcloud compute images describe-from-family "${TARGET_IMAGE_FAMILY}" --project="${PROJECT_ID}" --format="value(labels.source-gce-image)")
  echo "Source image of our latest curated image: ${LAST_CURATED_SOURCE_IMAGE}"
else
  echo "Image family '${TARGET_IMAGE_FAMILY}' not found. Assuming this is the first build."
fi

# Only skip when running nightly, and there is no new base image
if [[ "${LATEST_PUBLIC_IMAGE}" == "${LAST_CURATED_SOURCE_IMAGE}" ]] && \
   [[ "${LOUHI_TRIGGER_TYPE}" == "cron-trigger" ]]; then
  echo "Source image '${LATEST_PUBLIC_IMAGE}' has not changed. Signaling to skip build."
  echo "SKIP" > /workspace/build_status.txt
# Else, we either have a new image, or this is trigger by git changes
# Note that we set the Louhi Git trigger to only watch cloudbuild/gpu-image-builder directory
else
  if [[ "${LATEST_PUBLIC_IMAGE}" != "${LAST_CURATED_SOURCE_IMAGE}" ]]; then
    echo "New source image '${LATEST_PUBLIC_IMAGE}' detected or first run. Signaling to run build."
  else
    echo "New image building triggered by GitHub changes (Louhi trigger type = '${LOUHI_TRIGGER_TYPE}')"
  fi
  echo "${LATEST_PUBLIC_IMAGE}" > /workspace/new_source_image.txt
  echo "RUN" > /workspace/build_status.txt
fi
