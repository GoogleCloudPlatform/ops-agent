New-Item -Path "$env:KOKORO_ARTIFACTS_DIR" -Name 'result' -ItemType 'directory'

robocopy "${env:KOKORO_GFILE_DIR}/result" "${env:KOKORO_ARTIFACTS_DIR}/result" /E

Set-Location "${env:KOKORO_ARTIFACTS_DIR}/result"

$timestamp_server = 'http://timestamp.digicert.com'
Write-Host "Using the timestamp server: '$timestamp_server'"

$files_to_sign = @(
    'out/bin/fluent-bit.exe',
    'out/bin/fluent-bit.dll',
    'out/bin/google-cloud-ops-agent.exe',
    'out/bin/google-cloud-ops-agent-wrapper.exe',
    'pkg/goo/maint.ps1'
)

$metrics_agent_files = Get-ChildItem -Path 'out/bin/' -Filter 'google-cloud-metrics-agent_windows_*.exe' -ErrorAction SilentlyContinue |
    Where-Object { $_.Name -match '^google-cloud-metrics-agent_windows_(amd64|386)\.exe$' }

if ($metrics_agent_files) {
    # If multiple files match, we'll use the first one found and issue a warning.
    if ($metrics_agent_files.Count -gt 1) {
        throw "ERROR: Multiple Google Cloud Metrics Agent executables found in 'out/bin/'. Cannot proceed with signing. Files found: $($found_files -join ', ')"
    }
    $files_to_sign += "out/bin/$($metrics_agent_files[0].Name)"
    Write-Host "Found Google Cloud Metrics Agent executable: 'out/bin/$($metrics_agent_files[0].Name)'"
}
else {
  # Throw an error if no file matching the specific pattern is found
  throw "ERROR: Could not find the Google Cloud Metrics Agent executable for amd64 or 386 in 'out/bin/'."
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
