# ==============================================================
# 3-setup-services.ps1
# Register NSSM services for rolling deploy
# Run as Administrator on Windows Server 2022
# ==============================================================

# ==============================================================
# CONFIGURATION — edit this block to reuse for any project
# ==============================================================

$Config = @{
    # App identity
    AppName             = "admin-be"            # Prefix for all service names

    # Paths
    BaseDir             = "C:\admin-be"         # Must match WIN_BASE_DIR in workflow
    WebBinary           = "web.exe"             # Must match WEB_BINARY in workflow
    WorkerBinary        = "worker.exe"          # Must match WORKER_BINARY in workflow

    # Instances — must match workflow WEB_INSTANCE_COUNT / WORKER_INSTANCE_COUNT
    WebInstanceCount    = 2
    WorkerInstanceCount = 2

    # Ports — first web instance gets WebBasePort, next gets WebBasePort+1, etc.
    # Must match WEB_BASE_PORT in workflow
    WebBasePort         = 8000

    # NSSM path
    NssmPath            = "C:\ProgramData\chocolatey\bin\nssm.exe"

    # Log rotation
    RotateFiles         = 1
    RotateOnline        = 1
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
Write-Host "  NSSM SERVICES SETUP" -ForegroundColor Cyan
Write-Host "  App        : $($Config.AppName)" -ForegroundColor Gray
Write-Host "  Base Dir   : $($Config.BaseDir)" -ForegroundColor Gray
Write-Host "  Web        : $($Config.WebInstanceCount) instance(s) starting at port $($Config.WebBasePort)" -ForegroundColor Gray
Write-Host "  Worker     : $($Config.WorkerInstanceCount) instance(s)" -ForegroundColor Gray
Write-Host "============================================================" -ForegroundColor Cyan


# ── 1. Directories ────────────────────────────────────────────────────────────
Write-Step "Creating directories"

@($Config.BaseDir, "$($Config.BaseDir)\logs") | ForEach-Object {
    if (Test-Path $_) { Write-Skip "$_ exists" }
    else {
        New-Item -ItemType Directory -Path $_ -Force | Out-Null
        Write-OK "Created $_"
    }
}


# ── 2. NSSM check ─────────────────────────────────────────────────────────────
Write-Step "Checking NSSM"

if (-not (Test-Path $Config.NssmPath)) {
    Write-Fail "NSSM not found at $($Config.NssmPath). Run 1-setup-server.ps1 first."
    exit 1
}
Write-OK "NSSM found"


# ── 3. Build service list dynamically ─────────────────────────────────────────
$Services = @()

for ($i = 1; $i -le $Config.WebInstanceCount; $i++) {
    $Services += @{
        Name    = "$($Config.AppName)-web-$i"
        Exe     = "$($Config.BaseDir)\$($Config.WebBinary)"
        EnvFile = "$($Config.BaseDir)\.env.web.$i"
        Port    = $Config.WebBasePort + ($i - 1)
        Type    = "web"
    }
}

for ($i = 1; $i -le $Config.WorkerInstanceCount; $i++) {
    $Services += @{
        Name    = "$($Config.AppName)-worker-$i"
        Exe     = "$($Config.BaseDir)\$($Config.WorkerBinary)"
        EnvFile = "$($Config.BaseDir)\.env.worker.$i"
        Port    = $null
        Type    = "worker"
    }
}


# ── 4. Create .env placeholders ───────────────────────────────────────────────
Write-Step "Creating .env placeholder files"

$missingEnv = @()
foreach ($svc in $Services) {
    if (Test-Path $svc.EnvFile) {
        Write-Skip "$($svc.EnvFile) exists"
    } else {
        $portLine = if ($svc.Port) { "`nPORT=$($svc.Port)" } else { "" }
        Set-Content -Path $svc.EnvFile -Value "# $($svc.Name) environment variables$portLine"
        Write-OK "Created $($svc.EnvFile)"
        $missingEnv += $svc.EnvFile
    }
}

if ($missingEnv.Count -gt 0) {
    Write-Host ""
    Write-Host "  !! ACTION REQUIRED: Fill in these .env files before starting services:" -ForegroundColor Red
    foreach ($f in $missingEnv) { Write-Host "     $f" -ForegroundColor Yellow }
}


# ── 5. Create placeholder binaries ────────────────────────────────────────────
Write-Step "Checking binaries"

@(
    "$($Config.BaseDir)\$($Config.WebBinary)",
    "$($Config.BaseDir)\$($Config.WorkerBinary)"
) | ForEach-Object {
    if (Test-Path $_) {
        Write-Skip "$_ exists"
    } else {
        [System.IO.File]::WriteAllBytes($_, @())
        Write-OK "Created empty placeholder: $_ (will be replaced on first deploy)"
    }
}


# ── 6. Register NSSM services ─────────────────────────────────────────────────
Write-Step "Registering NSSM services"

foreach ($svc in $Services) {
    $existing = Get-Service $svc.Name -ErrorAction SilentlyContinue
    if ($existing) {
        Write-Skip "$($svc.Name) already registered"
    } else {
        & $Config.NssmPath install $svc.Name $svc.Exe
        & $Config.NssmPath set $svc.Name AppDirectory    $Config.BaseDir
        & $Config.NssmPath set $svc.Name AppEnvExtra     "ENV_FILE=$($svc.EnvFile)"
        & $Config.NssmPath set $svc.Name Start           SERVICE_AUTO_START
        & $Config.NssmPath set $svc.Name AppStdout       "$($Config.BaseDir)\logs\$($svc.Name).out.log"
        & $Config.NssmPath set $svc.Name AppStderr       "$($Config.BaseDir)\logs\$($svc.Name).err.log"
        & $Config.NssmPath set $svc.Name AppRotateFiles  $Config.RotateFiles
        & $Config.NssmPath set $svc.Name AppRotateOnline $Config.RotateOnline
        Write-OK "Registered: $($svc.Name)"
    }
}


# ── Done ──────────────────────────────────────────────────────────────────────
Write-Host "`n============================================================" -ForegroundColor Cyan
Write-Host "  SERVICES SETUP COMPLETE" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Registered services:" -ForegroundColor Yellow
foreach ($svc in $Services) {
    $portStr = if ($svc.Port) { " (port $($svc.Port))" } else { "" }
    Write-Host "  $($svc.Name)$portStr -> $($svc.Exe)"
}
Write-Host ""
Write-Host "Next steps:" -ForegroundColor Yellow
Write-Host "  1. Fill in all .env files in $($Config.BaseDir)"
Write-Host "  2. Push to main branch to trigger first deployment"
Write-Host "  3. Services will start automatically after deploy"
Write-Host ""