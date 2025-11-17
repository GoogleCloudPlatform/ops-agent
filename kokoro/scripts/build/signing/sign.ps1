New-Item -Path "$env:KOKORO_ARTIFACTS_DIR" -Name 'result' -ItemType 'directory'

robocopy "${env:KOKORO_GFILE_DIR}/result" "${env:KOKORO_ARTIFACTS_DIR}/result" /E

Set-Location "${env:KOKORO_ARTIFACTS_DIR}/result"

$timestamp_server = 'http://timestamp.digicert.com'
Write-Host "Using the timestamp server: '$timestamp_server'"

$files_to_sign = @(
    'out/bin/fluent-bit.exe',
    'out/bin/fluent-bit.dll',
    'out/bin/google-cloud-ops-agent.exe',
    'out/bin/google-cloud-ops-agent-diagnostics.exe',
    'out/bin/google-cloud-ops-agent-wrapper.exe',
    'pkg/goo/maint.ps1'
)

$amd64_path = 'out/bin/google-cloud-metrics-agent_windows_amd64.exe'
$x86_path = 'out/bin/google-cloud-metrics-agent_windows_386.exe'

# Check which file exists and add it to the list.
if (Test-Path $amd64_path) {
    $files_to_sign += $amd64_path
}
elseif (Test-Path $x86_path) {
    $files_to_sign += $x86_path
}
else {
    throw "ERROR: Could not find the Google Cloud Metrics Agent executable. Checked for '$amd64_path' and '$x86_path'."
}

$script_exit_code = 0

$files_to_sign | ForEach-Object {
    $file = $_

    $sign_command = "& ksigntool.exe sign GOOGLE_EXTERNAL /v /debug /t $timestamp_server $file 2>&1"
    Write-Host "Signing: $sign_command"
    $out = Invoke-Expression $sign_command
    $sign_code = $LastExitCode
    Write-Host -separator "`n" $out
    Write-Host "Exit code (Sign): $sign_code"

    if ($sign_code -ne 0) {
        Write-Error "ERROR: Signing $file FAILED with exit code $sign_code."
        $script_exit_code = $sign_code
    }

    $verify_command = "& signtool.exe verify /pa /all $file 2>&1"
    Write-Host "Verifying: $verify_command"
    $out = Invoke-Expression $verify_command
    $verify_code = $LastExitCode
    Write-Host -separator "`n" $out
    Write-Host "Exit code (Verify): $verify_code"

    if ($verify_code -ne 0) {
        Write-Error "ERROR: Verification of $file FAILED with exit code $verify_code."
        $script_exit_code = $verify_code
    }
}

exit $script_exit_code
