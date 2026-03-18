# ==============================================================
# 1-setup-server.ps1
# Base Windows Server setup: OpenSSH, Chocolatey, NSSM
# Run as Administrator on Windows Server 2022
# ==============================================================

# ==============================================================
# CONFIGURATION — edit this block to reuse for any project
# ==============================================================

$Config = @{
    # SSH
    SshPort         = 8195                                          # SSH port for both jump host and Windows server

    # Paths
    AppBaseDir      = "C:\admin-be"                                 # Base directory for app files

    # Chocolatey packages to install
    ChocoPackages   = @("nssm")                                     # Add more packages here if needed
}

# ==============================================================

$ErrorActionPreference = 'Stop'

function Write-Step { param($msg) Write-Host "`n>>> $msg" -ForegroundColor Cyan }
function Write-OK   { param($msg) Write-Host "  OK: $msg" -ForegroundColor Green }
function Write-Skip { param($msg) Write-Host "  SKIP: $msg (already done)" -ForegroundColor Yellow }
function Write-Fail { param($msg) Write-Host "  FAIL: $msg" -ForegroundColor Red }

if (-not ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Fail "Please run this script as Administrator"
    exit 1
}

Write-Host "`n============================================================" -ForegroundColor Cyan
Write-Host "  SERVER BASE SETUP" -ForegroundColor Cyan
Write-Host "  SSH Port   : $($Config.SshPort)" -ForegroundColor Gray
Write-Host "  App Dir    : $($Config.AppBaseDir)" -ForegroundColor Gray
Write-Host "============================================================" -ForegroundColor Cyan


# ── 1. OpenSSH Server ─────────────────────────────────────────────────────────
Write-Step "Installing OpenSSH Server"

$sshCap = Get-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0
if ($sshCap.State -eq "Installed") {
    Write-Skip "OpenSSH Server already installed"
} else {
    Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0
    Write-OK "OpenSSH Server installed"
}


# ── 2. sshd service ───────────────────────────────────────────────────────────
Write-Step "Configuring sshd service"

$sshd = Get-Service -Name sshd -ErrorAction SilentlyContinue
if ($sshd.Status -ne "Running") {
    Start-Service sshd
    Write-OK "sshd started"
} else {
    Write-Skip "sshd already running"
}

if ($sshd.StartType -ne "Automatic") {
    Set-Service -Name sshd -StartupType Automatic
    Write-OK "sshd set to Automatic"
} else {
    Write-Skip "sshd already Automatic"
}


# ── 3. sshd_config ────────────────────────────────────────────────────────────
Write-Step "Setting SSH port to $($Config.SshPort) in sshd_config"

$sshdConfig = "C:\ProgramData\ssh\sshd_config"
if (-not (Test-Path $sshdConfig)) {
    Write-Fail "sshd_config not found"
    exit 1
}

$content = Get-Content $sshdConfig -Raw

if ($content -match "(?m)^#?Port\s+\d+") {
    $content = $content -replace "(?m)^#?Port\s+\d+", "Port $($Config.SshPort)"
    Write-OK "Port updated to $($Config.SshPort)"
} else {
    $content = "Port $($Config.SshPort)`n" + $content
    Write-OK "Port $($Config.SshPort) added"
}

if ($content -match "(?m)^#?AuthorizedKeysFile\s+__PROGRAMDATA__/ssh/administrators_authorized_keys") {
    $content = $content -replace "(?m)^#?AuthorizedKeysFile\s+__PROGRAMDATA__/ssh/administrators_authorized_keys", "AuthorizedKeysFile __PROGRAMDATA__/ssh/administrators_authorized_keys"
    Write-OK "AuthorizedKeysFile uncommented"
}

Set-Content -Path $sshdConfig -Value $content -Encoding UTF8


# ── 4. authorized_keys ────────────────────────────────────────────────────────
Write-Step "Setting up administrators_authorized_keys"

$authKeys = "C:\ProgramData\ssh\administrators_authorized_keys"
if (-not (Test-Path $authKeys)) {
    New-Item -Path $authKeys -ItemType File -Force | Out-Null
    Write-OK "File created"
} else {
    Write-Skip "File already exists"
}

icacls $authKeys /inheritance:r | Out-Null
icacls $authKeys /grant "SYSTEM:(F)"                | Out-Null
icacls $authKeys /grant "BUILTIN\Administrators:(F)" | Out-Null
Write-OK "Permissions set"

$keyLines = Get-Content $authKeys | Where-Object { $_.Trim() -ne "" }
if ($keyLines.Count -eq 0) {
    Write-Host ""
    Write-Host "  !! ACTION REQUIRED: Paste your GitHub Actions public SSH key into:" -ForegroundColor Red
    Write-Host "     $authKeys" -ForegroundColor Yellow
}  else {
    Write-OK "$($keyLines.Count) key(s) present"
}


# ── 5. Firewall ───────────────────────────────────────────────────────────────
Write-Step "Opening firewall port $($Config.SshPort)"

$ruleName = "OpenSSH-$($Config.SshPort)"
$fwRule = Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue
if ($fwRule) {
    Write-Skip "Firewall rule $ruleName already exists"
} else {
    New-NetFirewallRule `
        -Name        $ruleName `
        -DisplayName $ruleName `
        -Enabled     True `
        -Direction   Inbound `
        -Protocol    TCP `
        -Action      Allow `
        -LocalPort   $Config.SshPort | Out-Null
    Write-OK "Firewall rule created for port $($Config.SshPort)"
}


# ── 6. Restart sshd ───────────────────────────────────────────────────────────
Write-Step "Restarting sshd to apply config"
Restart-Service sshd
Write-OK "sshd restarted"


# ── 7. Chocolatey ─────────────────────────────────────────────────────────────
Write-Step "Installing Chocolatey"

$choco = Get-Command choco -ErrorAction SilentlyContinue
if ($choco) {
    Write-Skip "Chocolatey already installed (version $(choco --version))"
} else {
    Set-ExecutionPolicy Bypass -Scope Process -Force
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
    iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
    Write-OK "Chocolatey installed"
}


# ── 8. Choco packages ─────────────────────────────────────────────────────────
Write-Step "Installing Chocolatey packages"

foreach ($pkg in $Config.ChocoPackages) {
    $installed = choco list --local-only $pkg 2>$null | Where-Object { $_ -match "^$pkg " }
    if ($installed) {
        Write-Skip "$pkg already installed"
    } else {
        choco install $pkg -y --force
        Write-OK "$pkg installed"
    }
}


# ── 9. App directories ────────────────────────────────────────────────────────
Write-Step "Creating app directories"

@($Config.AppBaseDir, "$($Config.AppBaseDir)\logs") | ForEach-Object {
    if (Test-Path $_) { Write-Skip "$_ exists" }
    else {
        New-Item -ItemType Directory -Path $_ -Force | Out-Null
        Write-OK "Created $_"
    }
}


# ── Done ──────────────────────────────────────────────────────────────────────
Write-Host "`n============================================================" -ForegroundColor Cyan
Write-Host "  BASE SETUP COMPLETE" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Next: run 2-setup-iis-arr.ps1" -ForegroundColor Yellow
Write-Host ""