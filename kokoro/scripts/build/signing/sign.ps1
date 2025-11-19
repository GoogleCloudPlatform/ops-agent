New-Item -Path "$env:KOKORO_ARTIFACTS_DIR" -Name 'result' -ItemType 'directory'

robocopy "${env:KOKORO_GFILE_DIR}/result" "${env:KOKORO_ARTIFACTS_DIR}/result" /E

Set-Location "${env:KOKORO_ARTIFACTS_DIR}/result"

$timestamp_server = 'http://timestamp.digicert.com'
Write-Host "Using the timestamp server: '$timestamp_server'"

# 1. Start with the specific manual files
$files_to_process = @(
    'pkg/goo/maint.ps1'
)

# 2. Add all .exe and .dll files found in out/bin
if (Test-Path 'out/bin') {
    $bin_files = Get-ChildItem -Path 'out/bin' -Recurse -Include *.exe, *.dll
    foreach ($file in $bin_files) {
        # We add the FullName (absolute path) to ensure accuracy
        $files_to_process += $file.FullName
    }
}
else {
    Write-Warning "Directory 'out/bin' was not found. No binaries were added from that location."
}

$script_exit_code = 0

foreach ($file_path in $files_to_process) {

    # Verify the file actually exists before trying to check signatures
    if (-not (Test-Path $file_path)) {
        Write-Warning "File not found: $file_path. Skipping."
        continue
    }

    # 3. Check if the file is already signed
    $current_signature = Get-AuthenticodeSignature $file_path

    if ($current_signature.Status -eq 'Valid') {
        Write-Host "Skipping '$file_path': Algorithm verified it is already signed."
        continue
    }

    Write-Host "Processing '$file_path': Signature status is '$($current_signature.Status)'. Proceeding to sign."

    # 4. Sign the file
    # We wrap $file_path in quotes to handle spaces
    $sign_command = "& ksigntool.exe sign GOOGLE_EXTERNAL /v /debug /t $timestamp_server `"$file_path`" 2>&1"
    Write-Host "Signing: $sign_command"

    $out = Invoke-Expression $sign_command
    $sign_code = $LastExitCode
    Write-Host -separator "`n" $out
    Write-Host "Exit code (Sign): $sign_code"

    if ($sign_code -ne 0) {
        Write-Error "ERROR: Signing $file_path FAILED with exit code $sign_code."
        $script_exit_code = $sign_code
        # Continue to next file, but script will exit with error code at the end
        continue
    }

    # 5. Verify the signature
    $verify_command = "& signtool.exe verify /pa /all `"$file_path`" 2>&1"
    Write-Host "Verifying: $verify_command"

    $out = Invoke-Expression $verify_command
    $verify_code = $LastExitCode
    Write-Host -separator "`n" $out
    Write-Host "Exit code (Verify): $verify_code"

    if ($verify_code -ne 0) {
        Write-Error "ERROR: Verification of $file_path FAILED with exit code $verify_code."
        $script_exit_code = $verify_code
    }
}

exit $script_exit_code
