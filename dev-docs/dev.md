# Ops Agent development

Cloud Operations Suite Agent (Ops Agent) combines logging and metrics into a
single agent. Under the hood, it embeds
[Fluent Bit](https://github.com/fluent/fluent-bit) as a subagent for logging,
and embeds
[OpenTelemetry Operations collector](https://github.com/GoogleCloudPlatform/opentelemetry-operations-collector)
as a subagent for metrics.

## Where code lives

Detailed structure of the repo:

<details>
<summary>Config generation related</summary>

Description                                                                                                                                                                                      | Link
------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----
Default config                                                                                                                                                                                   | [confgenerator/default-config.yaml](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/confgenerator/default-config.yaml) (Linux) and [confgenerator/windows-default-config.yaml](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/confgenerator/windows-default-config.yaml)(Windows)
Unified agent config generator. This handles the unified agent yaml reading and delegating subsections to the Open Telemetry Collector config translator and Fluent Bit config translator below. | [confgenerator/confgenerator.go](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/confgenerator/confgenerator.go) (implementation), [confgenerator/confgenerator_test.go](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/confgenerator/confgenerator_test.go) (unit tests), [confgenerator/testdata](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/confgenerator/testdata) (test data)
generate_config command implementation                                                                                                                                                           | [cmd/google_cloud_ops_agent_engine/main.go](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/cmd/google_cloud_ops_agent_engine/main.go) (Linux) and [cmd/ops_agent_windows/main_windows.go](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/cmd/google_cloud_ops_agent_engine/main.go) (Windows)
Open Telemetry Collector config translator                                                                                                                                                       | [otel/conf.go](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/otel/conf.go) (implementation) and [otel/conf_test.go](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/otel/conf_test.go) (unit tests)
Fluent Bit config translator                                                                                                                                                                     | [fluentbit/conf/conf.go](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/fluentbit/conf/conf.go) (implementation) and [fluentbit/conf/conf_test.go](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/fluentbit/conf/conf_test.go) (unit tests)

</details>

<details>
<summary>Systemd related</summary>

Description                                                               | Link
------------------------------------------------------------------------- | ----
Configs for the `google-cloud-ops-agent` service                          | [systemd/google-cloud-ops-agent.service](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/systemd/google-cloud-ops-agent.service)
Configs for the `google-cloud-ops-agent-open-telemetry-collector` service | [systemd/google-cloud-ops-agent-opentelemetry-collector.service](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/systemd/google-cloud-ops-agent-opentelemetry-collector.service)
Configs for the `google-cloud-ops-agent-fluent-bit` service               | [systemd/google-cloud-ops-agent-fluent-bit.service](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/systemd/google-cloud-ops-agent-fluent-bit.service)

</details>

<details>
<summary>Build related</summary>

Description                                                        | Link
------------------------------------------------------------------ | ----
Build container Dockerfile for Linux                               | [Dockerfile](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/Dockerfile)
Build container Dockerfile for Windows                             | [Dockerfile.windows](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/Dockerfile.windows)
Build script for a tarball                                         | [build.sh](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/build.sh)
Build script of the Deb packages for Debian and Ubuntu distros     | [pkg/deb/build.sh](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/pkg/deb/build.sh)
Build script of the RPM packages for CentOS, RHEL and Sles distros | [pkg/rpm/build.sh](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/pkg/rpm/build.sh)
Build script of the Windows packages for Windows distros           | [pkg/goo/build.ps1](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/pkg/goo/build.ps1)

</details>

## Check out code

Check out the code from the repo:

```shell
$ git clone --recurse-submodules git@github.com:GoogleCloudPlatform/ops-agent.git
$ cd ops-agent
```

Tip: After you make changes to the code base and run unit tests, in case the
golden files are updated, the following command can be used to see the diff
easily that excludes all golden files. This is helpful if you are doing PR code
reviews by checking out the code locally too, because GitHub does not have a
good way to easily see all the files that are changed.

```shell
$ git diff origin/master -- . ':!confgenerator/testdata'
```

## Run goimports

Run `goimports` to keep the codebase format style in sync:

```shell
ops-agent$ goimports -w .
```

## Test locally

### Run unit tests

#### Run all unit tests

```shell
ops-agent$ go test -mod=mod ./...
```

#### Only run unified agent config generator tests

```shell
ops-agent$ go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator
```

In case the failed diff is indeed expected, you can bulk update the golden
Fluent Bit and Open Telemetry Collector conf files in the
`confgenerator/testdata/valid` folder by:

```shell
ops-agent$ go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator -update_golden
```

Add "-v" to show details for which files are updated with what:

```shell
ops-agent$ go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator -update_golden -v
```

*   Understanding the config generator tests

    In each
    [test data](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/confgenerator/testdata/),
    `input.yaml` is the input, which is the Ops Agent YAML config file.
    `*_fluent_bit_*.conf` are the generated Fluent Bit configuration files.

    [public documentation](https://cloud.google.com/stackdriver/docs/solutions/ops-agent/configuration)
    that will have the official config syntax.

    The
    [config generator](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/confgenerator/confgenerator.go)
    uses https://github.com/go-yaml/yaml for basic YAML validations. It uses
    https://golang.org/pkg/text/template/ for templating the configs.

#### Only run Fluent Bit config translator tests

```shell
ops-agent$ go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/fluentbit/conf
```

#### Only run Open Telemetry Collector config translator tests

```shell
ops-agent$ go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/otel
```

### Test config generation manually

Config generation can be triggered manually via the `generate_config` command
for development.

```shell
# Ops Agent config path
$ export CONFIG_IN=confgenerator/default-config.yaml

# Output directory to write configs to
$ export CONFIG_OUT=/tmp/google-cloud-ops-agent/conf

$ mkdir -p $CONFIG_OUT
ops-agent$ go run -mod=mod cmd/google_cloud_ops_agent_engine/main.go \
  --service=fluentbit \
  --in=$CONFIG_IN \
  --out=$CONFIG_OUT
ops-agent$ go run -mod=mod cmd/google_cloud_ops_agent_engine/main.go \
  --service=otel \
  --in=$CONFIG_IN \
  --out=$CONFIG_OUT
$ tree $CONFIG_OUT
```

*   Sample generated
    [golden fluent bit main conf](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/confgenerator/testdata/valid/linux/default_config/golden_fluent_bit_main.conf)
    at `$CONFIG_OUT/fluent_bit_main.conf`.
*   Sample generated
    [golden fluent bit parser conf](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/confgenerator/testdata/valid/linux/default_config/golden_fluent_bit_parser.conf)
    at `$CONFIG_OUT/fluent_bit_parser.conf`.
*   Sample generated
    [golden otel conf](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/confgenerator/testdata/valid/linux/default_config/golden_otel.conf)
    at `$CONFIG_OUT/otel.conf`.

## Build and test manually on GCE VMs

### Linux

#### Build packages

```shell
# Output directory to write packages to.
$ export PACKAGES_OUT=/tmp/google-cloud-ops-agent

ops-agent$ DOCKER_BUILDKIT=1 docker build -o $PACKAGES_OUT .
$ ls $PACKAGES_OUT
```

<details>
<summary>You may need to update the submodules.</summary>

```shell
ops-agent$ git submodule update --init
```

Sometimes you can hit build failures if you forget to also update the submodules
when new changes are needed. This command should update the local submodules and
sync them with upstream.
</details>

This will leave you with a tarball and a package for each distro that contains
`fluent-bit`, `open-telemetry-collector`, `generate_config`, and systemd units.
Sample output:

```
total 180116
-rw-r--r-- 1 421646 89939  5007888 Oct  7 21:16 google-cloud-ops-agent-0.1.0-1.el7.x86_64.rpm
-rw-r--r-- 1 421646 89939  5144676 Oct  7 21:16 google-cloud-ops-agent-0.1.0-1.el8.x86_64.rpm
-rw-r--r-- 1 421646 89939  4919155 Oct  7 21:16 google-cloud-ops-agent-0.1.0-1.sles12.x86_64.rpm
-rw-r--r-- 1 421646 89939  4924528 Oct  7 21:16 google-cloud-ops-agent-0.1.0-1.sles15.x86_64.rpm
-rw-r--r-- 1 421646 89939  2915772 Oct  7 21:18 google-cloud-ops-agent_0.1.0~debian10_amd64.deb
-rw-r--r-- 1 421646 89939  2919650 Oct  7 21:18 google-cloud-ops-agent_0.1.0~debian9.13_amd64.deb
-rw-r--r-- 1 421646 89939  2914160 Oct  7 21:18 google-cloud-ops-agent_0.1.0~ubuntu18.04_amd64.deb
-rw-r--r-- 1 421646 89939  2944116 Oct  7 21:18 google-cloud-ops-agent_0.1.0~ubuntu20.04_amd64.deb
-rw-r--r-- 1 421646 89939  7561251 Oct  7 21:16 google-cloud-ops-agent-centos-7.tgz
-rw-r--r-- 1 421646 89939  7726721 Oct  7 21:16 google-cloud-ops-agent-centos-8.tgz
-rw-r--r-- 1 421646 89939 10560080 Oct  7 21:18 google-cloud-ops-agent-dbgsym_0.1.0~debian10_amd64.deb
-rw-r--r-- 1 421646 89939  8643334 Oct  7 21:18 google-cloud-ops-agent-dbgsym_0.1.0~debian9.13_amd64.deb
-rw-r--r-- 1 421646 89939 16809738 Oct  7 21:18 google-cloud-ops-agent-debian-buster.tgz
-rw-r--r-- 1 421646 89939 14837062 Oct  7 21:18 google-cloud-ops-agent-debian-stretch.tgz
-rw-r--r-- 1 421646 89939  7697140 Oct  7 21:15 google-cloud-ops-agent-sles-12.tgz
-rw-r--r-- 1 421646 89939  7718344 Oct  7 21:16 google-cloud-ops-agent-sles-15.tgz
-rw-r--r-- 1 421646 89939 15550083 Oct  7 21:18 google-cloud-ops-agent-ubuntu-bionic.tgz
-rw-r--r-- 1 421646 89939 17526671 Oct  7 21:18 google-cloud-ops-agent-ubuntu-focal.tgz
```

Inspect the tarball to see if there is anything obviously wrong.

```shell
$ tar -tvf $PACKAGES_OUT/google-cloud-ops-agent-debian-buster.tgz
```

#### Create a VM

See [Create a GCE Linux VM](create-gce-linux-vm.md).

#### Install the package on a VM

##### Using a package built via the previous step

NOTE: The firewall rule creation can be skipped if your project is exempted.

```shell
# Name of the package from the `$PACKAGES_OUT` folder. The packages you built
# following instructions in the `Build the package` section should be here
# already.
$ export PACKAGE_NAME=google-cloud-ops-agent_0.1.0~debian10_amd64.deb

$ gcloud compute firewall-rules create allow-ssh \
    --allow tcp:22 --project $VM_PROJECT_ID
$ gcloud compute scp \
    --zone $VM_ZONE \
    --project $VM_PROJECT_ID \
    $PACKAGES_OUT/$PACKAGE_NAME \
    $VM_NAME:/tmp/$PACKAGE_NAME
$ gcloud compute ssh \
    --zone $VM_ZONE \
    --project $VM_PROJECT_ID \
    $VM_NAME
```

**The remainder of the commands are run on a VM**

#### Debian / Ubuntu

```shell
$ sudo apt install -y /tmp/$PACKAGE_NAME
```

#### CentOS / RHEL

```shell
$ sudo yum install -y /tmp/$PACKAGE_NAME
```

#### SLES

```shell
$ sudo zypper install -n /tmp/$PACKAGE_NAME
```

### Windows

#### Create a Windows build VM

See [Create a GCE Windows build VM](create-gce-windows-build-vm.md).

#### Connect to Windows build VM

See [Connect to a Windows VM](connect-to-windows-vm.md).

#### One-time setup on Windows

See [Set up a Windows VM](set-up-windows-vm.md).

<details>
<summary>You also need to install Git.</summary>

1.  Run PowerShell as Administrator.

    From the start menu, find `Windows PowerShell`, right-click, and select `Run
    as administrator`.

1.  Download Git installer and install Git on the VM.

    Switch the folder first.

    ```
    cd $env:UserProfile
    ```

    Tip: Check https://github.com/git-for-windows/git/releases for all versions.

    ```
    # [Optional] Adjust the Git version to install.
    # $GIT_VERSION=2.30.0

    wget -o git.exe https://github.com/git-for-windows/git/releases/download/v$GIT_VERSION.windows.1/Git-$GIT_VERSION-64-bit.exe
    start git.exe
    ```

    Restart the `Windows PowerShell` after installation.

</details>

#### Trigger the build

Note: Make sure you have finished the 3 steps above including some one-time
setup before proceeding. The [one-time setup](#one-time-setup-on-windows) helps
speed up the build.

1.  Run PowerShell as Administrator.

1.  Check out Ops Agent repo, run Docker build, and copy the binary from within
    the Docker container to the Windows VM.

    Note: The build can take +50 minutes for the first run. Find something else
    to play with then come back later. The following runs should use cached
    layers so it should be faster.

    Note: Due to some issue with the RDP client, sometimes the `Windows
    PowerShell` windows may be frozen. In other words, the build may appear to
    be stuck when it's actually building. The solution is to reconnect RDP.

    **First time building**

    ```
    # [Optional] Adjust the branch or commit hash to check out the code from.
    # $WINDOWS_BUILD_BRANCH=master

    cd $env:UserProfile
    git clone https://github.com/GoogleCloudPlatform/ops-agent.git
    cd ops-agent
    git checkout $WINDOWS_BUILD_BRANCH
    git submodule update --init
    docker build -t full-build -f .\Dockerfile.windows .

    cd $env:UserProfile
    mkdir tmp
    docker create --name build-image full-build
    docker cp build-image:/work/out ./tmp
    docker rm build-image
    dir -Recurse ./tmp/out
    ```

    The last line of the `docker build` comamnd output should be:

    ```
    Successfully tagged full-build:latest
    ```

    The output of the `dir` command should list the following 3 exe files in the
    `C:\Users\${USER}\ops-agent\tmp\out\bin` directory:

    *   `fluent-bit.exe`
    *   `google-cloud-metrics-agent_windows_amd64.exe`
    *   `google-cloud-ops-agent.exe`

    <details>
    <summary>Sample output:</summary>

    ```
        Directory: C:\Users\{{USERNAME}}\tmp\out

    Mode                LastWriteTime         Length Name
    ----                -------------         ------ ----
    d-----         2/6/2021   7:08 AM                bin
    d-----         2/6/2021   7:08 AM                config
    -a----         2/6/2021   7:08 AM       44653740 google-cloud-ops-agent.x86_64.1.0.1@1.goo

        Directory: C:\Users\{{USERNAME}}\tmp\out\bin

    Mode                LastWriteTime         Length Name
    ----                -------------         ------ ----
    -a----         2/6/2021   6:59 AM        3717120 fluent-bit.dll
    -a----         2/6/2021   6:59 AM        4494336 fluent-bit.exe
    -a----         2/6/2021   7:03 AM       99622912 google-cloud-metrics-agent_windows_amd64.exe
    -a----         2/6/2021   7:08 AM        4888064 google-cloud-ops-agent.exe
    -a----       11/11/2020  12:27 PM         585096 msvcp140.dll
    -a----       11/11/2020  12:27 PM         330120 vccorlib140.dll
    -a----       11/11/2020  12:27 PM          94088 vcruntime140.dll

        Directory: C:\Users\{{USERNAME}}\tmp\out\config

    Mode                LastWriteTime         Length Name
    ----                -------------         ------ ----
    -a----         2/3/2021   9:30 PM           1265 config.yaml
    ```

    </details>

    <details>
    <summary>Retries</summary>

    If it fails, fix the issue then clean up and retry building:

    Warning: The cleanup will wipe the `C:\Users\{{USERNAME}}\tmp` directory.

    ```
    Stop-Service google-cloud-ops-agent -Force
    rm -r "C:\Users\{{USERNAME}}\tmp"
    rm -r "C:\Users\{{USERNAME}}\ops-agent\submodules\fluent-bit\build"

    cd $env:UserProfile
    cd ops-agent
    docker build -t full-build -f .\Dockerfile.windows .

    docker rm build-image
    docker create --name build-image full-build
    docker cp build-image:/work/out ./tmp
    docker rm build-image
    dir -Recurse ./tmp/out
    ```

    </details>

    <details>
    <summary>Reset build VM then rebuild</summary>

    If you want to reset the VM (aka clean up any existing code / artifacts and
    start from scratch), run the following for cleanup, then go back to the
    `First time building` section above:

    ```
    Stop-Service google-cloud-ops-agent -Force
    Remove-Item -Force -Recurse -Path "C:\Users\{{USERNAME}}\ops-agent\*"
    Remove-Item -Force -Recurse -Path "C:\Users\{{USERNAME}}\tmp\*"
    rm -r "C:\Users\{{USERNAME}}\tmp"
    rm -r "C:\Users\{{USERNAME}}\ops-agent"
    docker rm build-image
    ```

    </details>

#### Upload package to GCS from Windows build VM

To get a package out of a Windows VM, GCS is one of the few available options.

**Upload artifacts**

In `Windows PowerShell`, use gsutil to upload the artifacts to GCS.

```
# [Optional] Adjust the directory you want to upload to GCS.
# $LOCAL_ARTIFACTS_DIR=C:\Users\{{USERNAME}}\tmp\out

# [Optional] Adjust the destination GCS bucket dir.
# $GCP_BUCKET_DIR=ops_agent_windows

gsutil cp -r $LOCAL_ARTIFACTS_DIR\* gs://us.artifacts.$VM_PROJECT_ID.appspot.com/$GCP_BUCKET_DIR
```

To overwrite existing contents in the bucket:

```
gsutil rm -r gs://us.artifacts.$VM_PROJECT_ID.appspot.com/$GCP_BUCKET_DIR
gsutil cp -r $LOCAL_ARTIFACTS_DIR\* gs://us.artifacts.$VM_PROJECT_ID.appspot.com/$GCP_BUCKET_DIR
```

<details>
<summary>Troubleshooting 403 errors</summary>

In case your VM was not initially created with sufficient scopes to access GCS,
you need to:

1.  From your **workstation**, grant the Windows build VM sufficient access
    scope to read and write GCS.

    ```shell
    # The VM name to grant GCS access to.
    # $WIN_BUILD_VM_NAME={{USERNAME}}-win-build-vm

    $ gcloud compute instances stop \
        --zone $VM_ZONE --project $VM_PROJECT_ID \
        $WIN_BUILD_VM_NAME

    $ gcloud beta compute instances set-scopes \
        --zone $VM_ZONE --project $VM_PROJECT_ID \
        --scopes storage-rw \
        $WIN_BUILD_VM_NAME

    $ gcloud compute instances start \
        --zone $VM_ZONE --project $VM_PROJECT_ID \
        $WIN_BUILD_VM_NAME
    ```

2.  [Connect](#connect-to-windows-build-vm) to the Windows VM, open a `Windows
    PowerShell`, delete the cached credential, and redo gcloud login:

    ```
    rd -r $env:USERPROFILE\.gsutil
    ```

</details>

#### Download package from GCS to Linux workstation

```shell
# [Optional] Adjust the source GCS bucket dir from where to download the
# artifacts.
$ export GCP_BUCKET_DIR=ops_agent_windows

# [Optional] Adjust the local directory to where you want to store the
# artifacts.
$ export WINDOWS_RELEASE_DIR=/tmp/google-cloud-ops-agent/windows-release

$ rm -rf $WINDOWS_RELEASE_DIR/$GCP_BUCKET_DIR
$ mkdir -p $WINDOWS_RELEASE_DIR/$GCP_BUCKET_DIR
$ gsutil cp -r gs://us.artifacts.$VM_PROJECT_ID.appspot.com/$GCP_BUCKET_DIR/* $WINDOWS_RELEASE_DIR/$GCP_BUCKET_DIR
$ ls $WINDOWS_RELEASE_DIR/$GCP_BUCKET_DIR/google-cloud-ops-agent.x86_64.*.goo
```

#### Create a test VM

See [Create a GCE Windows test VM](create-gce-windows-test-vm.md).

#### Install the agent on the test VM

##### Using a local package

1.  Start a `Windows PowerShell` in admin mode by right-clicking it (use this
    for commands below)

1.  [Optional] Download the `google-cloud-ops-agent.exe` binary you want to
    test.

    Note: If you are testing the binary directly on the build VM, you can skip
    this step. There is some downside of doing so because the build VM is
    optimized for container usage (e.g. Windows Event Logs from `Application`
    channel do not work.  But it's
    sufficient for a quick sanity check. If you are testing the binary on a
    separate Windows VM, continue.

    Download the artifacts from the GCS bucket you uploaded the artifacts to in
    the previous step.


    ```
    # [Optional] Adjust the source GCS bucket dir from where to download the
    # artifacts. This directory should have `bin` and `config` sub directory in
    # it.
    # $GCP_BUCKET_DIR=ops_agent_windows

    # [Optional] Adjust the local directory to where you want to store the
    # artifacts.
    # $LOCAL_ARTIFACTS_DIR=C:\Users\{{USERNAME}}\tmp\out

    md $LOCAL_ARTIFACTS_DIR
    gsutil cp -r gs://us.artifacts.$VM_PROJECT_ID.appspot.com/$GCP_BUCKET_DIR/* $LOCAL_ARTIFACTS_DIR
    ls $LOCAL_ARTIFACTS_DIR
    ```

    To overwrite existing contents in the local directory:

    ```
    Stop-Service google-cloud-ops-agent -Force
    rd -r "$LOCAL_ARTIFACTS_DIR"
    md $LOCAL_ARTIFACTS_DIR
    gsutil cp -r gs://us.artifacts.$VM_PROJECT_ID.appspot.com/$GCP_BUCKET_DIR/* $LOCAL_ARTIFACTS_DIR
    ls $LOCAL_ARTIFACTS_DIR
    ```

1.  Install the `google-cloud-ops-agent` Googet package.

    ```
    # Version of the package.
    # $WINDOWS_PACKAGE_VERSION=1.0.1@1

    googet -noconfirm install $LOCAL_ARTIFACTS_DIR\google-cloud-ops-agent.x86_64.$WINDOWS_PACKAGE_VERSION.goo
    ```

    Expected output:

    ```
    PS C:\Users\lingshi\tmp\out> googet -noconfirm install C:\Users\lingshi\tmp\out\google-cloud-ops-agent.x86_64.$WINDOWS_PACKAGE_VERSION.goo
    Installing google-cloud-ops-agent $WINDOWS_PACKAGE_VERSION...
    2021/02/09 04:57:17 installed services
    Installation of google-cloud-ops-agent completed
    ```

    **Retries**

    In case of retries, clean up the existing services and data folders first:

    ```
    sc delete google-cloud-ops-agent-fluent-bit
    sc delete google-cloud-ops-agent-otel
    sc delete google-cloud-ops-agent
    rd -r "C:\ProgramData\Google\Cloud Operations\Ops Agent"
    rd -r "C:\Program Files\Google\Cloud Operations\Ops Agent"
    ```

1.  Verify that the service is indeed running.

    ```
    Get-Service google-cloud-ops-agent*
    ```

    <details>
    <summary>Expected output:</summary>

    ```
    Status   Name                               DisplayName
    ------   ----                               -----------
    Running  google-cloud-ops-agent             Google Cloud Ops Agent
    Running  google-cloud-ops-agent-fluent-bit  Google Cloud Ops Agent - Logging Agent
    Running  google-cloud-ops-agent-otel        Google Cloud Ops Agent - Metrics Agent
    ```

    </details>

**Troubleshooting**

-   Check logs in `Event Viewer` app
-   Check service status and start / stop them in `Services` app
-   Inspect running processes in `Task Manager` app
-   Run Fluent Bit individually:

    Temporary step: Manually create a `C:\ProgramData\Google\Cloud
    Operations\Ops Agent\run\buffers` folder. `C:\ProgramData` is by default
    hidden, you need to show it.

    ![image](https://user-images.githubusercontent.com/5287526/133006710-2ab21ef6-22e7-43ff-826d-15e82a2d3e08.png)


    ```
    C:\Users\{{USERNAME}}\tmp\out\bin\fluent-bit.exe --config="C:\ProgramData\Google\Cloud Operations\Ops Agent\generated_configs\fluentbit\fluent_bit_main.conf" --parser="C:\ProgramData\Google\Cloud Operations\Ops Agent\generated_configs\fluentbit\fluent_bit_parser.conf" --storage_path="C:\ProgramData\Google\Cloud Operations\Ops Agent\run\buffers"
    ```

-   Run Open Telemetry Metrics Agent individually:

    ```
    C:\Users\{{USERNAME}}\tmp\out\bin\google-cloud-metrics-agent_windows_amd64.exe "--config=C:\ProgramData\Google\Cloud Operations\Ops Agent\generated_configs\otel\otel.yaml"
    ```

#### Uninstall the agent

##### Linux

1.  Stop the agent:

    ```shell
    sudo service google-cloud-ops-agent stop
    ```

1.  Uninstall the package:

    ```shell
    sudo apt remove google-cloud-ops-agent
    ```

##### Windows

1.  Run the following commands in Windows PowerShell (admin mode):

    ```
    googet -noconfirm remove google-cloud-ops-agent
    ```

#### [Optional] Edit config

Edit the Ops Agent configuration file as needed:

##### Linux

```shell
$ sudo vim /etc/google-cloud-ops-agent/config.yaml
```

Sample
[config](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/confgenerator/default-config.yaml)

###### Windows

```
notepad "C:\Program Files\Google\Cloud Operations\Ops Agent\config\config.yaml"
```

Sample
[config](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/confgenerator/windows-default-config.yaml)

Restart the agent to apply the new config.

##### Linux

```shell
$ sudo service google-cloud-ops-agent restart
```

#### Windows

Open `Windows PowerShell` as administrator and run:

```
Restart-Service google-cloud-ops-agent -Force
```

#### Send syslog via TCP (Linux)

Follow the step above to edit the config file to include syslog tcp config as
below:

```
logging:
  receivers:
    syslog_tcp:
      type: syslog
      transport_protocol: tcp
      listen_host: 0.0.0.0
      listen_port: 5140
  exporters:
    google:
      type: google_cloud_logging
  service:
    pipelines:
      my_pipeline:
        receivers:
        - syslog_tcp
        exporters:
        - google
```

Send a log via tcp

```shell
# The log to send via tcp.
$ export TCP_LOG=${USER}-test-tcp

$ logger -n 0.0.0.0 --tcp --port=5140 -- $TCP_LOG
```

#### Uninstall the agent

##### Linux

1.  Stop the agent:

    ```shell
    sudo systemctl stop google-cloud-ops-agent.target
    ```

1.  Uninstall the package:

    ```shell
    sudo apt remove google-cloud-ops-agent
    ```

##### Windows

1.  Run the following commands in Windows PowerShell (admin mode):

    ```
    googet -noconfirm remove google-cloud-ops-agent
    ```

## Verify logs and metrics ingestion

### Logs

Set `$LOG_NAME` to
the log name to query. With the default config, the agent collects logs from
`/var/log/syslog` and `/var/log/message` files for Linux VMs and Windows Event
Logs for Windows VMs, and ingest them with the log name `syslog` for Linux VMs
and `winevt.raw` for Windows VMs. Adjust this if you are looking for some custom
logs. The log name should map to `log_source_id` in the configuration.

*   Using Log Viewer

    Go to
    [Log Viewer](https://console.cloud.google.com/logs/query?project=$VM_PROJECT_ID)
    and use the following query in the Query Builder:

    TODO(lingshi): Add instance name filter after bug is fixed. aka
    append `labels."compute.googleapis.com/resource_name"="$VM_NAME"` to the
    query below.

    ```
    logName="projects/$VM_PROJECT_ID/logs/syslog"
    resource.type="gce_instance"
    ```

    There should be some logs from the VM.

*   Using gcloud

    ```shell
    $ VM_ID=$(gcloud compute instances describe \
        --zone $VM_ZONE \
        --project $VM_PROJECT_ID \
        --format="value(id)" \
        $VM_NAME)
    $ gcloud logging read \
        --project $VM_PROJECT_ID \
        "logName=projects/$VM_PROJECT_ID/logs/$LOG_NAME \
        AND resource.type=gce_instance \
        AND resource.labels.instance_id=$VM_ID" --limit 1
    ```

### Metrics

Go to
[Metrics Explorer](https://console.cloud.google.com/monitoring/metrics-explorer)
and use the following query in the Query Editor to figure out whether the agent
is up running and ingesting metrics:

```
fetch gce_instance
| metric 'agent.googleapis.com/agent/uptime'
| filter (metric.version =~ 'google-cloud-ops-agent.*')
| align rate(1m)
| every 1m
| group_by
    [resource.project_id, resource.zone,
     metadata.system.name: metadata.system_labels.name, metric.version],
    [value_uptime_aggregate: aggregate(value.uptime)]
```


In case of specific metrics, something like below looks for
`agent.googleapis.com/agent/memory_usage` metrics:

```
fetch gce_instance
| metric 'agent.googleapis.com/agent/memory_usage'
| group_by 1m, [value_memory_usage_mean: mean(value.memory_usage)]
| every 1m
| group_by [metadata.system.name: metadata.system_labels.name],
    [value_memory_usage_mean_aggregate: aggregate(value_memory_usage_mean)]
```

## Updating a submodule

1.  Make sure your local repo is up-to-date:

    ##### Existing local repo

    ```shell
    ops-agent$ git checkout master && git pull
    ops-agent$ git submodule update --init --recursive
    ```

    ###### Create new local clone

    If you haven't cloned the repo, do so with the `--recurse-submodules` flag:

    ```shell
    $ TMP_DIR=$(mktemp -d)
    $ cd $TMP_DIR
    $ git clone --recurse-submodules git@github.com:GoogleCloudPlatform/ops-agent.git
    $ cd ops-agent
    ```

1.  To update submodule

    Set `$SUBMODULE_NAME` to one of:
    
    - fluent-bit
    - opentelemetry-operations-collector
    - collectd

    Set `$SUBMODULE_TAG` to commit/tag, e.g. `v1.8.5`. See tags for
    [`fluent bit`](https://github.com/fluent/fluent-bit/tags),
    [`otel collector`](https://github.com/GoogleCloudPlatform/opentelemetry-operations-collector/tags),
    or [`collectd`](https://github.com/Stackdriver/collectd/tags)
    
    Run:

    ```shell
    ops-agent$ (cd submodules/$SUBMODULE_NAME && git fetch --tags && git checkout $SUBMODULE_TAG_OR_COMMIT)
    ops-agent$ git add submodules/$SUBMODULE_NAME
    ops-agent$ git commit -m "Update $SUBMODULE_NAME submodule to $SUBMODULE_TAG_OR_COMMIT."
    ops-agent$ git checkout -b ${USER}-update-$SUBMODULE_NAME
    ops-agent$ git push origin ${USER}-update-$SUBMODULE_NAME
    ```

## License and Copyright

Automatically recognizes the source files that need a license header and
recursively traverses all files to add a license header:

```shell
$ go get -u github.com/google/addlicense
ops-agent$ addlicense -c "Google LLC" -l apache .
```

Find licensing bits that might not be right, or files with no licenses, or code
not copyright Google that should be located in `third_party/`:

```shell
ops-agent$ /google/bin/releases/opensource/thirdparty/cross/cross .
```
## PUBLIC: Troubleshooting

### Where to find things

#### Linux

Description                                                                                          | Link
---------------------------------------------------------------------------------------------------- | ----
Config file - Ops Agent                                                                              | `/etc/google-cloud-ops-agent/config.yaml` (custom agent configurations)
Logs - Fluent Bit                                                                                    | `/var/log/google-cloud-ops-agent/subagents/logging-module.log`
Logs - OT Metrics Agent                                                                              | `/var/log/syslog` or `/var/log/messages`. This agent does not write logs directly to disk today.
Config file - Ops Agent (Debug)                                                                      | `/run/google-cloud-ops-agent-fluent-bit/conf/debug/` or `/run/google-cloud-ops-agent-opentelemetry-collector/conf/debug/`
Config file - Fluent Bit - Main config. Only present when the systemd fluent-bit unit is running.    | `/var/run/google-cloud-ops-agent-fluent-bit/fluent_bit_main.conf`
Config file - Fluent Bit - Parsers config. Only present when the systemd fluent-bit unit is running. | `/var/run/google-cloud-ops-agent-fluent-bit/fluent_bit_parser.conf`
Config file - OT Metrics Agent                                                                       | `/var/run/google-cloud-ops-agent-opentelemetry-collector/otel.yaml`
Buffer files - Fluent Bit                                                                            | `/var/lib/google-cloud-ops-agent/fluent-bit/buffers/`

#### Windows

Description                               | Link
----------------------------------------- | ----
Config file - Ops Agent                   | `C:\Program Files\Google\Cloud Operations\Ops Agent\config\config.yaml`
Logs - Fluent Bit                         | `C:\ProgramData\Google\Cloud Operations\Ops Agent\log\logging-module.log`
Logs - OT Metrics Agent                   | In the `Event Viewer` app, click `Windows Logs` then `Application`, filter by `Source` equal to `Google Cloud Ops Agent - Metrics Agent`. This agent does not write logs directly to disk today.
Config file - Fluent Bit - Main config    | `C:\ProgramData\Google\Cloud Operations\Ops Agent\generated_configs\fluentbit\fluent_bit_main.conf`
Config file - Fluent Bit - Parsers config | `C:\ProgramData\Google\Cloud Operations\Ops Agent\generated_configs\fluentbit\fluent_bit_parser.conf`
Config file - OpenTelemetry               | `C:\ProgramData\Google\Cloud Operations\Ops Agent\generated_configs\otel\otel.yaml`
Buffer files - Fluent Bit                 | `C:\ProgramData\Google\Cloud Operations\Ops Agent\run\buffers`

### Check logs

#### Linux

*   Check all related syslog

    ```shell
    $ sudo grep -r " google_cloud_ops_agent_engine\| otelopscol\| systemd\| fluent-bit" /var/log/syslog
    $ sudo grep -r " google_cloud_ops_agent_engine\| otelopscol\| systemd\| fluent-bit" /var/log/messages
    ```

*   Check Fluent Bit related logs

    ```shell
    $ sudo grep -r " fluent-bit" /var/log/syslog
    $ sudo grep -r " fluent-bit" /var/log/messages
    $ journalctl -u google-cloud-ops-agent-fluent-bit
    $ cat /var/log/google-cloud-ops-agent/subagents/logging-module.log
    ```

*   Check Open Telemetry Collector related logs

    ```shell
    $ sudo grep -r " otelopscol" /var/log/syslog
    $ sudo grep -r " otelopscol" /var/log/messages
    $ journalctl -u google-cloud-ops-agent-opentelemetry-collector
    $ cat /var/log/google-cloud-ops-agent/subagents/metrics-module.log
    ```

*   Check Systemd related syslog

    ```shell
    $ sudo grep -r " systemd" /var/log/syslog
    $ sudo grep -r " systemd" /var/log/messages
    ```

#### Windows

The OT Collector subagent logs can be found locally in `Event Viewer`. From
`Start`, open the `Event Viewer` app, click `Windows Logs` then `Application`,
filter by `Source` equal to `Google Cloud Ops Agent - Metrics Agent`. This agent
does not write logs directly to disk today.

The Fluent Bit subagent writes logs to this local file:

```
notepad "C:\ProgramData\Google\Cloud Operations\Ops Agent\log\logging-module.log"
```

### Verify Ops Agent and subagent status

#### Linux

```shell
$ sudo systemctl status google-cloud-ops-agent"*"
```

#### Windows

Open `Windows PowerShell` as administrator and run:

```
Get-Service google-cloud-ops-agent
```

You can also check service status in the `Services` app and inspect running
processes in the `Task Manager` app.

### Restart Ops Agent and subagents

#### Linux

```shell
$ sudo service google-cloud-ops-agent restart
```

```shell
$ sudo systemctl start google-cloud-ops-agent.target
```

```shell
$ sudo systemctl stop google-cloud-ops-agent.target
```

#### Windows

Open `Windows PowerShell` as administrator and run:

```
Restart-Service google-cloud-ops-agent -Force
```

```
Start-Service google-cloud-ops-agent* -Force
```

```
Stop-Service google-cloud-ops-agent* -Force
```

You can also start / stop services in the `Services` app.

### Run Fluent Bit individually on Windows

Temporary step: Manually create a `C:\ProgramData\Google\Cloud Operations\Ops
Agent\run\buffers` folder. `C:\ProgramData` is by default hidden, you need to
show it.

```
'C:\Program Files\Google\Cloud Operations\Ops Agent\bin\fluent-bit.exe' --config="C:\ProgramData\Google\Cloud Operations\Ops Agent\generated_configs\fluentbit\fluent_bit_main.conf" --parser="C:\ProgramData\Google\Cloud Operations\Ops Agent\generated_configs\fluentbit\fluent_bit_parser.conf" --storage_path="C:\ProgramData\Google\Cloud Operations\Ops Agent\run\buffers"
```

### Run Open Telemetry Metrics Agent individually on Windows

```
'C:\Program Files\Google\Cloud Operations\Ops Agent\bin\google-cloud-metrics-agent_windows_amd64.exe' "--config=C:\ProgramData\Google\Cloud Operations\Ops Agent\generated_configs\otel\otel.yaml"
```

### Known issues

#### Docker image failed to build

You may see errors like:

```
failed to solve with frontend dockerfile.v0: failed to build LLB: failed to load cache key: docker.io/library/debian:stretch not found
```

To fix this, run something like below (change the path as needed):

```shell
$ docker pull docker.io/library/debian:stretch
```

## References

### Fluent Bit

*   Windows build
    [documentation](https://github.com/fluent/fluent-bit-docs/blob/master/installation/windows.md)
