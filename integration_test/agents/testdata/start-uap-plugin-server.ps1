$ErrorActionPreference = 'Stop'
$taskName = "UAPWindowsPlugin"
if (-not(Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue)) {
    $action = New-ScheduledTaskAction -Execute "C:\plugin.exe" -Argument "-address localhost:1234 -errorlogfile errorlog.txt -protocol tcp" -WorkingDirectory "C:\"
    $trigger = New-ScheduledTaskTrigger -Once -At (Get-Date)
    $principal = New-ScheduledTaskPrincipal -UserId "NT AUTHORITY\SYSTEM" -LogonType ServiceAccount -RunLevel Highest
    Register-ScheduledTask -TaskName $taskName -Action $action  -Principal $principal
}
Start-ScheduledTask -TaskName $taskName


