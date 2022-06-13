@ECHO OFF

PowerShell.exe -Command "& %KOKORO_ARTIFACTS_DIR%/github/unified_agents/kokoro/scripts/build/build_package.ps1"

exit %ERRORLEVEL%
