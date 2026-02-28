#!/bin/bash
# setup_vm.sh - Provisioning script for Packer, executed via Shell Provisioner.
set -euo pipefail

# Source Image: rocky-linux-accelerator-cloud:rocky-linux-9-optimized-gcp-nvidia-580
# Source Image Description: Rocky Linux, Rocky Linux, 9 with the Nvidia 580 driver, x86_64 optimized for GCP with the Nvidia 580 driver built on {date}
# Output Image: stackdriver-test-143416:rocky-linux-9

# The accelerator image already has the driver (R580) installed. 
# Follow https://developer.nvidia.com/cuda-13-0-0-download-archive?target_os=Linux&target_arch=x86_64&Distribution=Rocky&target_version=9&target_type=rpm_network
# to install the matching CUDA toolkit 13.0 (without driver)
sudo dnf config-manager --add-repo https://developer.download.nvidia.com/compute/cuda/repos/rhel9/x86_64/cuda-rhel9.repo
sudo dnf clean all
sudo dnf -y install cuda-toolkit-13-0 git make