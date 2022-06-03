@ECHO OFF

cd %KOKORO_ARTIFACTS_DIR%/piper/google3

PowerShell.exe -Command "& ./cloud/monitoring/agents/kokoro/build/ops_agent/build_package.ps1"

exit %ERRORLEVEL%