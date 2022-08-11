# Set up a Windows VM

## Firewalls and Antivirus

Doing the follow things makes your life easier with Windows development (e.g.
faster build time; freedom to download installers from the Internet.)

*   Disable enhanced security

*   Turn off the Antivirus process.

*   Turn off all the firewalls.

The easiest way to do this is via the `Server Manager`.

![image](https://user-images.githubusercontent.com/5287526/133006540-94e2170f-8366-40c1-8280-7bb92e3f4378.png)

## Install Git

Install Git if you would like to clone the repo and build. 

1.  Run PowerShell as Administrator.

    From the start menu, find `Windows PowerShell`, right-click, and select `Run
    as administrator`.

1.  Download Git installer and install Git on the VM.

    Switch the folder first.

    ```powershell
    cd $env:UserProfile
    ```

    Tip: Check https://github.com/git-for-windows/git/releases for all versions.

    ```powershell
    # [Optional] Adjust the Git version to install.
    $env:GIT_VERSION='2.37.0'
    wget -o git.exe https://github.com/git-for-windows/git/releases/download/v$env:GIT_VERSION.windows.1/Git-$env:GIT_VERSION-64-bit.exe
    start .\git.exe
    ```

    Restart the `Windows PowerShell` after installation.


## Install Docker 

Docker is preinstalled on GCE container images such as `Windows Server 2019 for Containers`. 

Check if Docker is installed and running:

1. Run PowerShell as Administrator, and execute
    ```powershell
    docker ps
    ```
2. The command should run without error and show an empty list:
    ```powershell
    PS C:\Windows\system32> docker ps
    CONTAINER ID        IMAGE               COMMAND             CREATED             STATUS              PORTS               NAMES
    ```

For GCE images not built for containers, install Docker following steps [here](https://docs.microsoft.com/en-us/virtualization/windowscontainers/quick-start/set-up-environment?tabs=dockerce#windows-server-1). 
