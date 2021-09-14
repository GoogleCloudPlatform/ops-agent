# Create a GCE Windows test VM

See a list of all the Windows VM images we support.

```shell
$ gcloud compute images list | grep windows | grep -v sql | grep -v container
```

Create a Windows VM and set a password for ${USER}.

Tip: Avoid VMs with `-core` in the NAME family if you need to use a GUI.

> Tip: [Windows versions](https://en.wikipedia.org/wiki/Windows_Server) can be
> confusing.
>
> Some versions are 4-digit year:
>
> *   Windows Server 2012
> *   Windows Server 2016
> *   Windows Server 2019
>
> Some versions are 2-digit year + 2-digit month
>
> *   Windows Server, version 1809 # 2018 September
> *   Windows Server, version 1903 # 2019 March
> *   Windows Server, version 1909 # 2019 September
> *   Windows Server, version 2004 # 2020 April
>
> Don't get confused - "2012" is a year (and fairly old); "2004" is a build
> number and NEWER than 2012.

```shell
# Your dev GCP project ID.
$ export VM_PROJECT_ID=${USER}-sandbox

# The zone to create the VM in.
$ export VM_ZONE=us-central1-a

# VM image family.
$ export WIN_TEST_VM_OS_IMAGE_FAMILY=windows-2016

# The VM name to use.
$ export WIN_TEST_VM_NAME=${USER}-windows-vm

$ gcloud compute instances create \
    --project $VM_PROJECT_ID \
    --zone $VM_ZONE \
    --image-project windows-cloud \
    --image-family $WIN_TEST_VM_OS_IMAGE_FAMILY \
    $WIN_TEST_VM_NAME

$ gcloud compute reset-windows-password --quiet \
    --project $VM_PROJECT_ID \
    --zone $VM_ZONE \
    $WIN_TEST_VM_NAME
```

Record the password to use for connecting to this Windows testing VM as
`$WIN_TEST_VM_PASSWORD`.

