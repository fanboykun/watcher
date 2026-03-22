# ==============================================================
# install-watcher.ps1
# Interactive bootstrap: installs dependencies based on what
# features the user wants, then registers the watcher service.
# Run as Administrator on Windows 10/11 or Windows Server 2022.
# ==============================================================

$ErrorActionPreference = 'Stop'

# ==============================================================
# CONFIGURATION — edit this block before running
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

# ── Helpers ───────────────────────────────────────────────────
function Write-Step { param($msg) Write-Host "`n>>> $msg" -ForegroundColor Cyan }
function Write-OK   { param($msg) Write-Host "  OK: $msg" -ForegroundColor Green }
function Write-Skip { param($msg) Write-Host "  SKIP: $msg (already done)" -ForegroundColor Yellow }
function Write-Fail { param($msg) Write-Host "  FAIL: $msg" -ForegroundColor Red }

# ── Admin check ───────────────────────────────────────────────
if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Fail "Run as Administrator"
    exit 1
}

# ── Detect OS type ────────────────────────────────────────────
$isServer = (Get-CimInstance Win32_OperatingSystem).ProductType -ne 1
$osLabel = if ($isServer) { "Windows Server" } else { "Windows Desktop" }

# ==============================================================
# INTERACTIVE MENU
# ==============================================================
Write-Host ""
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "  WATCHER BOOTSTRAP" -ForegroundColor Cyan
Write-Host "  OS: $osLabel" -ForegroundColor Gray
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "What will this watcher manage?" -ForegroundColor Yellow
Write-Host ""
Write-Host "  [1] Binary services only (NSSM)" -ForegroundColor White
Write-Host "      Installs: Chocolatey, NSSM" -ForegroundColor Gray
Write-Host "      For: Go APIs, background workers, any .exe" -ForegroundColor Gray
Write-Host ""
Write-Host "  [2] Static sites only (IIS)" -ForegroundColor White
Write-Host "      Installs: IIS features, URL Rewrite" -ForegroundColor Gray
Write-Host "      For: SvelteKit builds, React apps, docs" -ForegroundColor Gray
Write-Host ""
Write-Host "  [3] Both binaries + static sites" -ForegroundColor White
Write-Host "      Installs: Chocolatey, NSSM, IIS, URL Rewrite" -ForegroundColor Gray
Write-Host ""
Write-Host "  [4] Full stack (binaries + IIS + ARR reverse proxy)" -ForegroundColor White
Write-Host "      Installs: Everything above + ARR" -ForegroundColor Gray
Write-Host "      For: IIS as front door proxying to NSSM backends" -ForegroundColor Gray
Write-Host ""

$choice = Read-Host "Select [1-4]"
while ($choice -notin @("1","2","3","4")) {
    $choice = Read-Host "Invalid choice. Select [1-4]"
}

$installNSSM = $choice -in @("1","3","4")
$installIIS  = $choice -in @("2","3","4")
$installARR  = $choice -eq "4"

Write-Host ""
Write-Host "  Install plan:" -ForegroundColor Yellow
if ($installNSSM) { Write-Host "    + Chocolatey + NSSM" -ForegroundColor Green }
if ($installIIS)  { Write-Host "    + IIS features + URL Rewrite" -ForegroundColor Green }
if ($installARR)  { Write-Host "    + ARR (Application Request Routing)" -ForegroundColor Green }
Write-Host "    + Watcher agent service" -ForegroundColor Green
Write-Host ""

# ==============================================================
# STEP 1: Chocolatey (needed for NSSM, URL Rewrite, ARR)
# ==============================================================
$needChoco = $installNSSM -or $installIIS -or $installARR

if ($needChoco) {
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
} else {
    Write-Step "[1] Chocolatey — skipped (not needed)"
}


# ==============================================================
# STEP 2: NSSM (optional — for binary services)
# ==============================================================
if ($installNSSM) {
    Write-Step "[2] NSSM"
    if (Test-Path $Config.NssmPath) {
        Write-Skip "NSSM already installed at $($Config.NssmPath)"
    } else {
        Write-Host "  Installing NSSM via Chocolatey..." -ForegroundColor Yellow
        choco install nssm -y --force
        if (-not (Test-Path $Config.NssmPath)) {
            Write-Fail "NSSM installation failed — not found at $($Config.NssmPath)"
            exit 1
        }
        Write-OK "NSSM installed"
    }
} else {
    Write-Step "[2] NSSM — skipped (not selected)"
}


