#!/bin/bash
# setup_vm.sh - Provisioning script for Packer, executed via Shell Provisioner.
set -euo pipefail

# Source Image: ml-images:common-gpu-debian-11-py310
# Source Image description: Google, Deep Learning VM with CUDA 11.8, M126, Debian 11, Python 3.10. With CUDA 11.8 preinstalled.
# Output Image: stackdriver-test-143416:debian-11

# DLVM images come with a script to install the driver and CUDA toolkit. 
/opt/deeplearning/install-driver.sh