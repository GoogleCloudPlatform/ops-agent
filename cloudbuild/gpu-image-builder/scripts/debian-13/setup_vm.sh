#!/bin/bash
# initial_gpu_setup.sh - Runs initial setup and GPU driver installation on Debian 13.
set -euo pipefail
echo "--- Starting Packer Initial Provisioning on Debian 13 $(date) ---"

# --- Input Variables from PACKER_VAR_* environment variables ---
PROJECT_ID="${PACKER_VAR_project_id}"
# GPU_DRIVER_VERSION is not directly used by cuda_installer.pyz install_driver
BUILD_ID="${PACKER_VAR_build_id}"

echo "Fetched variables from PACKER_VAR_*:"
echo "  PROJECT_ID: ${PROJECT_ID}"
echo "  BUILD_ID: ${BUILD_ID}"


sudo apt update
KERNEL_VERSION=`uname -r`
sudo apt install -y linux-headers-${KERNEL_VERSION} pciutils gcc make dkms wget git

wget https://developer.download.nvidia.com/compute/cuda/repos/debian13/x86_64/cuda-keyring_1.1-1_all.deb
sudo dpkg -i cuda-keyring_1.1-1_all.deb
sudo apt-get update

sudo apt-get -y install cuda-13-1

