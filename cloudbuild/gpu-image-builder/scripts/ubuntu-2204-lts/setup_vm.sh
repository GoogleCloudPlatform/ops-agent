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

CUDA_VERSION_SHORT="${CUDA_VERSION%.*}"
CUDA_VERSION_DASHED="${CUDA_VERSION_SHORT//./-}"
PACKAGE_NAME="cuda-${CUDA_VERSION_DASHED}"

echo "Target package name: $PACKAGE_NAME"

wget --no-verbose https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2204/x86_64/cuda-keyring_1.1-1_all.deb
sudo dpkg -i cuda-keyring_1.1-1_all.deb
sudo apt-get update
sudo apt-get install -y build-essential 

# Check if the exact package exists and install
if apt-cache show "$PACKAGE_NAME" &> /dev/null; then
    echo "Package '$PACKAGE_NAME' found. Installing..."
    
    sudo apt-get install -y --no-install-recommends "$PACKAGE_NAME"
    
    # Create Symbolic Link
    # We use -snf to force the link creation and prevent dereferencing if it already exists
    TARGET_DIR="/usr/local/cuda-$CUDA_VERSION_SHORT"
    LINK_NAME="/usr/local/cuda"
    
    if [ -d "$TARGET_DIR" ]; then
        echo "Creating symbolic link: $LINK_NAME -> $TARGET_DIR"
        sudo ln -snf "$TARGET_DIR" "$LINK_NAME"
        
        echo "----------------------------------------------------------------"
        echo "Success! Samples installed."
        echo "Symlink created at $LINK_NAME"
        echo "Binaries located at: $LINK_NAME/extras/demo_suite/"
        echo "----------------------------------------------------------------"
    else
        echo "Error: Installation completed, but target directory $TARGET_DIR was not found."
        exit 1
    fi
else
    echo "Error: Package '$PACKAGE_NAME' was not found in your repositories."
    exit 1
fi