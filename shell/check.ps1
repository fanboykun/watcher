# ==============================================================
# check.ps1
# Verify all services and config are ready for deployment
# Run as Administrator on Windows Server 2022
# ==============================================================

# ==============================================================
# CONFIGURATION — keep in sync with your other setup scripts
# ==============================================================

$Config = @{
    # SSH
    SshPort             = 8195

    # App
    AppName             = "admin-be"
    BaseDir             = "C:\admin-be"
    WebBinary           = "web.exe"
    WorkerBinary        = "worker.exe"
    WebInstanceCount    = 2
    WorkerInstanceCount = 2
    WebBasePort         = 8000

    # IIS
    ProxyPort           = 8080
    SiteName            = "admin-be-proxy"
    FarmName            = "admin-be-farm"

    # NSSM
    NssmPath            = "C:\ProgramData\chocolatey\bin\nssm.exe"
}

# ==============================================================

$allGood = $true

function Write-OK   { param($msg) Write-Host "  OK: $msg" -ForegroundColor Green }
function Write-Skip { param($msg) Write-Host "  SKIP: $msg" -ForegroundColor Yellow }
function Write-Fail { param($msg) Write-Host "  FAIL: $msg" -ForegroundColor Red; $script:allGood = $false }

Write-Host "`n============================================================" -ForegroundColor Cyan
Write-Host "  DEPLOYMENT READINESS CHECK" -ForegroundColor Cyan
Write-Host "  App: $($Config.AppName)" -ForegroundColor Gray
Write-Host "============================================================" -ForegroundColor Cyan


# ── 1. OpenSSH ────────────────────────────────────────────────────────────────
Write-Host "`n[1] OpenSSH Server" -ForegroundColor Yellow
$cap = Get-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0
if ($cap.State -eq "Installed") { Write-OK "Installed" } else { Write-Fail "NOT installed" }


# ── 2. sshd service ───────────────────────────────────────────────────────────
Write-Host "`n[2] sshd Service" -ForegroundColor Yellow
$sshd = Get-Service -Name sshd -ErrorAction SilentlyContinue
if ($sshd) {
    if ($sshd.Status -eq "Running" -and $sshd.StartType -eq "Automatic") {
        Write-OK "Running / Automatic"
    } else {
        Write-Fail "Status=$($sshd.Status) StartType=$($sshd.StartType)"
    }
} else { Write-Fail "Service not found" }


# ── 3. SSH port ───────────────────────────────────────────────────────────────
Write-Host "`n[3] SSH Port $($Config.SshPort)" -ForegroundColor Yellow
$port = netstat -ano | findstr ":$($Config.SshPort)"
if ($port) { Write-OK "Listening on $($Config.SshPort)" } else { Write-Fail "Nothing on port $($Config.SshPort)" }


# ── 4. sshd_config ────────────────────────────────────────────────────────────
Write-Host "`n[4] sshd_config Port Setting" -ForegroundColor Yellow
$sshdConfig = "C:\ProgramData\ssh\sshd_config"
if (Test-Path $sshdConfig) {
    $line = Get-Content $sshdConfig | Where-Object { $_ -match "^Port\s+$($Config.SshPort)" }
    if ($line) { Write-OK "Port $($Config.SshPort) confirmed in sshd_config" } else { Write-Fail "Port $($Config.SshPort) NOT in sshd_config" }
} else { Write-Fail "sshd_config not found" }


# ── 5. authorized_keys ────────────────────────────────────────────────────────
Write-Host "`n[5] administrators_authorized_keys" -ForegroundColor Yellow
$authKeys = "C:\ProgramData\ssh\administrators_authorized_keys"
if (Test-Path $authKeys) {
    $keys = Get-Content $authKeys | Where-Object { $_.Trim() -ne "" }
    if ($keys.Count -gt 0) { Write-OK "$($keys.Count) key(s) present" } else { Write-Fail "File is EMPTY — add your public key" }
} else { Write-Fail "File not found" }


# ── 6. Firewall SSH ───────────────────────────────────────────────────────────
Write-Host "`n[6] Firewall: SSH port $($Config.SshPort)" -ForegroundColor Yellow
$fw = Get-NetFirewallRule | Where-Object { $_.Enabled -eq "True" -and $_.Direction -eq "Inbound" } |
    Get-NetFirewallPortFilter | Where-Object { $_.LocalPort -eq "$($Config.SshPort)" }
if ($fw) { Write-OK "Rule exists" } else { Write-Fail "No inbound rule for port $($Config.SshPort)" }


