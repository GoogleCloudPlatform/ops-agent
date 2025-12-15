#!/bin/bash
# post_reboot_gpu_setup.sh - Runs setup steps after the VM has rebooted on Debian 12.
set -euo pipefail
echo "--- Starting Packer Post-Reboot Provisioning on Debian 12 $(date) ---"

PROJECT_ID="${PACKER_VAR_project_id}"
CUDA_VERSION="${PACKER_VAR_cuda_version}" # e.g., 12.2.2
BUILD_ID="${PACKER_VAR_build_id}"

echo "Fetched variables from PACKER_VAR_*:"
echo "  PROJECT_ID: ${PROJECT_ID}"
echo "  CUDA_VERSION: ${CUDA_VERSION}"
echo "  BUILD_ID: ${BUILD_ID}"

# --- Persistent Installer Path ---
INSTALLER_DIR="/var/lib/cuda-installer"
CUDA_INSTALLER_PATH="${INSTALLER_DIR}/cuda_installer.pyz"

# Ensure the installer exists (it should, as it was downloaded in the first phase)
if [ ! -f "${CUDA_INSTALLER_PATH}" ]; then
  echo "ERROR: cuda_installer.pyz not found at ${CUDA_INSTALLER_PATH}!"
  exit 1
fi

echo "Running cuda_installer.pyz install_driver --ignore-no-gpu --installation-mode=repo --installation-branch=nfb"
sudo python3 "${CUDA_INSTALLER_PATH}" install_driver --ignore-no-gpu --installation-mode=repo --installation-branch=nfb || { echo "ERROR: cuda_installer.pyz install_driver failed!"; exit 1; }

echo "Running cuda_installer.pyz install_cuda --ignore-no-gpu --installation-mode=repo --installation-branch=nfb"
sudo python3 "${CUDA_INSTALLER_PATH}" install_cuda --ignore-no-gpu --installation-mode=repo --installation-branch=nfb || { echo "ERROR: cuda_installer.pyz install_cuda failed!"; exit 1; }

# # --- Install CUDA Samples ---
# echo "--- Installing CUDA Samples for ${CUDA_VERSION} ---"
# # Convert CUDA_VERSION (e.g., 12.2.2) to the apt package format (e.g., 12-2)
# CUDA_VERSION_SHORT="${CUDA_VERSION%.*}" # 12.2
# CUDA_VERSION_DASHED="${CUDA_VERSION_SHORT//./-}" # 12-2
# CUDA_SAMPLES_PACKAGE="cuda-demo-suite-${CUDA_VERSION_DASHED}"

# # Ensure NVIDIA repo is added - it should have been by cuda_installer.pyz or is pre-configured
# sudo apt-get update -y

# if apt-cache show "${CUDA_SAMPLES_PACKAGE}" &> /dev/null; then
#     echo "Package '${CUDA_SAMPLES_PACKAGE}' found. Installing..."
#     sudo apt-get install -y --no-install-recommends "${CUDA_SAMPLES_PACKAGE}" || { echo "ERROR: Failed to install ${CUDA_SAMPLES_PACKAGE}!"; exit 1; }
#     echo "CUDA Samples installed."
# else
#     echo "WARNING: CUDA Samples package '${CUDA_SAMPLES_PACKAGE}' not found in repositories."
#     echo "You may need to manually add the NVIDIA repos or check package naming for Debian 12."
# fi

# # --- Create Symbolic Link ---
# TARGET_DIR="/usr/local/cuda-${CUDA_VERSION_SHORT}"
# LINK_NAME="/usr/local/cuda"
# if [ -d "$TARGET_DIR" ]; then
#     echo "Creating symbolic link: $LINK_NAME -> $TARGET_DIR"
#     sudo ln -snf "$TARGET_DIR" "$LINK_NAME"
#     echo "Symlink created at $LINK_NAME"
# else
#     echo "WARNING: Target directory $TARGET_DIR not found for symlink."
# fi

# echo "--- Cleaning up ---"
# sudo apt-get clean
# sudo rm -rf /var/lib/apt/lists/*
# # The cuda_installer.pyz is deliberately left in ${INSTALLER_DIR} as requested.
# echo "--- Packer post-reboot setup complete $(date) ---"
# echo "--- Provisioning script finished $(date) ---"
