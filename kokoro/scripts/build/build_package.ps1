Set-PSDebug -Trace 1
$ErrorActionPreference = 'Stop'

# Invokes the first argument, expected to be an external program,
# passing it the rest of the arguments. Throws an error if the program
# finishes with a nonzero exit code.
function Invoke-Program() {
  & $Args[0] $Args[1..$Args.Length]
  if ( $LastExitCode -ne 0 ) {
    throw "failed: $Args"
  }
}

$tag = 'build'
$name = 'build-result'

# Try to disable Windows Defender antivirus for improved build speed.
Set-MpPreference -Force -DisableRealtimeMonitoring $true -ErrorAction Continue
# Try to disable Windows Defender firewall for improved build speed.
Set-NetFirewallProfile -Profile Domain,Public,Private -Enabled False -ErrorAction Continue

$gitDir = 'github'
if (Test-Path env:KOKORO_GOB_COMMIT_URL_unified_agents) {
  $gitDir = 'git'
}

Set-Location "$env:KOKORO_ARTIFACTS_DIR/$gitDir/unified_agents"
Invoke-Program git submodule update --init
Invoke-Program docker build -t $tag -f './Dockerfile.windows' .
Invoke-Program docker create --name $name $tag
Invoke-Program docker cp "${name}:/work/out" $env:KOKORO_ARTIFACTS_DIR

# Copy the .goo file from $env:KOKORO_ARTIFACTS_DIR/out to $env:KOKORO_ARTIFACTS_DIR/result.
New-Item -Path $env:KOKORO_ARTIFACTS_DIR -Name 'result' -ItemType 'directory'
Move-Item -Path "$env:KOKORO_ARTIFACTS_DIR/out/*.goo" -Destination "$env:KOKORO_ARTIFACTS_DIR/result"
