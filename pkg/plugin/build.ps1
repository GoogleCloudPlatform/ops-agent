<#
 Copyright 2025 Google LLC

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
#>

Param(
    [Parameter(Mandatory=$false)][string]$DestDir,
    [Parameter(Mandatory=$false)][string]$Arch
)

if (!$DestDir) {
    $DestDir = '.'
}

# Read PKG_VERSION from VERSION file.
$PkgVersion = Select-String -Path "VERSION" -Pattern '^PKG_VERSION="(.*)"$' | %{$_.Matches.Groups[1].Value}
$env:PKG_VERSION=$PkgVersion

# If ARCH is not supplied, set default value based on user's system.
if (!$Arch) {
  $Arch = (&{If([System.Environment]::Is64BitProcess) {'x86_64'} Else {'x86'}})
}

# Set GOARCH based on ARCH.
switch ($Arch) {
    'x86_64' { $GoArch = 'amd64'; break}
    'x86'    { $GoArch = '386';   break}
    default  { Throw 'Arch must be set to one of: x86, x86_64' }
}

# Create the license directory
$LicenseDir = "$DestDir\THIRD_PARTY_LICENSES"
$Subfolder = "$LicenseDir\subfolder"

New-Item -ItemType Directory -Path $LicenseDir -Force | Out-Null
New-Item -ItemType Directory -Path $Subfolder -Force | Out-Null

"license to be added" | Out-File "$LicenseDir\text1.txt"
"license to be added" | Out-File "$Subfolder\text2.txt"

$TarFileName = "google-cloud-ops-agent-plugin_$PkgVersion-windows-$Arch.tar.gz" # Define tar file name

$FilesToInclude = @(
    "msvcp140.dll",
    "vccorlib140.dll",
    "vcruntime140.dll",
    "fluent-bit.exe",
    "fluent-bit.dll",
    "opentelemetry-java-contrib-jmx-metrics.jar",
    "google-cloud-metrics-agent_windows_${GoArch}.exe",
    "google-cloud-ops-agent-wrapper.exe"
    "ops_agent.exe"
    "THIRD_PARTY_LICENSES"
    "OPS_AGENT_VERSION"
)

# Create the tar archive
& tar -cvzf "$DestDir\$TarFileName" -C "$DestDir" $FilesToInclude

Write-Host "Tar archive created: $($DestDir)\$TarFileName"