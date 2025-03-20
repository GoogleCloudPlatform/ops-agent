$ErrorActionPreference = 'Stop'
$currentUser = [System.Security.Principal.WindowsIdentity]::GetCurrent()
$action = New-ScheduledTaskAction -Execute "C:\plugin.exe" -Argument "-address localhost:1234 -errorlogfile errorlog.txt -protocol tcp" -WorkingDirectory "C:\"
$trigger = New-ScheduledTaskTrigger -Once -At (Get-Date)
$principal = New-ScheduledTaskPrincipal -UserId $currentUser.User -LogonType Interactive -RunLevel Highest
Register-ScheduledTask -TaskName "UAPWindowsPlugin" -Action $action -Trigger $trigger -Principal $principal
Start-ScheduledTask -TaskName "UAPWindowsPlugin"