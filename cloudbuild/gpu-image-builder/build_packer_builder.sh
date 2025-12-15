#!/bin/bash
# build_packer_builder.sh
# Builds the custom Packer Cloud Build builder if it doesn't exist.
# https://docs.cloud.google.com/build/docs/building/build-vm-images-with-packer

set -euo pipefail

PROJECT_ID="${1}"
PACKER_BUILDER_IMAGE="gcr.io/${PROJECT_ID}/packer"

if gcloud container images describe "${PACKER_BUILDER_IMAGE}" > /dev/null 2>&1; then
  echo "Packer builder image '${PACKER_BUILDER_IMAGE}' exists, skipping build."
else
  echo "Packer builder image not found. Building it now..."
  git clone https://github.com/GoogleCloudPlatform/cloud-builders-community.git --depth=1
  cd cloud-builders-community/packer
  gcloud builds submit --project="${PROJECT_ID}" . 
  cd -
  echo "Packer builder image built."
fi