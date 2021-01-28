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

$ErrorActionPreference = 'Stop'

# TODO: figure out these settings
function Set-ServiceConfig {
    # restart after 1s then 2s, reset error counter after 60s
    sc.exe failure google-cloud-metrics-agent reset=60 actions=restart/1000/restart/2000
    # set delayed start
    sc.exe config google-cloud-metrics-agent depend="rpcss" start=delayed-auto
    # create trrigger to start the service on first IP address
    sc.exe triggerinfo google-cloud-metrics-agent start/networkon
}

$InstallDir = "%ProgramFiles%\Google\Cloud Operations\Ops Agent"

try
{
    if (-not (Get-Service "google-cloud-metrics-agent" -ErrorAction SilentlyContinue))
    {
        New-Service -DisplayName "Google Cloud Metrics Agent" `
            -Name "google-cloud-metrics-agent" `
            -BinaryPathName "$InstallDir\google-cloud-metrics-agent.exe --add-instance-id=false --config=""$InstallDir\config.yaml""" `
            -Description "Collects agent metrics and reports them to Google Cloud Operations."
        Set-ServiceConfig
        Start-Service -Name "google-cloud-metrics-agent" -Verbose -ErrorAction Stop
    }
    else
    {
        Set-ServiceConfig
        Restart-Service -Name "google-cloud-metrics-agent"
    }
}
catch
{
    Write-Output $_.InvocationInfo.PositionMessage
    Write-Output "Install failed: $($_.Exception.Message)"
    exit 1
}
