# ESSL Attendance Sync - Windows Task Scheduler Setup
# Run as Administrator

$installDir = "C:\AttendanceSync"
$taskName = "ESSL Attendance Sync"

# Create install directory if not exists
if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir -Force
    Write-Host "Created directory: $installDir"
}

# Check if exe exists
if (-not (Test-Path "$installDir\fetchFromMachine.exe")) {
    Write-Host "ERROR: fetchFromMachine.exe not found in $installDir"
    Write-Host "Please copy fetchFromMachine.exe and .env to $installDir first."
    exit 1
}

# Check if .env exists
if (-not (Test-Path "$installDir\.env")) {
    Write-Host "ERROR: .env not found in $installDir"
    Write-Host "Please copy .env.example to .env and configure it."
    exit 1
}

# Remove existing task if present
$existingTask = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
if ($existingTask) {
    Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
    Write-Host "Removed existing task: $taskName"
}

# Create action
$action = New-ScheduledTaskAction `
    -Execute "$installDir\fetchFromMachine.exe" `
    -WorkingDirectory $installDir

# Create trigger (every 2 hours, indefinitely)
$trigger = New-ScheduledTaskTrigger `
    -Once `
    -At (Get-Date) `
    -RepetitionInterval (New-TimeSpan -Hours 2) `
    -RepetitionDuration (New-TimeSpan -Days 3650)

# Create settings
$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -RestartCount 0 `
    -ExecutionTimeLimit (New-TimeSpan -Minutes 30)

# Register task
Register-ScheduledTask `
    -TaskName $taskName `
    -Action $action `
    -Trigger $trigger `
    -Settings $settings `
    -Description "Fetches attendance from ESSL biometric machine and posts to API every 2 hours"

Write-Host ""
Write-Host "SUCCESS: Task '$taskName' created!"
Write-Host ""
Write-Host "To verify: Open Task Scheduler and check '$taskName'"
Write-Host "To run now: Right-click the task and select 'Run'"
Write-Host "To view history: Enable 'All Tasks History' in Task Scheduler"
