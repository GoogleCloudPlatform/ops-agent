# Copyright 2021, Google Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

#Requires -Version 3.0

Param(
    [Parameter(Mandatory=$true)][string]$InstallDir,
    [Parameter(Mandatory=$true)][ValidateSet('install','uninstall')][string]$Action
)

$ErrorActionPreference = 'Stop'

$envFromMatch = {
  Param($match)
  (Get-ChildItem -Path Env: | `
     Where-Object -Property Name -eq $match.Groups[1].Value).Value
}
$InstallDir = [regex]::Replace($InstallDir,'^<([^>]+)>',$envFromMatch)

$configFilePath = "$InstallDir\config\config.yaml"

if ($Action -eq "install") {
    if (-not(Test-Path -Path $configFilePath -PathType Leaf)) {
         try {
             New-Item -ItemType directory -Path "$InstallDir\config" -Force
             Copy-Item -Path "$($PSScriptRoot.TrimEnd("\pkg\goo"))\confgenerator\default-config.yaml" -Destination "$configFilePath"
             Write-Host "The file [$configFilePath] has been created."
         }
         catch {
             throw $_.Exception.Message
         }
     }
     else {
         Write-Host "Keep [$configFilePath] as-is because a file with that name already exists."
     }
     # Sleep for 20s before installing services to allow previous service deletion to complete.
     Start-Sleep -s 20
}

& "$InstallDir\bin\google-cloud-ops-agent.exe" "--$Action"
