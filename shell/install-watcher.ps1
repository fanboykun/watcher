# ==============================================================
# install-watcher.ps1
# First-time bootstrap: installs watcher.exe as a Windows service
# Copy watcher.exe + config.json to InstallDir, then run this script
# Run as Administrator
# ==============================================================

# ==============================================================
# CONFIGURATION
# ==============================================================
$Config = @{
    ServiceName  = "app-watcher"
    InstallDir   = "C:\apps\watcher"
    WatcherExe   = "C:\apps\watcher\watcher.exe"
    ConfigFile   = "C:\apps\watcher\config.json"
    LogDir       = "C:\apps\watcher\logs"
    NssmPath     = "C:\ProgramData\chocolatey\bin\nssm.exe"
    RestartDelay = 5000   # ms before NSSM restarts on crash
}
# ==============================================================

$ErrorActionPreference = 'Stop'

function Write-Step { param($msg) Write-Host "`n>>> $msg" -ForegroundColor Cyan }
function Write-OK   { param($msg) Write-Host "  OK: $msg" -ForegroundColor Green }
function Write-Skip { param($msg) Write-Host "  SKIP: $msg" -ForegroundColor Yellow }
function Write-Fail { param($msg) Write-Host "  FAIL: $msg" -ForegroundColor Red }

if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Fail "Run as Administrator"
    exit 1
}

Write-Host "`n============================================================" -ForegroundColor Cyan
Write-Host "  WATCHER BOOTSTRAP" -ForegroundColor Cyan
Write-Host "  Service : $($Config.ServiceName)" -ForegroundColor Gray
Write-Host "  Dir     : $($Config.InstallDir)" -ForegroundColor Gray
Write-Host "============================================================`n" -ForegroundColor Cyan


# ── Preflight checks ──────────────────────────────────────────────────────────
Write-Step "Preflight checks"

if (-not (Test-Path $Config.WatcherExe)) {
    Write-Fail "watcher.exe not found at $($Config.WatcherExe)"
    Write-Host "  Copy watcher.exe here before running this script" -ForegroundColor Yellow
    exit 1
}
Write-OK "watcher.exe found"

if (-not (Test-Path $Config.ConfigFile)) {
    Write-Fail "config.json not found at $($Config.ConfigFile)"
    Write-Host "  Copy and fill in config.json here before running this script" -ForegroundColor Yellow
    exit 1
}
Write-OK "config.json found"

if (-not (Test-Path $Config.NssmPath)) {
    Write-Fail "NSSM not found at $($Config.NssmPath)"
    Write-Host "  Run: choco install nssm" -ForegroundColor Yellow
    exit 1
}
Write-OK "NSSM found"


# ── Create directories ────────────────────────────────────────────────────────
Write-Step "Creating directories"

@($Config.InstallDir, $Config.LogDir) | ForEach-Object {
    if (Test-Path $_) { Write-Skip $_ }
    else {
        New-Item -ItemType Directory -Path $_ -Force | Out-Null
        Write-OK "Created $_"
    }
}


# ── Install or update NSSM service ───────────────────────────────────────────
Write-Step "Configuring NSSM service: $($Config.ServiceName)"

$existing = Get-Service $Config.ServiceName -ErrorAction SilentlyContinue

if ($existing) {
    if ($existing.Status -eq "Running") {
        Write-Host "  Stopping existing service..." -ForegroundColor Yellow
        & $Config.NssmPath stop $Config.ServiceName confirm
        Start-Sleep 3
    }
    Write-Host "  Updating existing service"
    & $Config.NssmPath set $Config.ServiceName Application $Config.WatcherExe
} else {
    Write-Host "  Installing new service"
    & $Config.NssmPath install $Config.ServiceName $Config.WatcherExe
}

& $Config.NssmPath set $Config.ServiceName AppParameters   "-config `"$($Config.ConfigFile)`""
& $Config.NssmPath set $Config.ServiceName AppDirectory    $Config.InstallDir
& $Config.NssmPath set $Config.ServiceName Start           SERVICE_AUTO_START
& $Config.NssmPath set $Config.ServiceName AppStdout       "$($Config.LogDir)\watcher.out.log"
& $Config.NssmPath set $Config.ServiceName AppStderr       "$($Config.LogDir)\watcher.err.log"
& $Config.NssmPath set $Config.ServiceName AppRotateFiles  1
& $Config.NssmPath set $Config.ServiceName AppRotateOnline 1
& $Config.NssmPath set $Config.ServiceName AppRotateSeconds 86400
& $Config.NssmPath set $Config.ServiceName AppRestartDelay $Config.RestartDelay

Write-OK "NSSM service configured"


# ── Start service ─────────────────────────────────────────────────────────────
Write-Step "Starting $($Config.ServiceName)"

& $Config.NssmPath start $Config.ServiceName
Start-Sleep 4

$svc = Get-Service $Config.ServiceName -ErrorAction SilentlyContinue
if ($svc -and $svc.Status -eq "Running") {
    Write-OK "Service is running"
} else {
    Write-Fail "Service did not start — check logs at $($Config.LogDir)"
    exit 1
}


# ── Done ──────────────────────────────────────────────────────────────────────
Write-Host "`n============================================================" -ForegroundColor Cyan
Write-Host "  WATCHER INSTALLED SUCCESSFULLY" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Logs    : $($Config.LogDir)\watcher.out.log" -ForegroundColor Yellow
Write-Host "Config  : $($Config.ConfigFile)" -ForegroundColor Yellow
Write-Host ""
Write-Host "Useful commands:" -ForegroundColor Yellow
Write-Host "  Status    : Get-Service $($Config.ServiceName)"
Write-Host "  Stop      : nssm stop $($Config.ServiceName)"
Write-Host "  Start     : nssm start $($Config.ServiceName)"
Write-Host "  Uninstall : nssm remove $($Config.ServiceName) confirm"
Write-Host ""