# ==============================================================
# STEP 3: IIS features (optional — for static sites or ARR)
# ==============================================================
if ($installIIS) {
    Write-Step "[3] IIS features"

    if ($isServer) {
        # Windows Server: use Install-WindowsFeature
        $iisFeatures = @(
            "Web-Server", "Web-WebServer", "Web-Common-Http", "Web-Default-Doc",
            "Web-Static-Content", "Web-Http-Errors", "Web-Http-Redirect",
            "Web-Health", "Web-Http-Logging", "Web-Request-Monitor", "Web-Http-Tracing",
            "Web-Performance", "Web-Stat-Compression", "Web-Dyn-Compression",
            "Web-Security", "Web-Filtering", "Web-Mgmt-Tools", "Web-Mgmt-Console",
            "Web-Scripting-Tools"
        )
        foreach ($f in $iisFeatures) {
            if ((Get-WindowsFeature -Name $f).Installed) { Write-Skip $f }
            else {
                Install-WindowsFeature -Name $f | Out-Null
                Write-OK "$f installed"
            }
        }
    } else {
        # Windows 10/11: use Enable-WindowsOptionalFeature
        $iisFeatures = @(
            "IIS-WebServerRole", "IIS-WebServer", "IIS-CommonHttpFeatures",
            "IIS-DefaultDocument", "IIS-StaticContent", "IIS-HttpErrors",
            "IIS-HttpRedirect", "IIS-HealthAndDiagnostics", "IIS-HttpLogging",
            "IIS-RequestMonitor", "IIS-HttpTracing", "IIS-Performance",
            "IIS-HttpCompressionStatic", "IIS-HttpCompressionDynamic",
            "IIS-Security", "IIS-RequestFiltering", "IIS-ManagementConsole",
            "IIS-ManagementScriptingTools"
        )
        foreach ($f in $iisFeatures) {
            $state = Get-WindowsOptionalFeature -Online -FeatureName $f -ErrorAction SilentlyContinue
            if ($state -and $state.State -eq "Enabled") { Write-Skip $f }
            else {
                Enable-WindowsOptionalFeature -Online -FeatureName $f -All -NoRestart | Out-Null
                Write-OK "$f enabled"
            }
        }
    }

    # URL Rewrite (via Chocolatey, works on both)
    Write-Step "[3b] URL Rewrite"
    if (Test-Path "C:\Windows\System32\inetsrv\rewrite.dll") {
        Write-Skip "URL Rewrite already installed"
    } else {
        choco install urlrewrite -y --force
        Write-OK "URL Rewrite installed"
    }
} else {
    Write-Step "[3] IIS — skipped (not selected)"
}


# ==============================================================
# STEP 4: ARR (optional — for reverse proxy)
# ==============================================================
if ($installARR) {
    Write-Step "[4] ARR (Application Request Routing)"

    if (Test-Path "C:\Windows\System32\inetsrv\arr.dll") {
        Write-Skip "ARR already installed"
    } else {
        choco install iis-arr -y --force
        Write-OK "ARR installed"
    }

    # Enable ARR proxy
    Write-Step "[4b] Enabling ARR proxy"
    Import-Module WebAdministration
    $arrEnabled = Get-WebConfigurationProperty `
        -PSPath "MACHINE/WEBROOT/APPHOST" `
        -Filter "system.webServer/proxy" `
        -Name "enabled" -ErrorAction SilentlyContinue

    if ($arrEnabled.Value -eq $true) {
        Write-Skip "ARR proxy already enabled"
    } else {
        Set-WebConfigurationProperty `
            -PSPath "MACHINE/WEBROOT/APPHOST" `
            -Filter "system.webServer/proxy" `
            -Name "enabled" -Value "True"
        Write-OK "ARR proxy enabled"
    }
} else {
    Write-Step "[4] ARR — skipped (not selected)"
}


# ==============================================================
# STEP 5: Create directories
# ==============================================================
Write-Step "[5] Creating directories"

@($Config.InstallDir, $Config.LogDir) | ForEach-Object {
    if (Test-Path $_) { Write-Skip $_ }
    else {
        New-Item -ItemType Directory -Path $_ -Force | Out-Null
        Write-OK "Created $_"
    }
}


# ==============================================================
# STEP 6: Preflight — check watcher.exe
# ==============================================================
Write-Step "[6] Preflight checks"

if (-not (Test-Path $Config.WatcherExe)) {
    Write-Fail "watcher.exe not found at $($Config.WatcherExe)"
    Write-Host "  Copy watcher.exe to $($Config.InstallDir) before running this script" -ForegroundColor Yellow
    exit 1
}
Write-OK "watcher.exe found"


# ==============================================================
# STEP 7: Create .env
# ==============================================================
Write-Step "[7] Environment config (.env)"

