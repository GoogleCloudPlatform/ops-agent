#!/bin/bash
# setup_gpu_apps.sh - Provisioning script for Packer, executed via Shell Provisioner.
set -euo pipefail

echo "--- Starting Packer Provisioning $(date) ---"

# --- Input Variables from PACKER_VAR_* environment variables ---
PROJECT_ID="${PACKER_VAR_project_id}"
GPU_DRIVER_VERSION="${PACKER_VAR_gpu_driver_version}" # e.g., 535.161.01
CUDA_VERSION="${PACKER_VAR_cuda_version}" # e.g., 12.2.2
BUILD_ID="${PACKER_VAR_build_id}"

echo "Fetched variables from PACKER_VAR_*:"
echo "  PROJECT_ID: ${PROJECT_ID}"
echo "  GPU_DRIVER_VERSION: ${GPU_DRIVER_VERSION}"
echo "  CUDA_VERSION: ${CUDA_VERSION}"
echo "  BUILD_ID: ${BUILD_ID}"

retry_command() {
    local max_attempts="$1"
    local sleep_time="$2"
    local cmd="$3"

    echo "Starting command: $cmd"
    echo "----------------------------------------"

    for ((i=1; i<=max_attempts; i++)); do
        echo "[Attempt $i/$max_attempts] Running..."
        
        # Run the command using bash -c to handle complex commands (like those with &&)
        if bash -c "$cmd"; then
            echo "----------------------------------------"
            echo "Success!"
            return 0
        fi

        echo "Attempt failed."

        # Sleep only if we have attempts left
        if [ $i -lt $max_attempts ]; then
            echo "Waiting $sleep_time seconds before retrying..."
            sleep $sleep_time
        fi
    done

    echo "----------------------------------------"
    echo "Error: Command failed after $max_attempts attempts."
    exit 1
}

retry_command 5 5 "sudo /usr/sbin/registercloudguest --force"
retry_command 120 5 "sudo zypper --non-interactive --gpg-auto-import-keys refresh && sudo zypper --non-interactive install --force coreutils"

sudo zypper --non-interactive install -y kernel-default-devel=$(uname -r | sed 's/\-default//') pciutils gcc make wget git

# Install CUDA and driver together, since the `exercise` script needs to run a
# CUDA sample app to generating GPU process metrics
# Prefer to install from the package manager since it is normally faster and has
# less errors on installation; fallback to the runfile method if the package
# manager's package is not working or not compitible with the GPU model
DISTRIBUTION=$(. /etc/os-release;echo $ID$VERSION_ID | sed -e 's/\.[0-9]//')
echo "Installing latest version of NVIDIA CUDA and driver"
sudo zypper --non-interactive ar http://developer.download.nvidia.com/compute/cuda/repos/${DISTRIBUTION}/x86_64/cuda-${DISTRIBUTION}.repo
sudo zypper --gpg-auto-import-keys --non-interactive refresh
sudo zypper --non-interactive install -y nvidia-compute-utils-G06
sudo zypper --non-interactive install -y cuda-12-9

