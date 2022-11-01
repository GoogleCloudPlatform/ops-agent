Set-PSDebug -Trace 1
$ErrorActionPreference = 'Stop'
$global:ProgressPreference = 'SilentlyContinue'

# Invokes the first argument (expected to be an external program) and passes it
# the rest of the arguments. Throws an error if the program finishes with a
# nonzero exit code.
#   Example: Invoke-Program git submodule update --init
function Invoke-Program() {
  & $Args[0] $Args[1..$Args.Length]
  if ( $LastExitCode -ne 0 ) {
    throw "failed: $Args"
  }
}
Asdf
