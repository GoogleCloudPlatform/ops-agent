#!/bin/bash
# post_reboot.sh - Runs setup steps after the VM has rebooted on Debian 12. Provisioning script for Packer, executed via Shell Provisioner.
set -euo pipefail

INSTALLER_DIR="/var/lib/cuda-installer"
CUDA_INSTALLER_PATH="${INSTALLER_DIR}/cuda_installer.pyz"

# Rerun `install_driver` to finish driver installation
sudo python3 "${CUDA_INSTALLER_PATH}" install_driver --ignore-no-gpu --installation-mode=repo --installation-branch=nfb || { echo "ERROR: cuda_installer.pyz install_driver failed!"; exit 1; }

# Install CUDA toolkit
sudo python3 "${CUDA_INSTALLER_PATH}" install_cuda --ignore-no-gpu --installation-mode=repo --installation-branch=nfb || { echo "ERROR: cuda_installer.pyz install_cuda failed!"; exit 1; }
