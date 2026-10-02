# Create a GCE Windows build VM

Create a Windows build VM for running Windows Dockerfile and set a password for `$USER`.

Note: The `--machine-type` and `--boot-disk-size` settings are required to
ensure the build VM has enough power/disk space to do the builds.

Note: Using `ssd` speeds up the build.

## Create VM from a Public Image

1. Create the VM: 
    ```shell
    # VM image family. Use the Windows Server version that matches
    # WINDOWS_VERSION in Dockerfile.windows (ltsc2022 by default).
    export WIN_BUILD_VM_OS_IMAGE_FAMILY=windows-2022

    # Your dev GCP project ID.
    export VM_PROJECT_ID=${USER}-sandbox

    # The zone to create the VM in.
    export VM_ZONE=us-central1-a

    # The VM name to use.
    export WIN_BUILD_VM_NAME=${USER}-win-build-vm

    gcloud compute instances create \
        --project $VM_PROJECT_ID \
        --zone $VM_ZONE \
        --image-project windows-cloud \
        --image-family $WIN_BUILD_VM_OS_IMAGE_FAMILY \
        --machine-type e2-highmem-8 \
        --boot-disk-type pd-ssd \
        --boot-disk-size 200GB \
        --scopes storage-rw,logging-write,monitoring-write \
        $WIN_BUILD_VM_NAME
    ```

1. Once the VM is running, reset the password:

    ```shell
    gcloud compute reset-windows-password --quiet \
        --project $VM_PROJECT_ID \
        --zone $VM_ZONE \
        $WIN_BUILD_VM_NAME
    ```

    Record the password to use for connecting to this VM as `$WIN_BUILD_VM_PASSWORD`.
