#!/bin/bash
# initial_gpu_setup.sh - Runs initial setup and GPU driver installation on Debian 12.
set -euo pipefail
echo "--- Starting Packer Initial Provisioning on Debian 12 $(date) ---"

# --- Input Variables from PACKER_VAR_* environment variables ---
PROJECT_ID="${PACKER_VAR_project_id}"
# GPU_DRIVER_VERSION is not directly used by cuda_installer.pyz install_driver
BUILD_ID="${PACKER_VAR_build_id}"

echo "Fetched variables from PACKER_VAR_*:"
echo "  PROJECT_ID: ${PROJECT_ID}"
echo "  BUILD_ID: ${BUILD_ID}"

echo "--- Running apt updates and installing prerequisites ---"
sudo apt-get update -y
sudo apt-get install -y --no-install-recommends python3 python3-pip wget curl gnupg git || { echo "ERROR: Failed to install prerequisites!"; exit 1; }

echo "--- Installing GPU Driver using cuda_installer.pyz ---"
INSTALLER_DIR="/var/lib/cuda-installer"
CUDA_INSTALLER_PATH="${INSTALLER_DIR}/cuda_installer.pyz"
sudo mkdir -p "${INSTALLER_DIR}"
sudo curl -L https://storage.googleapis.com/compute-gpu-installation-us/installer/latest/cuda_installer.pyz --output "${CUDA_INSTALLER_PATH}"
sudo chmod +x "${CUDA_INSTALLER_PATH}"

echo "Running cuda_installer.pyz install_driver --ignore-no-gpu --installation-mode=repo --installation-branch=nfb"
sudo python3 "${CUDA_INSTALLER_PATH}" install_driver --ignore-no-gpu --installation-mode=repo --installation-branch=nfb || { echo "ERROR: cuda_installer.pyz install_driver failed!"; exit 1; }
# The script will reboot