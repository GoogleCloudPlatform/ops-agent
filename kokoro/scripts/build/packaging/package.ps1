$ErrorActionPreference = "Stop"

# Configuration
$MetadataServerIP = "169.254.169.254"

function Invoke-PackageBuild {
    param(
        [Parameter(Mandatory=$true)][string]$Arch,
        [Parameter(Mandatory=$true)][string]$OutputDir,
        [Parameter(Mandatory=$true)][string]$InputDir
    )

    # Validate arguments
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
    # build.ps1 uses the destination, but we ensure it exists here to be safe
    New-Item -Path "out" -ItemType Directory -Force | Out-Null
    New-Item -Path $OutputDir -ItemType Directory -Force | Out-Null

    # Move pre-built files: mv "${InputDir}/result/out/"* ./out/
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

# New wrapper function to handle gcloud calls and error checking
function Upload-Artifact {
    param(
        [Parameter(Mandatory=$true)]
        [string]$Path,
        [Parameter(Mandatory=$true)]
        [string]$Destination
    )

    Write-Host "Uploading $Path to $Destination..."

    # Execute gcloud command
    gcloud storage cp $Path $Destination

    # Immediate error check
    if ($LASTEXITCODE -ne 0) {
        Throw "Error: Failed to upload '$Path'. gcloud exited with code $LASTEXITCODE."
    }
}

Write-Host "Starting Entry Point Script..."

# 1. Validate Environment Variables
$RequiredVars = @("_LOUHI_TAG_NAME", "KOKORO_GFILE_DIR", "KOKORO_ARTIFACTS_DIR", "_STAGING_ARTIFACTS_PROJECT_ID")
foreach ($var in $RequiredVars) {
    if (-not (Get-Item "env:$var" -ErrorAction SilentlyContinue)) {
        Throw "Error: Required environment variable '$var' is not set."
    }
}

# 2. Parse the Louhi tag
# Example: louhi/2.46.0/abcdef/windows/x86_64/start
$LouhiTag = $env:_LOUHI_TAG_NAME
$LouhiParts = $LouhiTag -split "/"

if ($LouhiParts.Count -lt 5) {
    Throw "Error: _LOUHI_TAG_NAME format is unexpected: $LouhiTag"
}

$Arch = $LouhiParts[4]
$Ver = $LouhiParts[1]
$Ref = $LouhiParts[2]
$Target = $LouhiParts[3]

# 3. Define Paths
$INPUT_DIR = $env:KOKORO_GFILE_DIR
$OUTPUT_DIR = Join-Path $env:KOKORO_ARTIFACTS_DIR "result"

# 4. Change Directory
$TargetDir = "git\unified_agents"
Write-Host "Changing directory to $TargetDir..."

if (-not (Test-Path $TargetDir)) {
    # Fail fast if the directory structure isn't what we expect
    Throw "Error: Could not find directory '$TargetDir'. Current location is $(Get-Location)"
}

Set-Location $TargetDir

# 5. Execute Core Build Logic (Function Call)
Invoke-PackageBuild -Arch $Arch -InputDir $INPUT_DIR -OutputDir $OUTPUT_DIR

# 6. Upload Artifacts
Write-Host "Uploading artifacts..."

# Workaround for a known issue where Windows containers cannot reach the GCP Metadata server.
# see b/467401022 for details.
Write-Host "Applying Network Routing Fix..."
$gateway = (Get-NetRoute | Where-Object { $_.DestinationPrefix -eq '0.0.0.0/0' } | Sort-Object RouteMetric | Select-Object -ExpandProperty NextHop -First 1)
$ifIndex = (Get-NetAdapter -InterfaceDescription "Hyper-V Virtual Ethernet*" | Sort-Object | Select-Object -ExpandProperty ifIndex -First 1)

# Using the variable for the route prefix
New-NetRoute -DestinationPrefix "$MetadataServerIP/32" -InterfaceIndex $ifIndex -NextHop $gateway -ErrorAction SilentlyContinue

# Verify Metadata Connectivity
Write-Host "Verifying connectivity to Metadata server ($MetadataServerIP)..."
try {
    # Using the variable for the connection test
    Test-Connection -ComputerName $MetadataServerIP -Count 1
}
catch {
    Write-Warning "Ping to Metadata server failed. Authentication might fail in the next steps."
}

# Construct GCS Bucket URL
$GcsBucket = "gs://$($env:_STAGING_ARTIFACTS_PROJECT_ID)-ops-agent-releases/$Ver/$Ref/$Target/$Arch/"
Write-Host "Destination: $GcsBucket"

# Upload .goo files
$GooFiles = Join-Path $OUTPUT_DIR "*.goo"
Upload-Artifact -Path $GooFiles -Destination $GcsBucket

# Upload tar.gz plugin files
$PluginTar = Join-Path $INPUT_DIR "result\google-cloud-ops-agent-plugin*.tar.gz"
Upload-Artifact -Path $PluginTar -Destination $GcsBucket

# Upload SHA256 text file
$ShaFile = Join-Path $INPUT_DIR "result\google-cloud-ops-agent-plugin-sha256.txt"
Upload-Artifact -Path $ShaFile -Destination $GcsBucket

Write-Host "Script finished successfully."
