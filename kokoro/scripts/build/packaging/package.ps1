$ErrorActionPreference = "Stop"
$global:ProgressPreference = 'SilentlyContinue'

$GoVersion = "1.24.11"
$GoInstallerUrl = "https://go.dev/dl/go$GoVersion.windows-amd64.msi"
$GoInstallDir = "C:\Go"

function Install-Go {
    Write-Host "Downloading and Installing Go..."

    $DownloadDir = "C:\local"
    $MsiPath = Join-Path $DownloadDir "go_installer.msi"

    # Ensure the local directory exists
    if (-not (Test-Path $DownloadDir)) {
        New-Item -Path $DownloadDir -ItemType Directory -Force | Out-Null
    }

    # Download the MSI
    Write-Host "Downloading Go $GoVersion from $GoInstallerUrl..."
    Invoke-WebRequest -Uri $GoInstallerUrl -OutFile $MsiPath

    # Install the MSI
    Write-Host "Running MSI Installer..."
    Start-Process msiexec.exe `
        -ArgumentList "/i $MsiPath /quiet /norestart ALLUSERS=1 INSTALLDIR=$GoInstallDir" `
        -NoNewWindow -Wait

    # Add Go to the path for the current session
    $env:Path = "$env:Path;$GoInstallDir\bin"

    # Verify installation
    $InstalledVersion = go version
    Write-Host "Go installed successfully: $InstalledVersion"
}

function Invoke-PackageBuild {
    param(
        [Parameter(Mandatory=$true)][string]$Arch,
        [Parameter(Mandatory=$true)][string]$OutputDir,
        [Parameter(Mandatory=$true)][string]$InputDir
    )

    if ([string]::IsNullOrWhiteSpace($Arch) -or `
        [string]::IsNullOrWhiteSpace($OutputDir) -or `
        [string]::IsNullOrWhiteSpace($InputDir)) {
        Throw "Error: Missing required arguments for build function."
    }

    Write-Host "Preparing build environment for Arch: ${Arch}"

    # Configure Git
    git config --global --add safe.directory "$(Get-Location)"

    # @latest has an issue with path separators, so pin to an older version for now.
    # https://github.com/google/googet/issues/83#issuecomment-2536975624
    Write-Host "Installing goopack..."
    go install -trimpath -ldflags="-s -w" github.com/google/googet/v2/goopack@v2.18.4

    Write-Host "Preparing directories and files..."
    New-Item -Path "out" -ItemType Directory -Force | Out-Null
    New-Item -Path $OutputDir -ItemType Directory -Force | Out-Null

    # Move pre-built files
    $SourcePath = Join-Path $InputDir "result\out\*"
    Move-Item -Path $SourcePath -Destination ".\out\" -Force

    Write-Host "Current directory contents:"
    Get-ChildItem -Force | Select-Object Name, LastWriteTime, Length

    $BuildScriptPath = ".\pkg\goo\build.ps1"
    Write-Host "Delegating package creation to $BuildScriptPath..."

    if (-not (Test-Path $BuildScriptPath)) {
        Throw "Error: build.ps1 not found at: $BuildScriptPath"
    }

    & $BuildScriptPath -DestDir $OutputDir -Arch $Arch

    if ($LASTEXITCODE -ne 0) {
        Throw "Error: build.ps1 failed with exit code $LASTEXITCODE"
    }

    Write-Host "Package process complete. Output at: ${OutputDir}"
}

Write-Host "Starting Entry Point Script..."

# 1. Validate Environment Variables
$RequiredVars = @("_LOUHI_TAG_NAME", "KOKORO_GFILE_DIR", "KOKORO_ARTIFACTS_DIR", "_STAGING_ARTIFACTS_PROJECT_ID")
foreach ($var in $RequiredVars) {
    if (-not (Get-Item "env:$var" -ErrorAction SilentlyContinue)) {
        Throw "Error: Required environment variable '$var' is not set."
    }
}

# 2. Install Go
Install-Go

# 3. Parse the Louhi tag
$LouhiTag = $env:_LOUHI_TAG_NAME
$LouhiParts = $LouhiTag -split "/"

if ($LouhiParts.Count -lt 5) {
    Throw "Error: _LOUHI_TAG_NAME format is unexpected: $LouhiTag"
}

$Ver    = $LouhiParts[1]
$Ref    = $LouhiParts[2]
$Target = $LouhiParts[3]
$Arch   = $LouhiParts[4]

# 4. Define Paths (Standardized to PascalCase)
$InputDir  = $env:KOKORO_GFILE_DIR
$OutputDir = Join-Path $env:KOKORO_ARTIFACTS_DIR "result"
$TargetDir = "git\unified_agents"

# 5. Change Directory
Write-Host "Changing directory to $TargetDir..."
if (-not (Test-Path $TargetDir)) {
    Throw "Error: Could not find directory '$TargetDir'. Current location is $(Get-Location)"
}
Set-Location $TargetDir

# 6. Execute Core Build Logic
Invoke-PackageBuild -Arch $Arch -InputDir $InputDir -OutputDir $OutputDir

# 7. Upload Artifacts
Write-Host "Uploading artifacts..."

$GcsBucket = "gs://$($env:_STAGING_ARTIFACTS_PROJECT_ID)-ops-agent-releases/$Ver/$Ref/$Target/$Arch/"

Write-Host "Copying artifacts to $GcsBucket"

# Upload .goo files
$GooFiles = Join-Path $OutputDir "*.goo"
gsutil cp $GooFiles "$GcsBucket"

# Upload tar.gz plugin files
$PluginTar = Join-Path $InputDir "result\google-cloud-ops-agent-plugin*.tar.gz"
gsutil cp $PluginTar "$GcsBucket"

# Upload SHA256 text file
$ShaFile = Join-Path $InputDir "result\google-cloud-ops-agent-plugin-sha256.txt"
gsutil cp $ShaFile "$GcsBucket"

Write-Host "Script finished successfully."
