#!/bin/bash
set -eo pipefail

# This script is the entry point. It resolves environment-specific
# variables, changes into the correct directory, and then calls the
# generic builder.sh script.

# Ensure required environment variables exist.
if [[ -z "$_LOUHI_TAG_NAME" || -z "$KOKORO_GFILE_DIR" || -z "$KOKORO_ARTIFACTS_DIR" || -z "$_STAGING_ARTIFACTS_PROJECT_ID" ]]; then
    echo "Error: Required environment variables are not set." >&2
    exit 1
fi

# Parse the Louhi tag to get the architecture.
# Example value: louhi/2.46.0/abcdef/windows/x86_64/start
IFS="/"
read -ra louhi_tag_components <<< "$_LOUHI_TAG_NAME"
Arch="${louhi_tag_components[4]}"

# Define paths based on Kokoro environment variables.
INPUT_DIR="$KOKORO_GFILE_DIR"
OUTPUT_DIR="${KOKORO_ARTIFACTS_DIR}/result"

# Change into the working directory for the build.
echo "Changing directory to git/unified_agents..."
cd git/unified_agents

# Execute the main builder script, which is now in the parent directory.
# Pass variables as arguments.
./kokoro/scripts/build/packaging/package_windows.sh -a "$Arch" -i "$INPUT_DIR" -o "$OUTPUT_DIR"

# After the build, upload the artifacts using Louhi/Kokoro variables.
echo "Uploading artifacts..."
ver="${louhi_tag_components[1]}"
ref="${louhi_tag_components[2]}"
target="${louhi_tag_components[3]}"

gcs_bucket="gs://${_STAGING_ARTIFACTS_PROJECT_ID}-ops-agent-releases/${ver}/${ref}/${target}/${Arch}/"

echo "Copying *.goo files to ${gcs_bucket}"
gsutil cp "${OUTPUT_DIR}"/*.goo "${gcs_bucket}"

echo "Script finished successfully."
