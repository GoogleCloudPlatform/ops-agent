# Create a GCE Windows build VM

Create a Windows build VM for running Windows Dockerfile and set a password for `$USER`.

Note: The `--machine-type` and `--boot-disk-size` settings are required to
ensure the build VM has enough power/disk space to do the builds.

Note: Using `ssd` speeds up the build.

## Option 1: Create VM from a Public Image

1. Create the VM: 
    ```shell
    # VM image family.
    export WIN_BUILD_VM_OS_IMAGE_FAMILY=windows-2019

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


## Option 2: Create VM from a Pre-configured Custom Image

We maintain pre-configured Windows custom images internally to simplify the creation of VMs. 

Current available images: 

| Project ID              | Image Name                                                      | Base Image                                      | Family                      |
|-------------------------|-----------------------------------------------------------------|-------------------------------------------------|-----------------------------|
| stackdriver-test-143416 | ops-agent-build-windows-server-2019-dc-for-containers-v20220713 | windows-server-2019-dc-for-containers-v20220713 | windows-2019-for-containers |


1. Create the VM: 
    ```shell
    # Custom VM image.
    export CUSTOM_IMAGE_NAME=ops-agent-build-windows-server-2019-dc-for-containers-v20220713
    export CUSTOM_IMAGE_PATH=projects/stackdriver-test-143416/global/images/${CUSTOM_IMAGE_NAME}

    # Your dev GCP project ID.
    export VM_PROJECT_ID=${USER}-sandbox

    # The zone to create the VM in.
    export VM_ZONE=us-central1-a

    # The VM name to use.
    export WIN_BUILD_VM_NAME=${USER}-win-build-vm

    gcloud compute instances create \
        --project $VM_PROJECT_ID \
        --zone $VM_ZONE \
        --image $CUSTOM_IMAGE_PATH \
        --machine-type e2-highmem-8 \
        --boot-disk-type pd-ssd \
        --boot-disk-size 200GB \
        --scopes storage-rw,logging-write,monitoring-write \
        $WIN_BUILD_VM_NAME
    ```

2. Once the VM is running, reset the password:

    ```shell
    gcloud compute reset-windows-password --quiet \
        --project $VM_PROJECT_ID \
        --zone $VM_ZONE \
        --user=opsagentdev \
        $WIN_BUILD_VM_NAME
    ```

    Note that the default user for the VM is `opsagentdev`. 
    Record the password to use for connecting to this VM as `$WIN_BUILD_VM_PASSWORD`.

    Skip the steps in [Set up a Windows VM](set-up-windows-vm.md); follow the steps in [Connect to a Windows VM](connect-to-windows-vm.md) to connect via RDP. 
