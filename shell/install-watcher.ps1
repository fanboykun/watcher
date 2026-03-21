# install-watcher.ps1
# Full bootstrap: installs Chocolatey, NSSM, creates .env config,
# then registers watcher.exe as a Windows service.
# Run as Administrator.

# ==============================================================
# CONFIGURATION -- edit this block before running
# ==============================================================
$Config = @{
    ServiceName  = "app-watcher"
    InstallDir   = "C:\apps\watcher"
    WatcherExe   = "C:\apps\watcher\watcher.exe"
    EnvFile      = "C:\apps\watcher\.env"
    LogDir       = "C:\apps\watcher\logs"
    NssmPath     = "C:\ProgramData\chocolatey\bin\nssm.exe"
    DBPath       = "C:\apps\watcher\watcher.db"
    APIPort      = "8080"
    RestartDelay = 5000
}
# ==============================================================

$ErrorActionPreference = 'Stop'

function Write-Step { param($msg) Write-Host "`n>>> $msg" -ForegroundColor Cyan }
function Write-OK   { param($msg) Write-Host "  OK: $msg" -ForegroundColor Green }
function Write-Skip { param($msg) Write-Host "  SKIP: $msg (already done)" -ForegroundColor Yellow }
function Write-Fail { param($msg) Write-Host "  FAIL: $msg" -ForegroundColor Red }

if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Fail "Run as Administrator"
    exit 1
}

Write-Host "`n============================================================" -ForegroundColor Cyan
Write-Host "  WATCHER BOOTSTRAP" -ForegroundColor Cyan
Write-Host "  Service : $($Config.ServiceName)" -ForegroundColor Gray
Write-Host "  Dir     : $($Config.InstallDir)" -ForegroundColor Gray
Write-Host "  API     : http://localhost:$($Config.APIPort)" -ForegroundColor Gray
Write-Host "============================================================`n" -ForegroundColor Cyan


# [1] Chocolatey
Write-Step "[1] Chocolatey"

$choco = Get-Command choco -ErrorAction SilentlyContinue
if ($choco) {
    Write-Skip "Chocolatey already installed (version $(choco --version))"
} else {
    Write-Host "  Installing Chocolatey..." -ForegroundColor Yellow
    Set-ExecutionPolicy Bypass -Scope Process -Force
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
    iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))

    $env:PATH = [System.Environment]::GetEnvironmentVariable("PATH", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("PATH", "User")

    if (-not (Get-Command choco -ErrorAction SilentlyContinue)) {
        Write-Fail "Chocolatey installation failed"
        exit 1
    }
    Write-OK "Chocolatey installed (version $(choco --version))"
}


# [2] NSSM
Write-Step "[2] NSSM"

if (Test-Path $Config.NssmPath) {
    Write-Skip "NSSM already installed at $($Config.NssmPath)"
} else {
    Write-Host "  Installing NSSM via Chocolatey..." -ForegroundColor Yellow
    choco install nssm -y --force

    if (-not (Test-Path $Config.NssmPath)) {
        Write-Fail "NSSM installation failed -- not found at $($Config.NssmPath)"
        exit 1
    }
    Write-OK "NSSM installed"
}


# [3] Create directories
Write-Step "[3] Creating directories"

@($Config.InstallDir, $Config.LogDir) | ForEach-Object {
    if (Test-Path $_) {
        Write-Skip $_
    } else {
        New-Item -ItemType Directory -Path $_ -Force | Out-Null
        Write-OK "Created $_"
    }
}


# [4] Preflight: watcher.exe
Write-Step "[4] Preflight checks"

if (-not (Test-Path $Config.WatcherExe)) {
    Write-Fail "watcher.exe not found at $($Config.WatcherExe)"
    Write-Host "  Copy watcher.exe to $($Config.InstallDir) before running this script" -ForegroundColor Yellow
    exit 1
}
Write-OK "watcher.exe found"


# [5] Create .env if it doesn't exist
Write-Step "[5] Environment config (.env)"

if (Test-Path $Config.EnvFile) {
    Write-Skip ".env already exists at $($Config.EnvFile)"
} else {
    Write-Host "  Creating default .env..." -ForegroundColor Yellow
    $envContent = @"
ENVIRONMENT=production
GITHUB_TOKEN=
LOG_DIR=$($Config.LogDir)
NSSM_PATH=$($Config.NssmPath)
DB_PATH=$($Config.DBPath)
API_PORT=$($Config.APIPort)
"@
    Set-Content -Path $Config.EnvFile -Value $envContent -Encoding UTF8
    Write-OK ".env created at $($Config.EnvFile)"
    Write-Host "  Edit .env to set your GITHUB_TOKEN if using private repos" -ForegroundColor Yellow
}


# [6] Secure .env permissions
Write-Step "[6] Securing .env permissions"

icacls $Config.EnvFile /inheritance:r | Out-Null
icacls $Config.EnvFile /grant "SYSTEM:(F)" | Out-Null
icacls $Config.EnvFile /grant "BUILTIN\Administrators:(F)" | Out-Null
Write-OK ".env restricted to SYSTEM and Administrators only"


