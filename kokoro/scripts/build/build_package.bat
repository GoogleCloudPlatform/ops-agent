@ECHO OFF

REM %~dp0 means "the directory this script is stored in, with a trailing backslash".
REM So overall, this command means "execute subling file build_package.ps1".
PowerShell.exe -Command "& %~dp0build_package.ps1"

exit %ERRORLEVEL%
