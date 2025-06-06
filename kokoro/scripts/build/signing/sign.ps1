@REM Copyright 2025 Google LLC
@REM
@REM Licensed under the Apache License, Version 2.0 (the "License");
@REM you may not use this file except in compliance with the License.
@REM You may obtain a copy of the License at
@REM
@REM     http://www.apache.org/licenses/LICENSE-2.0
@REM
@REM Unless required by applicable law or agreed to in writing, software
@REM distributed under the License is distributed on an "AS IS" BASIS,
@REM WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
@REM See the License for the specific language governing permissions and
@REM limitations under the License.

@REM Copy input directory into the artifacts directory recursively.
robocopy "%KOKORO_GFILE_DIR%"\result "%KOKORO_ARTIFACTS_DIR%"\result /E

Set-Location "%KOKORO_ARTIFACTS_DIR%"\result

$timestamp_server = 'http://timestamp.digicert.com'
Write-Host "Using the timestamp server: '$timestamp_server'"

$files_to_sign = @(
'out/bin/fluent-bit.exe',
'out/bin/fluent-bit.dll',
'out/bin/google-cloud-metrics-agent.exe',
'out/bin/google-cloud-ops-agent.exe',
'out/bin/google-cloud-ops-agent-diagnostics.exe',
'out/bin/google-cloud-ops-agent-wrapper.exe',
'pkg/goo/maint.ps1'
)

$files_to_sign | ForEach-Object {
    $sign_command = "& ksigntool.exe sign GOOGLE_EXTERNAL /v /debug /t $timestamp_server $_ 2>&1"
    $verify_command = "& signtool.exe verify /pa /all $_ 2>&1"

    Write-Host "Signing: $sign_command"
    $out = Invoke-Expression $sign_command
    Write-Host -separator "`n" $out
    Write-Host "Exit code: $LastExitCode"
    Write-Host "Verifying: $verify_command"
    $out = Invoke-Expression $verify_command
    Write-Host -separator "`n" $out
    Write-Host "Exit code: $LastExitCode"
}

exit $LastExitCode
