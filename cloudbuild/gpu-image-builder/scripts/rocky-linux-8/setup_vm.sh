#!/bin/bash
# setup_gpu_apps.sh - Provisioning script for Packer on Rocky Linux 8, executed via Shell Provisioner.
set -euo pipefail

echo "--- Starting Packer Provisioning on Rocky Linux 8 $(date) ---"

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

# Convert CUDA_VERSION (e.g., 12.2.2) to the format used in package names (e.g., 12-2)
CUDA_VERSION_SHORT="${CUDA_VERSION%.*}" # 12.2
CUDA_VERSION_DASHED="${CUDA_VERSION_SHORT//./-}" # 12-2

# --- NVIDIA Repository Setup for Rocky Linux 8 ---
echo "--- Configuring NVIDIA CUDA Repository for Rocky Linux 8 ---"
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/rhel8/x86_64/cuda-rhel8.repo

sudo dnf clean all
sudo dnf makecache
sudo dnf install -y git make

# --- Install CUDA Toolkit and Samples ---
echo "--- Installing CUDA Toolkit ${CUDA_VERSION} and Samples ---"

echo "Installing CUDA Toolkit and Samples..."
sudo dnf install -y  cuda-"${CUDA_VERSION_DASHED}"

# # --- Create Symbolic Link ---
# # CUDA on Linux often installs to /usr/local/cuda-${CUDA_VERSION_SHORT}
# TARGET_DIR="/usr/local/cuda-${CUDA_VERSION_SHORT}"
# LINK_NAME="/usr/local/cuda"

# if [ -d "$TARGET_DIR" ]; then
#     echo "Creating symbolic link: $LINK_NAME -> $TARGET_DIR"
#     sudo ln -snf "$TARGET_DIR" "$LINK_NAME"

#     echo "----------------------------------------------------------------"
#     echo "Success! CUDA Toolkit and Samples installed."
#     echo "Symlink created at $LINK_NAME"
#     echo "Samples located at: $LINK_NAME/samples/"
#     echo "----------------------------------------------------------------"
# else
#     echo "Error: Installation completed, but target directory $TARGET_DIR was not found."
#     exit 1
# fi
echo "--- Provisioning script finished $(date) ---"
