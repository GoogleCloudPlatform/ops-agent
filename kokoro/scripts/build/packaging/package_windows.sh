#!/bin/bash
set -eo pipefail

# This script performs the core build and packaging process.
# It receives all paths and variables as arguments and expects to be
# run from the 'git/unified_agents' directory.

helpFunction() {
    echo ""
    echo "Usage: $0 -a Arch -o OutputDirectory -i InputDirectory"
    echo -e "\t-a Architecture of the binary (must be x86_64 or x86)."
    echo -e "\t-o Directory to place the final packaged GooGet file."
    echo -e "\t-i Directory containing pre-built results and signed scripts."
    exit 1
}

while getopts "a:o:i:" opt; do
    case "$opt" in
        a) Arch="$OPTARG" ;;
        o) OutputDir="$OPTARG" ;;
        i) InputDir="$OPTARG" ;;
        *) helpFunction ;;
    esac
done

# Validate that all mandatory arguments were provided.
if [ -z "$Arch" ] || [ -z "$OutputDir" ] || [ -z "$InputDir" ]; then
    echo "Error: Missing required arguments."
    helpFunction
fi

# Set GOARCH based on Arch.
case $Arch in
    "x86_64") GoArch="amd64" ;;
    "x86") GoArch="386" ;;
    *)
      echo "ERROR: Architecture must be set to one of: x86, x86_64" >&2
      exit 1
      ;;
esac

echo "Starting build for Arch: ${Arch} (GoArch: ${GoArch})"

# The working directory is now set by the calling script.
git config --global --add safe.directory "$(pwd)"

echo "Installing goopack..."
go install -trimpath -ldflags="-s -w" github.com/google/googet/v2/goopack@latest

echo "Preparing directories and files..."
mkdir -p out
mkdir -p "$OutputDir"

# Move pre-built files from the input directory into the current structure.
mv "${InputDir}/result/out/"* ./out/
mv "${InputDir}/result/pkg/goo/maint.ps1" ./pkg/goo/maint.ps1

echo "Current directory contents:"
ls -la

# Extract the package version from the VERSION file in the current directory.
releaseName=$(awk -F "=" '/PKG_VERSION/ {print $2}' ./VERSION | tr -d '"')
echo "Building package version: ${releaseName}"

# Run the packager. Paths are relative to the current directory.
"$GOPATH"/bin/goopack -output_dir "$OutputDir" \
    -var:PKG_VERSION="$releaseName" \
    -var:ARCH="$Arch" \
    -var:GOOS=windows \
    -var:GOARCH="$GoArch" \
    pkg/goo/google-cloud-ops-agent.goospec

echo "Build process complete. Output at: ${OutputDir}"