if (Test-Path $Config.EnvFile) {
    Write-Skip ".env already exists at $($Config.EnvFile)"
} else {
    Write-Host "  Creating default .env..." -ForegroundColor Yellow
    $nssmLine = if ($installNSSM) { "NSSM_PATH=$($Config.NssmPath)" } else { "# NSSM_PATH= (not installed)" }
    $envContent = @"
ENVIRONMENT=production
GITHUB_TOKEN=
LOG_DIR=$($Config.LogDir)
$nssmLine
DB_PATH=$($Config.DBPath)
API_PORT=$($Config.APIPort)
"@
    Set-Content -Path $Config.EnvFile -Value $envContent -Encoding UTF8
    Write-OK ".env created at $($Config.EnvFile)"
    Write-Host "  Edit .env to set your GITHUB_TOKEN if using private repos" -ForegroundColor Yellow
}


# ==============================================================
# STEP 8: Secure .env permissions
# ==============================================================
Write-Step "[8] Securing .env permissions"

icacls $Config.EnvFile /inheritance:r | Out-Null
icacls $Config.EnvFile /grant "SYSTEM:(F)" | Out-Null
icacls $Config.EnvFile /grant "BUILTIN\Administrators:(F)" | Out-Null
Write-OK ".env restricted to SYSTEM and Administrators only"


# ==============================================================
# STEP 9: Outbound HTTPS check
# ==============================================================
Write-Step "[9] Outbound HTTPS to github.com"

try {
    $resp = Invoke-WebRequest -Uri "https://github.com" -UseBasicParsing -TimeoutSec 10
    if ($resp.StatusCode -eq 200) {
        Write-OK "github.com reachable (HTTP $($resp.StatusCode))"
    } else {
        Write-Fail "github.com returned HTTP $($resp.StatusCode)"
        exit 1
    }
} catch {
    Write-Fail "Cannot reach github.com — check firewall or proxy settings"
    Write-Host "  Error: $_" -ForegroundColor Red
    exit 1
}


# ==============================================================
# STEP 10: Register watcher NSSM service
# ==============================================================
if ($installNSSM) {
    Write-Step "[10] Configuring NSSM service: $($Config.ServiceName)"

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

    # Start service
    Write-Step "[11] Starting $($Config.ServiceName)"
    & $Config.NssmPath start $Config.ServiceName
    Start-Sleep 4

    $svc = Get-Service $Config.ServiceName -ErrorAction SilentlyContinue
    if ($svc -and $svc.Status -eq "Running") {
        Write-OK "Service is running"
    } else {
        Write-Fail "Service did not start — check logs at $($Config.LogDir)"
        exit 1
    }
} else {
    Write-Step "[10] NSSM service registration — skipped (NSSM not selected)"
    Write-Host "  You will need to run watcher.exe manually or register it as a service" -ForegroundColor Yellow
}


# ==============================================================
# STEP 12: Health check
# ==============================================================
Write-Step "[12] Verifying API is responding"

Start-Sleep 2
try {
    $resp = Invoke-WebRequest -Uri "http://localhost:$($Config.APIPort)/api/status" -UseBasicParsing -TimeoutSec 5
    Write-OK "API is up (HTTP $($resp.StatusCode))"
} catch {
    Write-Host "  WARN: API not responding yet — check logs" -ForegroundColor Yellow
}


# ==============================================================
# DONE
# ==============================================================
Write-Host "`n============================================================" -ForegroundColor Cyan
Write-Host "  WATCHER INSTALLED SUCCESSFULLY" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Dashboard  : http://localhost:$($Config.APIPort)" -ForegroundColor Yellow
Write-Host "Logs       : $($Config.LogDir)\watcher.out.log" -ForegroundColor Yellow
Write-Host "Config     : $($Config.EnvFile)" -ForegroundColor Yellow
Write-Host "Database   : $($Config.DBPath)" -ForegroundColor Yellow
Write-Host ""
Write-Host "Installed features:" -ForegroundColor Yellow
if ($installNSSM) { Write-Host "  [x] NSSM (binary service management)" -ForegroundColor Green }
else              { Write-Host "  [ ] NSSM" -ForegroundColor Gray }
if ($installIIS)  { Write-Host "  [x] IIS (static site serving)" -ForegroundColor Green }
else              { Write-Host "  [ ] IIS" -ForegroundColor Gray }
if ($installARR)  { Write-Host "  [x] ARR (reverse proxy)" -ForegroundColor Green }
else              { Write-Host "  [ ] ARR" -ForegroundColor Gray }
Write-Host ""
if ($installNSSM) {
    Write-Host "Commands:" -ForegroundColor Yellow
    Write-Host "  Status    : Get-Service $($Config.ServiceName)"
    Write-Host "  Stop      : nssm stop $($Config.ServiceName)"
    Write-Host "  Start     : nssm start $($Config.ServiceName)"
    Write-Host "  Uninstall : nssm remove $($Config.ServiceName) confirm"
    Write-Host ""
}