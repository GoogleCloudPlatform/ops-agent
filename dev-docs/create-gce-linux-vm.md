# Create a GCE Linux VM

To find the available OS image project and family values, see
https://cloud.google.com/compute/docs/images/os-details or use the following
gcloud command:

```shell
# A keyword to search for.
$ export IMAGE_SEARCH_KEYWORD=debian

$ gcloud compute images list | grep $IMAGE_SEARCH_KEYWORD
```

```shell
# VM image project.
$ export OS_IMAGE_PROJECT=debian-cloud

# VM image family.
$ export OS_IMAGE_FAMILY=debian-10

# Your dev GCP project ID.
$ export VM_PROJECT_ID=${USER}-sandbox

# The zone to create the VM in.
$ export VM_ZONE=us-central1-a

# The VM name to use.
$ export VM_NAME=${USER}-vm

$ gcloud compute instances create \
    --image-project $OS_IMAGE_PROJECT \
    --image-family $OS_IMAGE_FAMILY \
    --zone $VM_ZONE \
    --project $VM_PROJECT_ID \
    $VM_NAME
```
