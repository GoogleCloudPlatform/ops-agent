Set-PSDebug -Trace 1
$ErrorActionPreference = 'Stop'
$global:ProgressPreference = 'SilentlyContinue'

# Invokes the first argument (expected to be an external program) and passes it
# the rest of the arguments. Throws an error if the program finishes with a
# nonzero exit code.
#   Example: Invoke-Program git submodule update --init
function Invoke-Program() {
  $outpluserr = cmd /c $Args 2`>`&1
  if ( $LastExitCode -ne 0 ) {
    throw "failed: $Args, output: $outpluserr"
  }
  return $outpluserr
}

$tag = 'build'
$name = 'build-result'

# Try to disable Windows Defender antivirus for improved build speed.
# Sometimes it seems that Defender is already disabled and it fails with
# an error like "Set-MpPreference : Operation failed with the following error: 0x800106ba"
Set-MpPreference -Force -DisableRealtimeMonitoring $true -ErrorAction Continue
# Try to disable Windows Defender firewall for improved build speed.
Set-NetFirewallProfile -Profile Domain,Public,Private -Enabled False -ErrorAction Continue

$gitOnBorgLocation = "$env:KOKORO_ARTIFACTS_DIR/git/unified_agents"
if (Test-Path -Path $gitOnBorgLocation) {
  Set-Location $gitOnBorgLocation
}
else {
  Set-Location "$env:KOKORO_ARTIFACTS_DIR/github/unified_agents"
}

# Record OPS_AGENT_REPO_HASH so that we can later run tests from the
# same commit that the agent was built from. This only applies to the
# build+test flow for release builds, not the GitHub presubmits.
$hash = Invoke-Program git -C . rev-parse HEAD

# Set variables from the VERSION file. Currently this is only PKG_VERSION.
Get-Content VERSION | Where-Object length | ForEach-Object { Invoke-Expression "`$env:$_" };

# Write OPS_AGENT_REPO_HASH and PACKAGE_VERSION into Sponge custom config variables file.
Write-Output @"
OPS_AGENT_REPO_HASH,$hash
PACKAGE_VERSION,$env:PKG_VERSION
"@ |
  Out-File -FilePath "$env:KOKORO_ARTIFACTS_DIR/custom_sponge_config.csv" -Encoding ascii

Invoke-Program git submodule update --init
$artifact_registry='us-docker.pkg.dev'
Invoke-Program docker-credential-gcr configure-docker --registries="$artifact_registry"
$arch = Invoke-Program docker info --format '{{.Architecture}}'
$cache_location="${artifact_registry}/stackdriver-test-143416/google-cloud-ops-agent-build-cache/ops-agent-cache:windows-${arch}"
Invoke-Program docker pull $cache_location
Invoke-Program docker build --cache-from="${cache_location}" -t $tag -f './Dockerfile.windows' .
Invoke-Program docker create --name $name $tag
Invoke-Program docker cp "${name}:/work/out" $env:KOKORO_ARTIFACTS_DIR


# Tell our continuous build to update the cache. Our other builds do not
# write to any kind of cache, for example a per-PR cache, because the
# push takes a few minutes and adds little value over just using the continuous
# build's cache.
if ($env:KOKORO_ROOT_JOB_TYPE -eq 'CONTINUOUS_INTEGRATION') {
  Invoke-Program docker image tag $tag $cache_location
  Invoke-Program docker push $cache_location
}

# Copy the .goo file from $env:KOKORO_ARTIFACTS_DIR/out to $env:KOKORO_ARTIFACTS_DIR/result.
# The .goo file is the installable package that is distributed to customers.
New-Item -Path $env:KOKORO_ARTIFACTS_DIR -Name 'result' -ItemType 'directory'
Move-Item -Path "$env:KOKORO_ARTIFACTS_DIR/out/*.goo" -Destination "$env:KOKORO_ARTIFACTS_DIR/result"
# Copy the .pdb and .dll files from $env:KOKORO_ARTIFACTS_DIR/out/bin to $env:KOKORO_ARTIFACTS_DIR/result.
# The .pdb and .dll files are saved so the team can use them in the event that we have to debug this Ops Agent build. 
# They are not distributed to customers.
Move-Item -Path "$env:KOKORO_ARTIFACTS_DIR/out/bin/*.pdb" -Destination "$env:KOKORO_ARTIFACTS_DIR/result"
Move-Item -Path "$env:KOKORO_ARTIFACTS_DIR/out/bin/*.dll" -Destination "$env:KOKORO_ARTIFACTS_DIR/result"
