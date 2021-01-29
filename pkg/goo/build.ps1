Param(
    [Parameter(Mandatory=$false)][string]$DestDir,
    [Parameter(Mandatory=$false)][string]$Arch
)

if (!$DestDir) {
  $DestDir = '.'
}

# Read PKG_VERSION from VERSION file.
$PkgVersion = Select-String -Path "VERSION" -Pattern '^PKG_VERSION="(.*)"$' | %{$_.Matches.Groups[1].Value}

# If ARCH is not supplied, set default value based on user's system.
if (!$Arch) {
  $Arch = (&{If([System.Environment]::Is64BitProcess) {'x86_64'} Else {'x86'}})
}

$GoOs = 'windows'

# Set GOARCH based on ARCH.
switch ($Arch) {
    'x86_64' { $GoArch = 'amd64'; break}
    'x86'    { $GoArch = '386';   break}
    default  { Throw 'Arch must be set to one of: x86, x86_64' }
}

# Substitute variables.
(Get-Content pkg/goo/google-cloud-ops-agent.goospec) `
  -replace '\${DESTDIR}',$DestDir `
  -replace '\${PKG_VERSION}',$PkgVersion `
  -replace '\${ARCH}',$Arch `
  -replace '\${GOOS}',$GoOs `
  -replace '\${GOARCH}',$GoArch `
  | Set-Content pkg/goo/tmp-google-cloud-ops-agent.goospec

# Build the .goo package.
# TODO: invoke the subagent builds via goopack.
& $env:GOPATH\bin\goopack -output_dir $DestDir pkg/goo/tmp-google-cloud-ops-agent.goospec
