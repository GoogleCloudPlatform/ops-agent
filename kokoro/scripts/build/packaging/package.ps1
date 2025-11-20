$ErrorActionPreference = "Stop"

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

    # Map Arch to GoArch
    switch ($Arch) {
        "x86_64" { $GoArch = "amd64" }
        "x86"    { $GoArch = "386" }
        Default { Throw "ERROR: Architecture must be set to one of: x86, x86_64" }
    }

    Write-Host "Starting build for Arch: ${Arch} (GoArch: ${GoArch})"

    # Configure Git
    git config --global --add safe.directory "$(Get-Location)"

    # @latest has an issue with path separators, so pin to an older version for now.
    # https://github.com/google/googet/issues/83#issuecomment-2536975624
    Write-Host "Installing goopack..."
    go install -trimpath -ldflags="-s -w" github.com/google/googet/v2/goopack@v2.18.4

    Write-Host "Preparing directories and files..."
    New-Item -Path "out" -ItemType Directory -Force | Out-Null
    New-Item -Path $OutputDir -ItemType Directory -Force | Out-Null

    # Move pre-built files: mv "${InputDir}/result/out/"* ./out/
    $SourcePath = Join-Path $InputDir "result\out\*"
    Move-Item -Path $SourcePath -Destination ".\out\" -Force

    Write-Host "Current directory contents:"
    Get-ChildItem -Force | Select-Object Name, LastWriteTime, Length

    # Extract package version from VERSION file
    try {
        $VersionContent = Get-Content -Path ".\VERSION" -ErrorAction Stop
        # Look for the line containing PKG_VERSION
        $VersionLine = $VersionContent | Select-String "PKG_VERSION"
        if ($VersionLine) {
            # Parse 'PKG_VERSION="1.2.3"' -> split by '=', trim spaces and quotes
            $releaseName = $VersionLine.ToString().Split('=')[1].Trim().Trim('"')
        } else {
            Throw "PKG_VERSION not found in ./VERSION"
        }
    }
    catch {
        Throw "Failed to read release version: $_"
    }

    Write-Host "Building package version: ${releaseName}"

    # Determine GOPATH
    $GOPATH = $env:GOPATH
    if ([string]::IsNullOrWhiteSpace($GOPATH)) {
        $GOPATH = "$env:USERPROFILE\go"
    }

    # Run goopack
    $GoopackPath = "$GOPATH\bin\goopack.exe"

    & $GoopackPath -output_dir "$OutputDir" `
        "-var:PKG_VERSION=$releaseName" `
        "-var:ARCH=$Arch" `
        "-var:GOOS=windows" `
        "-var:GOARCH=$GoArch" `
        pkg/goo/google-cloud-ops-agent.goospec

    Write-Host "Build process complete. Output at: ${OutputDir}"
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
# Indices: 0    /1     /2     /3      /4     /5
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
Write-Host "Changing directory to git/unified_agents..."
$TargetDir = Join-Path (Get-Location) "git\unified_agents"

if (Test-Path $TargetDir) {
    Set-Location $TargetDir
} else {
    # Fallback if running relative to root already or path structure differs
    if (Test-Path "git/unified_agents") { Set-Location "git/unified_agents" }
    else { Throw "Could not find directory 'git/unified_agents'" }
}

# 5. Execute Core Build Logic (Function Call)
# This replaces the call to ./kokoro/scripts/build/packaging/package_windows.sh
Invoke-PackageBuild -Arch $Arch -InputDir $INPUT_DIR -OutputDir $OUTPUT_DIR

# 6. Upload Artifacts
Write-Host "Uploading artifacts..."

# Construct GCS Bucket URL
$GcsBucket = "gs://$($env:_STAGING_ARTIFACTS_PROJECT_ID)-ops-agent-releases/$Ver/$Ref/$Target/$Arch/"

Write-Host "Copying *.goo files to $GcsBucket"

# Upload .goo files
# We verify files exist first to avoid cryptic gsutil errors, though gsutil handles wildcards well.
$GooFiles = Join-Path $OUTPUT_DIR "*.goo"
gsutil cp $GooFiles "$GcsBucket"

# Upload tar.gz plugin files
$PluginTar = Join-Path $INPUT_DIR "result\google-cloud-ops-agent-plugin*.tar.gz"
gsutil cp $PluginTar "$GcsBucket"

# Upload SHA256 text file
$ShaFile = Join-Path $INPUT_DIR "result\google-cloud-ops-agent-plugin-sha256.txt"
gsutil cp $ShaFile "$GcsBucket"

Write-Host "Script finished successfully."
