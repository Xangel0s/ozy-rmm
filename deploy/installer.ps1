<#
.SYNOPSIS
    Installs the ApexRMM agent as a Windows service.
.DESCRIPTION
    Copies agent.exe to C:\ProgramData\OzyShield, registers it as a Windows
    service with recovery policy, configures environment variables, and starts
    the service. Run as Administrator.
.PARAMETER BackendUrl
    The backend server address (e.g. "https://rmm.example.com" or "localhost:8080").
.PARAMETER EnrollToken
    Required. Registration token obtained from the RMM dashboard Settings page.
.PARAMETER BinaryPath
    Path to agent.exe. Defaults to .\agent.exe relative to the script.
.EXAMPLE
    .\installer.ps1 -BackendUrl "https://rmm.example.com" -EnrollToken "abc123..."
#>

param(
    [Parameter(Mandatory)]
    [string]$BackendUrl,
    [Parameter(Mandatory)]
    [string]$EnrollToken,
    [string]$BinaryPath = ""
)

$ErrorActionPreference = "Stop"
$ServiceName = "OzyShieldAgent"
$InstallDir = "C:\ProgramData\OzyShield"
$AgentExe = "agent.exe"
$EventSource = "OzyShieldAgent"

# ─── Admin check ───────────────────────────────────────────────────────────
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator
)
if (-not $isAdmin) {
    Write-Error "This script must be run as Administrator."
    exit 1
}

# ─── Resolve binary path ───────────────────────────────────────────────────
if (-not $BinaryPath) {
    $ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
    $BinaryPath = Join-Path -Path $ScriptDir -ChildPath "..\agent\agent.exe"
    if (-not (Test-Path $BinaryPath)) {
        $BinaryPath = Join-Path -Path $ScriptDir -ChildPath "agent.exe"
        if (-not (Test-Path $BinaryPath)) {
            $BinaryPath = ".\agent.exe"
        }
    }
}
$BinaryPath = Resolve-Path $BinaryPath -ErrorAction Stop

# ─── Create install directory ──────────────────────────────────────────────
if (-not (Test-Path $InstallDir)) {
    $null = New-Item -ItemType Directory -Force -Path $InstallDir
}

# ─── Stop + remove existing service ────────────────────────────────────────
$existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($existing) {
    Write-Host "[*] Stopping existing service..."
    Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
    & sc.exe delete $ServiceName 2>&1 | Out-Null
    Start-Sleep -Seconds 1
}

# ─── Copy binary ───────────────────────────────────────────────────────────
Write-Host "[*] Copying $BinaryPath -> $InstallDir\$AgentExe"
Copy-Item -Path $BinaryPath -Destination "$InstallDir\$AgentExe" -Force

# ─── Register service ──────────────────────────────────────────────────────
$binaryPath = "`"$InstallDir\$AgentExe`""
& sc.exe create $ServiceName binPath= $binaryPath start= auto | Out-Null
& sc.exe description $ServiceName "ApexRMM Remote Monitoring Agent" | Out-Null

# ─── Recovery policy: restart after 5s, restart after 10s, restart after 60s ──
& sc.exe failure $ServiceName reset= 86400 actions= restart/5000/restart/10000/restart/60000 | Out-Null

# ─── Environment variables (SCM reads this for the service process) ────────
$regPath = "HKLM:\SYSTEM\CurrentControlSet\Services\$ServiceName"
$envList = @(
    "BACKEND_URL=$BackendUrl",
    "ENROLL_TOKEN=$EnrollToken"
)
$null = New-ItemProperty -Path $regPath -Name "Environment" -Value $envList -PropertyType MultiString -Force

# ─── Event Log source ──────────────────────────────────────────────────────
# Create the event source if it doesn't already exist. This requires admin.
if (-not [System.Diagnostics.EventLog]::SourceExists($EventSource)) {
    try {
        [System.Diagnostics.EventLog]::CreateEventSource($EventSource, "Application")
        Write-Host "[+] Event source '$EventSource' registered." -ForegroundColor Green
    } catch {
        Write-Host "[!] Could not register event source: $_" -ForegroundColor DarkYellow
    }
}

# ─── Start service ─────────────────────────────────────────────────────────
Write-Host "[*] Starting service '$ServiceName'..." -ForegroundColor Cyan
Start-Service -Name $ServiceName
Start-Sleep -Seconds 3

# ─── Verify ────────────────────────────────────────────────────────────────
$svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($svc.Status -eq "Running") {
    Write-Host "[+] Service '$ServiceName' is running." -ForegroundColor Green
} else {
    Write-Host "[!] Service status: $($svc.Status). Check Event Viewer for details." -ForegroundColor Yellow
}

# Check agent process
$proc = Get-Process -Name "agent" -ErrorAction SilentlyContinue
if ($proc) {
    Write-Host "[+] Agent process running (PID $($proc.Id))." -ForegroundColor Green
} else {
    Write-Host "[!] Agent process not found. Check $InstallDir\queue.db for enrollment status." -ForegroundColor Yellow
}

Write-Host "[+] Installation complete." -ForegroundColor Green
Write-Host "    Service : $ServiceName"
Write-Host "    Binary  : $InstallDir\$AgentExe"
Write-Host "    Backend : $BackendUrl"
Write-Host "    Logs    : Event Viewer -> Windows Logs -> Application (source: $EventSource)"
