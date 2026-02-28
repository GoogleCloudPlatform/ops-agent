#!/bin/bash
# setup_vm.sh - Provisioning script for Packer, executed via Shell Provisioner.
set -euo pipefail

# Source Image: suse-cloud:sles-15
# Output Image: stackdriver-test-143416:sles-15

# Mimic our prepareSLES() logic in gce_testing.go
# https://github.com/GoogleCloudPlatform/opentelemetry-operations-collector/blob/ec757f2f48c865c7aa1afaed27891d8727a28f2e/integration_test/gce-testing-internal/gce/gce_testing.go#L1057
retry_command() {
    local max_attempts="$1"
    local sleep_time="$2"
    local cmd="$3"

    echo "Starting command: $cmd"
    echo "----------------------------------------"

    for ((i=1; i<=max_attempts; i++)); do
        echo "[Attempt $i/$max_attempts] Running..."
        
        # Run the command using bash -c to handle complex commands (like those with &&)
        if bash -c "$cmd"; then
            echo "----------------------------------------"
            echo "Success!"
            return 0
        fi

        echo "Attempt failed."

        # Sleep only if we have attempts left
        if [ $i -lt $max_attempts ]; then
            echo "Waiting $sleep_time seconds before retrying..."
            sleep $sleep_time
        fi
    done

    echo "----------------------------------------"
    echo "Error: Command failed after $max_attempts attempts."
    exit 1
}

retry_command 5 5 "sudo /usr/sbin/registercloudguest --force"
retry_command 120 5 "sudo zypper --non-interactive --gpg-auto-import-keys refresh && sudo zypper --non-interactive install --force coreutils"

sudo zypper --non-interactive install -y kernel-default-devel=$(uname -r | sed 's/\-default//') pciutils gcc make wget git

# Install CUDA and driver together, since the `exercise` script needs to run a
# CUDA app to generating GPU process metrics
# Prefer to install from the package manager since it is normally faster and has
# less errors on installation. The cuda-12-9 mega-package installs driver and 
# CUDA together
sudo zypper --non-interactive addrepo https://developer.download.nvidia.com/compute/cuda/repos/sles15/x86_64/cuda-sles15.repo
sudo zypper --gpg-auto-import-keys --non-interactive refresh
# CUDA 13 is not yet working with the SLES 15 image
sudo zypper --non-interactive install -y nvidia-compute-utils-G06
sudo zypper --non-interactive install -y cuda-12-9
