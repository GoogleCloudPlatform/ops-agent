#!/bin/bash
# setup_vm.sh - Provisioning script for Packer, executed via Shell Provisioner.
set -euo pipefail

# Source Image: ubuntu-os-accelerator-images:ubuntu-accelerator-2404-amd64-with-nvidia-580
# Source Image Description: Canonical, Ubuntu, 24.04 LTS NVIDIA version: 580, amd64 noble image built on {date}
# Output Image: stackdriver-test-143416:ubuntu-2404-lts

# The accelerator image already has the driver (R580) installed. 
# Follow https://developer.nvidia.com/cuda-13-0-0-download-archive?target_os=Linux&target_arch=x86_64&Distribution=Ubuntu&target_version=24.04&target_type=deb_network
# to install the matching CUDA toolkit 13.0 (without driver)
wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2404/x86_64/cuda-keyring_1.1-1_all.deb
sudo dpkg -i cuda-keyring_1.1-1_all.deb
sudo apt-get update
sudo apt-get -y install build-essential cuda-toolkit-13-0