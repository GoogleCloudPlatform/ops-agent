#!/bin/bash
# setup_vm.sh - Provisioning script for Packer, executed via Shell Provisioner.
set -euo pipefail

# Source Image: debian-cloud:debian-12
# Output Image: stackdriver-test-143416:debian-12

sudo apt update -y
sudo apt install -y --no-install-recommends python3 python3-pip wget curl gnupg git || { echo "ERROR: Failed to install prerequisites!"; exit 1; }

INSTALLER_DIR="/var/lib/cuda-installer"
CUDA_INSTALLER_PATH="${INSTALLER_DIR}/cuda_installer.pyz"
sudo mkdir -p "${INSTALLER_DIR}"
sudo curl -L https://storage.googleapis.com/compute-gpu-installation-us/installer/latest/cuda_installer.pyz --output "${CUDA_INSTALLER_PATH}"
sudo chmod +x "${CUDA_INSTALLER_PATH}"

sudo python3 "${CUDA_INSTALLER_PATH}" install_driver --ignore-no-gpu --installation-mode=repo --installation-branch=nfb || { echo "ERROR: cuda_installer.pyz install_driver failed!"; exit 1; }
# The script will reboot
