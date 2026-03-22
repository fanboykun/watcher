# ==============================================================
# check.ps1
# Verify watcher installation and dependencies
# Run as Administrator
# ==============================================================

$Config = @{
    InstallDir  = "C:\apps\watcher"
    WatcherExe  = "C:\apps\watcher\watcher.exe"
    EnvFile     = "C:\apps\watcher\.env"
    LogDir      = "C:\apps\watcher\logs"
    DBPath      = "C:\apps\watcher\watcher.db"
    NssmPath    = "C:\ProgramData\chocolatey\bin\nssm.exe"
    ServiceName = "app-watcher"
    APIPort     = "8080"
}

$allGood = $true

function Write-OK   { param($msg) Write-Host "  OK: $msg" -ForegroundColor Green }
function Write-Skip { param($msg) Write-Host "  SKIP: $msg" -ForegroundColor Yellow }
function Write-Fail { param($msg) Write-Host "  FAIL: $msg" -ForegroundColor Red; $script:allGood = $false }
function Write-Info { param($msg) Write-Host "  INFO: $msg" -ForegroundColor Gray }

$isServer = (Get-CimInstance Win32_OperatingSystem).ProductType -ne 1
$osLabel = if ($isServer) { "Windows Server" } else { "Windows Desktop" }

Write-Host "`n============================================================" -ForegroundColor Cyan
Write-Host "  WATCHER READINESS CHECK" -ForegroundColor Cyan
Write-Host "  OS: $osLabel" -ForegroundColor Gray
Write-Host "============================================================" -ForegroundColor Cyan


# ── 1. Chocolatey ─────────────────────────────────────────────
Write-Host "`n[1] Chocolatey" -ForegroundColor Yellow
$choco = Get-Command choco -ErrorAction SilentlyContinue
if ($choco) { Write-OK "Installed ($(choco --version))" }
else { Write-Info "Not installed (only needed for NSSM/IIS/ARR features)" }


# ── 2. NSSM ──────────────────────────────────────────────────
Write-Host "`n[2] NSSM" -ForegroundColor Yellow
if (Test-Path $Config.NssmPath) { Write-OK "Found at $($Config.NssmPath)" }
else { Write-Info "Not installed (only needed for binary service management)" }


# ── 3. IIS ────────────────────────────────────────────────────
Write-Host "`n[3] IIS" -ForegroundColor Yellow
try {
    Import-Module WebAdministration -ErrorAction Stop
    Write-OK "IIS is available (WebAdministration module loaded)"

    # Check URL Rewrite
    if (Test-Path "C:\Windows\System32\inetsrv\rewrite.dll") {
        Write-OK "URL Rewrite installed"
    } else {
        Write-Info "URL Rewrite not installed"
    }

    # Check ARR
    if (Test-Path "C:\Windows\System32\inetsrv\arr.dll") {
        Write-OK "ARR installed"

        $arrEnabled = Get-WebConfigurationProperty `
            -PSPath "MACHINE/WEBROOT/APPHOST" `
            -Filter "system.webServer/proxy" `
            -Name "enabled" -ErrorAction SilentlyContinue
        if ($arrEnabled.Value -eq $true) { Write-OK "ARR proxy enabled" }
        else { Write-Info "ARR proxy not enabled" }
    } else {
        Write-Info "ARR not installed (only needed for reverse proxy)"
    }
} catch {
    Write-Info "IIS not installed (only needed for static site or ARR features)"
}


# ── 4. Watcher directories ───────────────────────────────────
Write-Host "`n[4] Directories" -ForegroundColor Yellow
@($Config.InstallDir, $Config.LogDir) | ForEach-Object {
    if (Test-Path $_) { Write-OK "$_ exists" } else { Write-Fail "$_ missing" }
}


# ── 5. Watcher binary ────────────────────────────────────────
Write-Host "`n[5] Watcher Binary" -ForegroundColor Yellow
if (Test-Path $Config.WatcherExe) { Write-OK "watcher.exe found" } else { Write-Fail "watcher.exe NOT found at $($Config.WatcherExe)" }


# ── 6. .env file ─────────────────────────────────────────────
Write-Host "`n[6] Config (.env)" -ForegroundColor Yellow
if (Test-Path $Config.EnvFile) { Write-OK ".env exists" } else { Write-Fail ".env NOT found at $($Config.EnvFile)" }


# ── 7. Database ──────────────────────────────────────────────
Write-Host "`n[7] Database" -ForegroundColor Yellow
if (Test-Path $Config.DBPath) { Write-OK "watcher.db exists" } else { Write-Info "watcher.db not yet created (will be created on first run)" }


# ── 8. Watcher service ───────────────────────────────────────
Write-Host "`n[8] Watcher Service" -ForegroundColor Yellow
$svc = Get-Service $Config.ServiceName -ErrorAction SilentlyContinue
if ($svc) {
    if ($svc.Status -eq "Running") { Write-OK "$($Config.ServiceName) is running" }
    else { Write-Fail "$($Config.ServiceName) status: $($svc.Status)" }
} else {
    Write-Info "$($Config.ServiceName) not registered (NSSM may not be installed)"
}


# ── 9. Outbound HTTPS ────────────────────────────────────────
Write-Host "`n[9] Outbound HTTPS" -ForegroundColor Yellow
try {
    $resp = Invoke-WebRequest -Uri "https://github.com" -UseBasicParsing -TimeoutSec 10
    if ($resp.StatusCode -eq 200) { Write-OK "github.com reachable" }
    else { Write-Fail "github.com returned HTTP $($resp.StatusCode)" }
} catch {
    Write-Fail "Cannot reach github.com"
}


# ── 10. API health ───────────────────────────────────────────
Write-Host "`n[10] API Health" -ForegroundColor Yellow
try {
    $resp = Invoke-WebRequest -Uri "http://localhost:$($Config.APIPort)/api/status" -UseBasicParsing -TimeoutSec 5
    Write-OK "API responding (HTTP $($resp.StatusCode))"
} catch {
    Write-Info "API not responding (watcher may not be running)"
}


# ── Result ────────────────────────────────────────────────────
Write-Host "`n============================================================" -ForegroundColor Cyan
if ($allGood) {
    Write-Host "  ALL CHECKS PASSED" -ForegroundColor Green
} else {
    Write-Host "  SOME CHECKS FAILED — fix the items above" -ForegroundColor Red
}
Write-Host "============================================================`n" -ForegroundColor Cyan