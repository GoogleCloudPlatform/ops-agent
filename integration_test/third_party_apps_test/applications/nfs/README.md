# NFS

This third party app test differs from the rest. The third party app test framework is used because it is somewhat similar (there is need to install and exercise a third party application before the test can work) but it does not directly map to a third party application receiver. A `files` receiver reads a log file from the shared NFS directory, and the expectations are:
* The logs are read and sent to Cloud Logging
* The tailed log file getting deleted from the directory is registered by the Ops Agent, and the `.nfsxxxxx` files are deleted as well

## Shared values

There is no official way to share values among these scripts so certain values are assumed across scripts:
* NFS Share directory: `/mnt/nfssrv`
* NFS Mount directory: `/var/nfsmnt`
* Name of log file in NFS share directory: `example.log`