#!/bin/bash
# setup_vm.sh - Provisioning script for Packer, executed via Shell Provisioner.
set -euo pipefail

# Source Image: debian-cloud:debian-13
# Output Image: stackdriver-test-143416:debian-13

# Install driver and CUDA toolkit
sudo apt update -y
KERNEL_VERSION=`uname -r`
sudo apt install -y linux-headers-${KERNEL_VERSION} pciutils gcc make dkms wget git

wget https://developer.download.nvidia.com/compute/cuda/repos/debian13/x86_64/cuda-keyring_1.1-1_all.deb
sudo dpkg -i cuda-keyring_1.1-1_all.deb
sudo apt update

sudo apt -y install cuda-13-1

