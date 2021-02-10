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

# Build the .goo package.
# TODO: invoke the subagent builds via goopack.
& $env:GOPATH\bin\goopack -output_dir $DestDir `
  -var:PKG_VERSION=$PkgVersion `
  -var:ARCH=$Arch `
  -var:GOOS=$Goos `
  -var:GOARCH=$GoArch `
  pkg/goo/google-cloud-ops-agent.goospec
