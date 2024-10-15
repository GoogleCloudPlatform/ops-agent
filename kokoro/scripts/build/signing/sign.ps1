param (
  [parameter(HelpMessage="Enter a GCS URL that needs to end in a '/'")]
  [ValidatePattern("^$|^gs://.*/.*/$")]
  [string]$GcsBucketUrl
)
if ([string]::IsNullOrEmpty($GcsBucketUrl)) { {
 Write-Host ('No Signing Bucket URL, skipping signing')
 exit 0
}

$file_filter = @(
'out/bin/fluent-bit.exe',
'out/bin/fluent-bit.dll',
'out/bin/google-cloud-metrics-agent.exe',
'out/bin/google-cloud-ops-agent.exe',
'out/bin/google-cloud-ops-agent-diagnostics.exe',
'out/bin/google-cloud-ops-agent-wrapper.exe',
'pkg/goo/maint.ps1'
)

$file_filter | ForEach-Object {
    Write-Host "Sending: $_ to be signed"
    gsutil cp "./$_" "${GcsBucketUrl}$_"
}

# Sent to indicate all binaries have been sent to be signed
New-Item ./UNSIGNED_READY.txt -type file

Write-Host "Sent all unsigned binaries"

# Wait for binaries to be signed.
$i = 0
do {
  if ($i -ge 300) {
    throw "Could not get signed binaries"
  }
  elseif ($i -gt 0) {
    # Sleep for 15 seconds before the next attempt to avoid hammering the timestamp server.
    Start-Sleep -Seconds 1
  }
  $i++
gsutil -q stat "${GcsBucketUrl}SIGNED_READY.txt"
} until ($LastExitCode -eq 0)

$file_filter | ForEach-Object {
    Write-Host "Receiving: signed $_"
    gsutil cp "${GcsBucketUrl}$_" "./$_"
}

gsutil rm "${GcsBucketUrl}*.goo"