# [7] Outbound HTTPS check
Write-Step "[7] Outbound HTTPS to github.com"

try {
    $resp = Invoke-WebRequest -Uri "https://github.com" -UseBasicParsing -TimeoutSec 10
    if ($resp.StatusCode -eq 200) {
        Write-OK "github.com reachable (HTTP $($resp.StatusCode))"
    } else {
        Write-Fail "github.com returned HTTP $($resp.StatusCode)"
        exit 1
    }
} catch {
    Write-Fail "Cannot reach github.com -- check firewall or proxy settings"
    Write-Host "  Error: $_" -ForegroundColor Red
    exit 1
}


# [8] Register NSSM service
Write-Step "[8] Configuring NSSM service: $($Config.ServiceName)"

$existing = Get-Service $Config.ServiceName -ErrorAction SilentlyContinue

if ($existing) {
    if ($existing.Status -eq "Running") {
        Write-Host "  Stopping existing service..." -ForegroundColor Yellow
        & $Config.NssmPath stop $Config.ServiceName confirm
        Start-Sleep 3
    }
    Write-Host "  Updating existing service binary path"
    $out = & $Config.NssmPath set $Config.ServiceName Application `"$($Config.WatcherExe)`" 2>&1
    Write-Host "  NSSM output: $out"
} else {
    Write-Host "  Registering new service: $($Config.ServiceName)"
    $out = & $Config.NssmPath install $Config.ServiceName `"$($Config.WatcherExe)`" 2>&1
    Write-Host "  NSSM output: $out"

    $created = Get-Service $Config.ServiceName -ErrorAction SilentlyContinue
    if (-not $created) {
        Write-Fail "NSSM install ran but service was not created"
        Write-Host "  Try running manually:" -ForegroundColor Yellow
        Write-Host "  $($Config.NssmPath) install $($Config.ServiceName) `"$($Config.WatcherExe)`"" -ForegroundColor Yellow
        exit 1
    }
    Write-OK "Service registered"
}

& $Config.NssmPath set $Config.ServiceName AppParameters   "-config `"$($Config.EnvFile)`"" | Out-Null
& $Config.NssmPath set $Config.ServiceName AppDirectory    $Config.InstallDir | Out-Null
& $Config.NssmPath set $Config.ServiceName Start           SERVICE_AUTO_START | Out-Null
& $Config.NssmPath set $Config.ServiceName AppStdout       "$($Config.LogDir)\watcher.out.log" | Out-Null
& $Config.NssmPath set $Config.ServiceName AppStderr       "$($Config.LogDir)\watcher.err.log" | Out-Null
& $Config.NssmPath set $Config.ServiceName AppRotateFiles  1 | Out-Null
& $Config.NssmPath set $Config.ServiceName AppRotateOnline 1 | Out-Null
& $Config.NssmPath set $Config.ServiceName AppRotateSeconds 86400 | Out-Null
& $Config.NssmPath set $Config.ServiceName AppRestartDelay $Config.RestartDelay | Out-Null
Write-OK "NSSM service configured"


# [9] Start service
Write-Step "[9] Starting $($Config.ServiceName)"

& $Config.NssmPath start $Config.ServiceName
Start-Sleep 4

$svc = Get-Service $Config.ServiceName -ErrorAction SilentlyContinue
if ($svc -and $svc.Status -eq "Running") {
    Write-OK "Service is running"
} else {
    Write-Fail "Service did not start -- check logs at $($Config.LogDir)"
    exit 1
}


# [10] Health check
Write-Step "[10] Verifying API is responding"

Start-Sleep 2
try {
    $resp = Invoke-WebRequest -Uri "http://localhost:$($Config.APIPort)/api/status" -UseBasicParsing -TimeoutSec 5
    Write-OK "API is up (HTTP $($resp.StatusCode))"
} catch {
    Write-Host "  WARN: API not responding yet -- check logs" -ForegroundColor Yellow
}


Write-Host "`n============================================================" -ForegroundColor Cyan
Write-Host "  WATCHER INSTALLED SUCCESSFULLY" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Dashboard : http://localhost:$($Config.APIPort)" -ForegroundColor Yellow
Write-Host "Logs      : $($Config.LogDir)\watcher.out.log" -ForegroundColor Yellow
Write-Host "Config    : $($Config.EnvFile)" -ForegroundColor Yellow
Write-Host "Database  : $($Config.DBPath)" -ForegroundColor Yellow
Write-Host ""
Write-Host "Commands:" -ForegroundColor Yellow
Write-Host "  Status    : Get-Service $($Config.ServiceName)"
Write-Host "  Stop      : nssm stop $($Config.ServiceName)"
Write-Host "  Start     : nssm start $($Config.ServiceName)"
Write-Host "  Uninstall : nssm remove $($Config.ServiceName) confirm"
Write-Host ""