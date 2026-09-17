$ErrorActionPreference = 'Stop'
$taskName = "UAPWindowsPlugin"
$portNum = 1234
if (-not (Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue)) {
    $action = New-ScheduledTaskAction -Execute "C:\ops_agent.exe" -Argument "-address localhost:$portNum -errorlogfile errorlog.txt -protocol tcp" -WorkingDirectory "C:\"
    $trigger = New-ScheduledTaskTrigger -Once -At (Get-Date)
    $principal = New-ScheduledTaskPrincipal -UserId "NT AUTHORITY\SYSTEM" -LogonType ServiceAccount -RunLevel Highest
    Register-ScheduledTask -TaskName $taskName -Action $action -Principal $principal
    Start-Sleep -Seconds 1
}

for ($i = 1; $i -le 10; $i++) {
    $proc = Get-Process -Name "ops_agent" -ErrorAction SilentlyContinue
    if (-not $proc) {
        Write-Host "Attempt ${i}: starting scheduled task $taskName..."
        Start-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    } else {
        Write-Host "Attempt ${i}: ops_agent.exe (PID $($proc.Id)) is running, waiting for port $portNum..."
    }
    Start-Sleep -Seconds 3
    $proc = Get-Process -Name "ops_agent" -ErrorAction SilentlyContinue
    $port = Get-NetTCPConnection -LocalPort $portNum -State Listen -ErrorAction SilentlyContinue
    if ($proc -and $port) {
        Write-Host "ops_agent.exe (PID $($proc.Id)) is running and listening on port $portNum."
        exit 0
    }
}

$info = Get-ScheduledTaskInfo -TaskName $taskName -ErrorAction SilentlyContinue | Format-List | Out-String
throw "Failed to start $taskName and bind port $portNum after 10 attempts. Task info: $info"