# ── 7. Firewall IIS ───────────────────────────────────────────────────────────
Write-Host "`n[7] Firewall: IIS port $($Config.ProxyPort)" -ForegroundColor Yellow
$fw2 = Get-NetFirewallRule | Where-Object { $_.Enabled -eq "True" -and $_.Direction -eq "Inbound" } |
    Get-NetFirewallPortFilter | Where-Object { $_.LocalPort -eq "$($Config.ProxyPort)" }
if ($fw2) { Write-OK "Rule exists" } else { Write-Fail "No inbound rule for port $($Config.ProxyPort)" }


# ── 8. Chocolatey ─────────────────────────────────────────────────────────────
Write-Host "`n[8] Chocolatey" -ForegroundColor Yellow
$choco = Get-Command choco -ErrorAction SilentlyContinue
if ($choco) { Write-OK "Installed ($(choco --version))" } else { Write-Fail "Not installed" }


# ── 9. NSSM ───────────────────────────────────────────────────────────────────
Write-Host "`n[9] NSSM" -ForegroundColor Yellow
if (Test-Path $Config.NssmPath) { Write-OK "Found at $($Config.NssmPath)" } else { Write-Fail "Not found at $($Config.NssmPath)" }


# ── 10. IIS + ARR ─────────────────────────────────────────────────────────────
Write-Host "`n[10] IIS Site: $($Config.SiteName)" -ForegroundColor Yellow
try {
    Import-Module WebAdministration -ErrorAction Stop
    $site = Get-Website -Name $Config.SiteName -ErrorAction SilentlyContinue
    if ($site) { Write-OK "Site exists (port $($Config.ProxyPort), status: $($site.State))" } else { Write-Fail "Site not found" }
} catch { Write-Fail "WebAdministration module not available (IIS not installed?)" }


# ── 11. ARR Farm ──────────────────────────────────────────────────────────────
Write-Host "`n[11] ARR Farm: $($Config.FarmName)" -ForegroundColor Yellow
try {
    $farm = Get-WebConfigurationProperty `
        -PSPath "MACHINE/WEBROOT/APPHOST" `
        -Filter "webFarms/webFarm[@name='$($Config.FarmName)']" `
        -Name "name" -ErrorAction SilentlyContinue
    if ($farm) { Write-OK "Farm exists" } else { Write-Fail "Farm not found" }
} catch { Write-Fail "Could not check ARR farm" }


# ── 12. App directories ───────────────────────────────────────────────────────
Write-Host "`n[12] App Directories" -ForegroundColor Yellow
@($Config.BaseDir, "$($Config.BaseDir)\logs") | ForEach-Object {
    if (Test-Path $_) { Write-OK "$_ exists" } else { Write-Fail "$_ missing" }
}


# ── 13. .env files ────────────────────────────────────────────────────────────
Write-Host "`n[13] .env Files" -ForegroundColor Yellow
for ($i = 1; $i -le $Config.WebInstanceCount; $i++) {
    $f = "$($Config.BaseDir)\.env.web.$i"
    if (Test-Path $f) { Write-OK "$f exists" } else { Write-Fail "$f missing" }
}
for ($i = 1; $i -le $Config.WorkerInstanceCount; $i++) {
    $f = "$($Config.BaseDir)\.env.worker.$i"
    if (Test-Path $f) { Write-OK "$f exists" } else { Write-Fail "$f missing" }
}


# ── 14. NSSM services ─────────────────────────────────────────────────────────
Write-Host "`n[14] NSSM Services" -ForegroundColor Yellow
for ($i = 1; $i -le $Config.WebInstanceCount; $i++) {
    $name = "$($Config.AppName)-web-$i"
    $svc = Get-Service $name -ErrorAction SilentlyContinue
    if ($svc) {
        Write-Host "  $name -> $($svc.Status)" -ForegroundColor Cyan
    } else {
        Write-Host "  $name -> not registered yet (will be created on first deploy)" -ForegroundColor Gray
    }
}
for ($i = 1; $i -le $Config.WorkerInstanceCount; $i++) {
    $name = "$($Config.AppName)-worker-$i"
    $svc = Get-Service $name -ErrorAction SilentlyContinue
    if ($svc) {
        Write-Host "  $name -> $($svc.Status)" -ForegroundColor Cyan
    } else {
        Write-Host "  $name -> not registered yet (will be created on first deploy)" -ForegroundColor Gray
    }
}


# ── Result ────────────────────────────────────────────────────────────────────
Write-Host "`n============================================================" -ForegroundColor Cyan
if ($allGood) {
    Write-Host "  ALL CHECKS PASSED - ready to deploy!" -ForegroundColor Green
} else {
    Write-Host "  SOME CHECKS FAILED - fix the items above" -ForegroundColor Red
}
Write-Host "============================================================`n" -ForegroundColor Cyan