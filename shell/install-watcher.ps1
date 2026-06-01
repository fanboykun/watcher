# ==============================================================
# install-watcher.ps1
# Interactive bootstrap: configures installation via an interactive CLI.
# Run as Administrator on Windows 10/11 or Windows Server 2022.
# ==============================================================

param(
    [switch]$Silent,
    [switch]$DebugMode
)

$ErrorActionPreference = "Stop"

# ==============================================================
# DEFAULTS -- used by interactive prompts and silent mode
# ==============================================================
$Defaults = @{
    Profile     = 0 # 0=Binary, 1=Static, 2=Both, 3=Full Stack
    InstallNSSM = $true
    InstallIIS  = $false
    InstallARR  = $false
    ServiceName = "app-watcher"
    InstallDir  = "C:\apps\watcher"
    LogDir      = "C:\apps\watcher\logs"
    NssmPath    = "C:\ProgramData\chocolatey\bin\nssm.exe"
    DBPath      = "C:\apps\watcher\watcher.db"
    APIPort     = "8080"
    GitHubToken = ""
}

$UrlRewriteDll = "C:\Windows\System32\inetsrv\rewrite.dll"
$ArrRouterDll  = "C:\Program Files\IIS\Application Request Routing\requestRouter.dll"
$ScriptPath    = if (-not [string]::IsNullOrWhiteSpace($PSCommandPath)) { $PSCommandPath } else { $MyInvocation.MyCommand.Path }
$ScriptDir     = Split-Path -Parent $ScriptPath
$ParentDir     = Split-Path -Parent $ScriptDir
$Script:IsServer = (Get-CimInstance Win32_OperatingSystem).ProductType -ne 1
$Script:LogPath  = Join-Path $env:TEMP ("watcher-installer-" + (Get-Date -Format "yyyyMMdd-HHmmss") + ".log")

# ==============================================================
# LOGGING
# ==============================================================
function Write-InstallerLog {
    param(
        [string]$Level,
        [string]$Message
    )

    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    $line = "[{0}] [{1}] {2}" -f $timestamp, $Level.ToUpperInvariant(), $Message

    try {
        Add-Content -Path $Script:LogPath -Value $line -Encoding ASCII
    } catch {}

    switch ($Level.ToUpperInvariant()) {
        "ERROR" { Write-Host $line -ForegroundColor Red }
        "WARN"  { Write-Host $line -ForegroundColor Yellow }
        "OK"    { Write-Host $line -ForegroundColor Green }
        "STEP"  { Write-Host $line -ForegroundColor Cyan }
        default { Write-Host $line }
    }
}

function Write-Step { param([string]$Message) Write-InstallerLog -Level "STEP" -Message $Message }
function Write-OK   { param([string]$Message) Write-InstallerLog -Level "OK"   -Message $Message }
function Write-Warn { param([string]$Message) Write-InstallerLog -Level "WARN" -Message $Message }
function Write-Info { param([string]$Message) Write-InstallerLog -Level "INFO" -Message $Message }

function Fail-Install {
    param([string]$Message)
    Write-InstallerLog -Level "ERROR" -Message $Message
    throw $Message
}

# ==============================================================
# HOST / ELEVATION
# ==============================================================
function Test-IsAdministrator {
    $principal = [Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Show-Message {
    param(
        [string]$Text,
        [string]$Title = "Watcher Installer",
        [ValidateSet("Information", "Warning", "Error")]
        [string]$Icon = "Information"
    )

    if ($Silent) {
        return
    }

    $color = switch ($Icon) {
        "Error" { "Red" }
        "Warning" { "Yellow" }
        default { "White" }
    }
    Write-Host ""
    Write-Host $Title -ForegroundColor $color
    Write-Host $Text -ForegroundColor $color
}

if (-not (Test-IsAdministrator)) {
    if ($Silent) {
        Fail-Install "Run as Administrator."
    }
    Show-Message -Text "Please run this installer from an elevated PowerShell or use install.bat." -Icon Error
    exit 1
}

# ==============================================================
# SHARED HELPERS
# ==============================================================
function Invoke-ExternalCommand {
    param(
        [string]$FilePath,
        [string[]]$Arguments,
        [string]$Description
    )

    Write-Info ("Running: {0}" -f $Description)
    $output = & $FilePath @Arguments 2>&1 | Out-String
    $exitCode = $LASTEXITCODE

    if (-not [string]::IsNullOrWhiteSpace($output)) {
        foreach ($line in ($output -split "`r?`n")) {
            if (-not [string]::IsNullOrWhiteSpace($line)) {
                Write-Info ("  {0}" -f $line.TrimEnd())
            }
        }
    }

    if ($exitCode -ne 0) {
        Fail-Install ("Command failed ({0}): exit code {1}" -f $Description, $exitCode)
    }

    return $output
}

function Set-ProgressStep {
    param(
        [int]$Value,
        [string]$Status
    )

    Write-Info ("Progress {0}%: {1}" -f $Value, $Status)
}

function Get-IISFeatureList {
    # Do not install the HTTP Redirect role service by default. Static IIS sites, URL Rewrite, and ARR
    # do not require it, and Web-Http-Redirect can hang on some Windows Server IIS plugin loads.
    if ($Script:IsServer) {
        return @(
            "Web-Server","Web-WebServer","Web-Common-Http","Web-Default-Doc",
            "Web-Static-Content","Web-Http-Errors",
            "Web-Health","Web-Http-Logging","Web-Request-Monitor","Web-Http-Tracing",
            "Web-Performance","Web-Stat-Compression","Web-Dyn-Compression",
            "Web-Security","Web-Filtering","Web-Mgmt-Tools","Web-Mgmt-Console",
            "Web-Scripting-Tools"
        )
    }

    return @(
        "IIS-WebServerRole","IIS-WebServer","IIS-CommonHttpFeatures",
        "IIS-DefaultDocument","IIS-StaticContent","IIS-HttpErrors",
        "IIS-HealthAndDiagnostics","IIS-HttpLogging",
        "IIS-RequestMonitor","IIS-HttpTracing","IIS-Performance",
        "IIS-HttpCompressionStatic","IIS-HttpCompressionDynamic",
        "IIS-Security","IIS-RequestFiltering","IIS-ManagementConsole",
        "IIS-ManagementScriptingTools"
    )
}

function Ensure-Chocolatey {
    param($Config)

    if (-not ($Config.InstallNSSM -or $Config.InstallIIS -or $Config.InstallARR)) {
        Write-Step "[1/12] Chocolatey skipped"
        Write-Info "Chocolatey not required for the selected profile."
        return
    }

    Write-Step "[1/12] Checking Chocolatey"
    $choco = Get-Command choco -ErrorAction SilentlyContinue
    if ($choco) {
        Write-OK ("Chocolatey already installed (version {0})" -f (choco --version))
        return
    }

    Write-Warn "Installing Chocolatey..."
    Set-ExecutionPolicy Bypass -Scope Process -Force
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
    Invoke-Expression ((New-Object System.Net.WebClient).DownloadString("https://community.chocolatey.org/install.ps1"))
    $env:PATH = [System.Environment]::GetEnvironmentVariable("PATH", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("PATH", "User")

    if (-not (Get-Command choco -ErrorAction SilentlyContinue)) {
        Fail-Install "Chocolatey installation failed."
    }

    Write-OK ("Chocolatey installed (version {0})" -f (choco --version))
}

function Ensure-NSSM {
    param($Config)

    if (-not $Config.InstallNSSM) {
        Write-Step "[2/12] NSSM skipped"
        Write-Info "NSSM not required for the selected profile."
        return
    }

    Write-Step "[2/12] Checking NSSM"
    if (Test-Path $Config.NssmPath) {
        Write-OK ("NSSM already installed at {0}" -f $Config.NssmPath)
        return
    }

    Write-Warn "Installing NSSM via Chocolatey..."
    Invoke-ExternalCommand -FilePath "choco" -Arguments @("install", "nssm", "-y", "--force") -Description "choco install nssm"

    if (-not (Test-Path $Config.NssmPath)) {
        Fail-Install ("NSSM installation failed; executable not found at {0}" -f $Config.NssmPath)
    }

    Write-OK "NSSM installed"
}

function Ensure-IIS {
    param($Config)

    if (-not $Config.InstallIIS) {
        Write-Step "[3/12] IIS features skipped"
        Write-Info "IIS not required for the selected profile."
        return
    }

    Write-Step "[3/12] Checking IIS features"
    foreach ($feature in (Get-IISFeatureList)) {
        if ($Script:IsServer) {
            $state = Get-WindowsFeature -Name $feature
            if ($state.Installed) {
                Write-Info ("Feature already installed: {0}" -f $feature)
            } else {
                Write-Info ("Installing IIS feature: {0}" -f $feature)
                Install-WindowsFeature -Name $feature | Out-Null
                Write-OK ("Installed: {0}" -f $feature)
            }
        } else {
            $state = Get-WindowsOptionalFeature -Online -FeatureName $feature -ErrorAction SilentlyContinue
            if ($state -and $state.State -eq "Enabled") {
                Write-Info ("Feature already enabled: {0}" -f $feature)
            } else {
                Write-Info ("Enabling IIS feature: {0}" -f $feature)
                Enable-WindowsOptionalFeature -Online -FeatureName $feature -All -NoRestart | Out-Null
                Write-OK ("Enabled: {0}" -f $feature)
            }
        }
    }

    Write-Step "[4/12] Checking URL Rewrite"
    if (Test-Path $UrlRewriteDll) {
        Write-OK "URL Rewrite already installed"
        return
    }

    Write-Warn "Installing URL Rewrite via Chocolatey..."
    Invoke-ExternalCommand -FilePath "choco" -Arguments @("install", "urlrewrite", "-y", "--force") -Description "choco install urlrewrite"

    if (-not (Test-Path $UrlRewriteDll)) {
        Fail-Install ("URL Rewrite install finished but rewrite.dll was not found at {0}" -f $UrlRewriteDll)
    }

    Write-OK "URL Rewrite installed"
}

function Ensure-ARR {
    param($Config)

    if (-not $Config.InstallARR) {
        Write-Step "[5/12] ARR skipped"
        Write-Info "ARR not required for the selected profile."
        return
    }

    Write-Step "[5/12] Checking ARR"
    if (Test-Path $ArrRouterDll) {
        Write-OK "ARR already installed"
    } else {
        Write-Warn "Installing ARR via Chocolatey..."
        Invoke-ExternalCommand -FilePath "choco" -Arguments @("install", "iis-arr", "-y", "--force") -Description "choco install iis-arr"

        if (-not (Test-Path $ArrRouterDll)) {
            Fail-Install ("ARR install finished but requestRouter.dll was not found at {0}" -f $ArrRouterDll)
        }
        Write-OK "ARR installed"
    }

    Write-Step "[6/12] Enabling ARR proxy"
    Import-Module WebAdministration -ErrorAction Stop
    $arrEnabled = Get-WebConfigurationProperty `
        -PSPath "MACHINE/WEBROOT/APPHOST" `
        -Filter "system.webServer/proxy" `
        -Name "enabled" -ErrorAction Stop
    if ($arrEnabled.Value -eq $true) {
        Write-OK "ARR proxy already enabled"
        return
    }

    Set-WebConfigurationProperty `
        -PSPath "MACHINE/WEBROOT/APPHOST" `
        -Filter "system.webServer/proxy" `
        -Name "enabled" -Value "True" -ErrorAction Stop
    Write-OK "ARR proxy enabled"
}

function Ensure-Directories {
    param($Config)

    Write-Step "[7/12] Creating directories"
    foreach ($path in @($Config.InstallDir, $Config.LogDir)) {
        if (Test-Path $path) {
            Write-OK ("Already exists: {0}" -f $path)
        } else {
            New-Item -ItemType Directory -Path $path -Force | Out-Null
            Write-OK ("Created: {0}" -f $path)
        }
    }
}

function Ensure-WatcherExecutable {
    param($Config)

    Write-Step "[8/12] Checking watcher.exe"
    $sourceExe = Join-Path $ParentDir "watcher.exe"
    if (-not (Test-Path $Config.WatcherExe) -and (Test-Path $sourceExe)) {
        Copy-Item $sourceExe $Config.WatcherExe
        Write-OK ("Copied watcher.exe to {0}" -f $Config.InstallDir)
    }

    if (-not (Test-Path $Config.WatcherExe)) {
        Fail-Install ("watcher.exe not found at {0}. Expected source: {1}" -f $Config.WatcherExe, $sourceExe)
    }

    Write-OK ("watcher.exe found at {0}" -f $Config.WatcherExe)
}

function Write-EnvironmentFile {
    param($Config)

    Write-Step "[9/12] Writing .env"
    if (Test-Path $Config.EnvFile) {
        Write-Warn (".env already exists at {0}; not overwriting." -f $Config.EnvFile)
        Write-Info "Delete .env and rerun the installer if you want to regenerate it."
        return
    }

    $nssmLine = if ($Config.InstallNSSM) { "NSSM_PATH=$($Config.NssmPath)" } else { "# NSSM_PATH= (NSSM not installed)" }
    $envContent = @"
ENVIRONMENT=production
GITHUB_TOKEN=$($Config.GitHubToken)
LOG_DIR=$($Config.LogDir)
$nssmLine
DB_PATH=$($Config.DBPath)
API_PORT=$($Config.APIPort)
"@
    [System.IO.File]::WriteAllText($Config.EnvFile, $envContent, [System.Text.Encoding]::ASCII)
    Write-OK (".env created at {0}" -f $Config.EnvFile)

    if ([string]::IsNullOrWhiteSpace($Config.GitHubToken)) {
        Write-Warn "GITHUB_TOKEN is empty; this is only valid for public repositories."
    }
}

function Secure-EnvironmentFile {
    param($Config)

    Write-Step "[10/12] Securing .env"
    Invoke-ExternalCommand -FilePath "icacls.exe" -Arguments @($Config.EnvFile, "/inheritance:r") -Description "icacls remove inheritance"
    Invoke-ExternalCommand -FilePath "icacls.exe" -Arguments @($Config.EnvFile, "/grant", "SYSTEM:(F)") -Description "icacls grant SYSTEM"
    Invoke-ExternalCommand -FilePath "icacls.exe" -Arguments @($Config.EnvFile, "/grant", "BUILTIN\Administrators:(F)") -Description "icacls grant Administrators"
    Write-OK ".env restricted to SYSTEM and Administrators only"
}

function Test-GitHubReachability {
    Write-Step "[11/12] Verifying outbound HTTPS to github.com"
    try {
        $response = Invoke-WebRequest -Uri "https://github.com" -UseBasicParsing -TimeoutSec 10
        if ($response.StatusCode -eq 200) {
            Write-OK ("github.com reachable (HTTP {0})" -f $response.StatusCode)
            return
        }
        Fail-Install ("github.com returned HTTP {0}" -f $response.StatusCode)
    } catch {
        Fail-Install ("Cannot reach github.com. Check firewall or proxy settings. {0}" -f $_.Exception.Message)
    }
}

function Configure-WatcherService {
    param($Config)

    if (-not $Config.InstallNSSM) {
        Write-Step "[12/12] NSSM service setup skipped"
        Write-Warn "Watcher service was not registered because NSSM is not selected."
        return
    }

    Write-Step "[12/12] Configuring Watcher NSSM service"
    $existing = Get-Service $Config.ServiceName -ErrorAction SilentlyContinue

    if ($existing) {
        if ($existing.Status -eq "Running") {
            Write-Warn ("Stopping existing service: {0}" -f $Config.ServiceName)
            Invoke-ExternalCommand -FilePath $Config.NssmPath -Arguments @("stop", $Config.ServiceName, "confirm") -Description "nssm stop"
            Start-Sleep -Seconds 3
        }

        Write-Info ("Updating existing service: {0}" -f $Config.ServiceName)
        Invoke-ExternalCommand -FilePath $Config.NssmPath -Arguments @("set", $Config.ServiceName, "Application", $Config.WatcherExe) -Description "nssm set Application"
    } else {
        Write-Info ("Registering new service: {0}" -f $Config.ServiceName)
        Invoke-ExternalCommand -FilePath $Config.NssmPath -Arguments @("install", $Config.ServiceName, $Config.WatcherExe) -Description "nssm install"

        $created = Get-Service $Config.ServiceName -ErrorAction SilentlyContinue
        if (-not $created) {
            Fail-Install ("NSSM install ran but service {0} was not created." -f $Config.ServiceName)
        }
        Write-OK "Service registered"
    }

    $nssmSettings = @(
        @("AppParameters", "-config `"$($Config.EnvFile)`""),
        @("AppDirectory", $Config.InstallDir),
        @("Start", "SERVICE_AUTO_START"),
        @("AppStdout", (Join-Path $Config.LogDir "watcher.out.log")),
        @("AppStderr", (Join-Path $Config.LogDir "watcher.err.log")),
        @("AppRotateFiles", "1"),
        @("AppRotateOnline", "1"),
        @("AppRotateSeconds", "86400"),
        @("AppRestartDelay", [string]$Config.RestartDelay)
    )

    foreach ($setting in $nssmSettings) {
        Invoke-ExternalCommand -FilePath $Config.NssmPath -Arguments @("set", $Config.ServiceName, $setting[0], $setting[1]) -Description ("nssm set {0}" -f $setting[0])
    }
    Write-OK "NSSM service configured"

    Write-Info ("Starting service: {0}" -f $Config.ServiceName)
    $startOutput = ""
    try {
        $startOutput = & $Config.NssmPath start $Config.ServiceName 2>&1 | Out-String
    } catch {
        $startOutput = ($_ | Out-String)
    }

    if (-not [string]::IsNullOrWhiteSpace($startOutput)) {
        foreach ($line in ($startOutput -split "`r?`n")) {
            if (-not [string]::IsNullOrWhiteSpace($line)) {
                Write-Info ("  {0}" -f $line.TrimEnd())
            }
        }
    }

    $startOutputUpper = $startOutput.ToUpperInvariant()
    if (
        $LASTEXITCODE -ne 0 -and
        $startOutputUpper -notmatch "SERVICE_START_PENDING" -and
        $startOutputUpper -notmatch "SERVICE_RUNNING"
    ) {
        Fail-Install ("nssm start failed for service {0}" -f $Config.ServiceName)
    }

    $service = $null
    for ($i = 0; $i -lt 15; $i++) {
        Start-Sleep -Seconds 2
        $service = Get-Service $Config.ServiceName -ErrorAction SilentlyContinue
        if ($service -and $service.Status -eq "Running") {
            break
        }
        if ($service) {
            Write-Info ("Waiting for service to start; current status: {0}" -f $service.Status)
        }
    }

    if (-not $service -or $service.Status -ne "Running") {
        $finalStatus = if ($service) { $service.Status } else { "missing" }
        Fail-Install ("Service {0} did not reach Running state (last status: {1}). Check logs in {2}" -f $Config.ServiceName, $finalStatus, $Config.LogDir)
    }

    Write-OK "Watcher service is running"
}

function Verify-API {
    param($Config)

    Write-Step "Final verification: checking API status"
    Start-Sleep -Seconds 2
    try {
        $response = Invoke-WebRequest -Uri ("http://localhost:{0}/api/status" -f $Config.APIPort) -UseBasicParsing -TimeoutSec 5
        Write-OK ("API is responding (HTTP {0})" -f $response.StatusCode)
    } catch {
        Write-Warn "API is not responding yet; the service may still be starting up."
    }
}

function Get-InstallSummary {
    param($Config)

    $lines = @(
        "Dashboard: http://localhost:$($Config.APIPort)",
        "Log file: $Script:LogPath",
        "Config: $($Config.EnvFile)",
        "Database: $($Config.DBPath)",
        "Features:"
    )

    $lines += if ($Config.InstallNSSM) { "  [x] NSSM" } else { "  [ ] NSSM" }
    $lines += if ($Config.InstallIIS)  { "  [x] IIS" } else { "  [ ] IIS" }
    $lines += if ($Config.InstallARR)  { "  [x] ARR" } else { "  [ ] ARR" }

    if ($Config.InstallNSSM) {
        $lines += "Commands:"
        $lines += "  Status    : Get-Service $($Config.ServiceName)"
        $lines += "  Stop      : nssm stop $($Config.ServiceName)"
        $lines += "  Start     : nssm start $($Config.ServiceName)"
        $lines += "  Uninstall : nssm remove $($Config.ServiceName) confirm"
    }

    return ($lines -join [Environment]::NewLine)
}

function Invoke-Installation {
    param($Config)

    Write-Step "Starting installation"
    Write-Info ("Service: {0}" -f $Config.ServiceName)
    Write-Info ("Install directory: {0}" -f $Config.InstallDir)
    Write-Info ("API port: {0}" -f $Config.APIPort)
    Write-Info ("Debug log: {0}" -f $Script:LogPath)

    Set-ProgressStep -Value 5  -Status "Preparing installation"
    Ensure-Chocolatey -Config $Config

    Set-ProgressStep -Value 15 -Status "Checking NSSM"
    Ensure-NSSM -Config $Config

    Set-ProgressStep -Value 35 -Status "Checking IIS components"
    Ensure-IIS -Config $Config

    Set-ProgressStep -Value 45 -Status "Checking ARR"
    Ensure-ARR -Config $Config

    Set-ProgressStep -Value 55 -Status "Creating directories"
    Ensure-Directories -Config $Config

    Set-ProgressStep -Value 65 -Status "Checking watcher.exe"
    Ensure-WatcherExecutable -Config $Config

    Set-ProgressStep -Value 72 -Status "Writing configuration"
    Write-EnvironmentFile -Config $Config

    Set-ProgressStep -Value 78 -Status "Securing configuration"
    Secure-EnvironmentFile -Config $Config

    Set-ProgressStep -Value 84 -Status "Verifying connectivity"
    Test-GitHubReachability

    Set-ProgressStep -Value 92 -Status "Configuring Watcher service"
    Configure-WatcherService -Config $Config

    Set-ProgressStep -Value 98 -Status "Verifying API"
    Verify-API -Config $Config

    Set-ProgressStep -Value 100 -Status "Installation completed successfully"
    Write-OK "Watcher installed successfully"
}

# ==============================================================
# CONSOLE INSTALLER
# ==============================================================
function Read-ConsoleDefault {
    param(
        [string]$Prompt,
        [string]$Default
    )

    if ([string]::IsNullOrWhiteSpace($Default)) {
        $value = Read-Host $Prompt
    } else {
        $value = Read-Host ("{0} [{1}]" -f $Prompt, $Default)
    }

    if ([string]::IsNullOrWhiteSpace($value)) {
        return $Default
    }
    return $value.Trim()
}

function Read-ConsoleYesNo {
    param(
        [string]$Prompt,
        [bool]$Default = $true
    )

    $suffix = if ($Default) { "[Y/n]" } else { "[y/N]" }
    while ($true) {
        $value = Read-Host ("{0} {1}" -f $Prompt, $suffix)
        if ([string]::IsNullOrWhiteSpace($value)) {
            return $Default
        }

        switch ($value.Trim().ToLowerInvariant()) {
            "y" { return $true }
            "yes" { return $true }
            "n" { return $false }
            "no" { return $false }
            default { Write-Warn "Enter Y or N." }
        }
    }
}

function Read-ConsoleIntDefault {
    param(
        [string]$Prompt,
        [int]$Default,
        [int]$Min,
        [int]$Max
    )

    while ($true) {
        $raw = Read-ConsoleDefault -Prompt $Prompt -Default ([string]$Default)
        $parsed = 0
        if ([int]::TryParse($raw, [ref]$parsed) -and $parsed -ge $Min -and $parsed -le $Max) {
            return $parsed
        }
        Write-Warn ("Enter a number between {0} and {1}." -f $Min, $Max)
    }
}

function Open-DebugLog {
    if (-not (Test-Path $Script:LogPath)) {
        Write-Warn ("Debug log file not found: {0}" -f $Script:LogPath)
        return
    }

    try {
        Start-Process -FilePath "notepad.exe" -ArgumentList @($Script:LogPath) -ErrorAction Stop
    } catch {
        Write-Warn ("Could not open debug log: {0}" -f $_.Exception.Message)
        Write-Info ("Debug log: {0}" -f $Script:LogPath)
    }
}

function Open-Dashboard {
    param($Config)

    $dashboardUrl = "http://localhost:{0}" -f $Config.APIPort
    try {
        Start-Process -FilePath $dashboardUrl -ErrorAction Stop
    } catch {
        Write-Warn ("Could not open dashboard: {0}" -f $_.Exception.Message)
        Write-Info ("Dashboard URL: {0}" -f $dashboardUrl)
    }
}

function Show-ConsoleWizard {
    $Config = $Defaults.Clone()

    Write-Host ""
    Write-Host "Watcher Installer" -ForegroundColor Cyan
    Write-Host "=================" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "Choose what this host should support:"
    Write-Host "  1. Binary Services  - NSSM for Windows service deployments"
    Write-Host "  2. IIS Static       - IIS hosting + URL Rewrite"
    Write-Host "  3. Hybrid           - Binary services + IIS hosting"
    Write-Host "  4. Full Stack       - Binary services + IIS hosting + ARR reverse proxy"
    Write-Host ""

    while ($true) {
        $profileRaw = Read-ConsoleDefault -Prompt "Preset" -Default "1"
        switch ($profileRaw) {
            "1" {
                $Config.Profile = 0
                $Config.InstallNSSM = $true
                $Config.InstallIIS = $false
                $Config.InstallARR = $false
                break
            }
            "2" {
                $Config.Profile = 1
                $Config.InstallNSSM = $false
                $Config.InstallIIS = $true
                $Config.InstallARR = $false
                break
            }
            "3" {
                $Config.Profile = 2
                $Config.InstallNSSM = $true
                $Config.InstallIIS = $true
                $Config.InstallARR = $false
                break
            }
            "4" {
                $Config.Profile = 3
                $Config.InstallNSSM = $true
                $Config.InstallIIS = $true
                $Config.InstallARR = $true
                break
            }
            default { Write-Warn "Choose 1, 2, 3, or 4." }
        }
        if ($profileRaw -in @("1", "2", "3", "4")) { break }
    }

    Write-Host ""
    $Config.InstallDir = (Read-ConsoleDefault -Prompt "Install directory" -Default $Config.InstallDir).TrimEnd("\")
    $Config.LogDir = (Read-ConsoleDefault -Prompt "Log directory" -Default $Config.LogDir).TrimEnd("\")
    $Config.ServiceName = Read-ConsoleDefault -Prompt "Watcher Windows service name" -Default $Config.ServiceName
    $Config.APIPort = Read-ConsoleIntDefault -Prompt "API / dashboard port" -Default ([int]$Config.APIPort) -Min 1 -Max 65535

    if ($Config.InstallNSSM) {
        $Config.NssmPath = Read-ConsoleDefault -Prompt "NSSM path" -Default $Config.NssmPath
    }

    $Config.GitHubToken = Read-ConsoleDefault -Prompt "GitHub token (optional; required for private repos)" -Default $Config.GitHubToken

    if ([string]::IsNullOrWhiteSpace($Config.InstallDir)) {
        Fail-Install "Install directory is required."
    }
    if ([string]::IsNullOrWhiteSpace($Config.ServiceName)) {
        Fail-Install "Service name is required."
    }
    if ($Config.ServiceName -match '[\\/:*?"<>|]') {
        Fail-Install "Service name contains invalid characters."
    }

    Write-Host ""
    Write-Host "Install summary" -ForegroundColor Cyan
    Write-Host "---------------" -ForegroundColor Cyan
    Write-Host ("Install directory : {0}" -f $Config.InstallDir)
    Write-Host ("Log directory     : {0}" -f $Config.LogDir)
    Write-Host ("Service name      : {0}" -f $Config.ServiceName)
    Write-Host ("Dashboard         : http://localhost:{0}" -f $Config.APIPort)
    Write-Host ("NSSM              : {0}" -f $(if ($Config.InstallNSSM) { "yes" } else { "no" }))
    Write-Host ("IIS               : {0}" -f $(if ($Config.InstallIIS) { "yes" } else { "no" }))
    Write-Host ("ARR               : {0}" -f $(if ($Config.InstallARR) { "yes" } else { "no" }))
    Write-Host ("Debug log         : {0}" -f $Script:LogPath)
    Write-Host ""

    if (-not (Read-ConsoleYesNo -Prompt "Proceed with installation?" -Default $true)) {
        Write-Info "Installation cancelled."
        return $null
    }

    return $Config
}

function Show-CompletionMenu {
    param($Config)

    Write-Host ""
    Write-Host "Installation completed." -ForegroundColor Green
    Write-Host ""
    Write-Host (Get-InstallSummary -Config $Config)

    while ($true) {
        Write-Host ""
        Write-Host "Next actions:"
        Write-Host "  1. Open dashboard and close installer"
        Write-Host "  2. Open debug log"
        Write-Host "  3. Finish / close installer"
        $choice = Read-ConsoleDefault -Prompt "Choose an action" -Default "1"

        switch ($choice) {
            "1" {
                Open-Dashboard -Config $Config
                return
            }
            "2" { Open-DebugLog }
            "3" { return }
            default { Write-Warn "Choose 1, 2, or 3." }
        }
    }
}

# ==============================================================
# ENTRYPOINT
# ==============================================================
try {
    if ($Silent) {
        $Config = $Defaults.Clone()
    } else {
        $Config = Show-ConsoleWizard
        if (-not $Config) {
            exit 0
        }
    }

    if ($null -eq $Config.InstallNSSM) {
        $Config.InstallNSSM = $Config.Profile -in @(0, 2, 3)
    }
    if ($null -eq $Config.InstallIIS) {
        $Config.InstallIIS = $Config.Profile -in @(1, 2, 3)
    }
    if ($null -eq $Config.InstallARR) {
        $Config.InstallARR = $Config.Profile -eq 3
    }
    $Config.WatcherExe   = Join-Path $Config.InstallDir "watcher.exe"
    $Config.EnvFile      = Join-Path $Config.InstallDir ".env"
    $Config.DBPath       = Join-Path $Config.InstallDir "watcher.db"
    $Config.RestartDelay = 5000

    Invoke-Installation -Config $Config
    Write-Info ""
    Write-Info (Get-InstallSummary -Config $Config)

    if ($Silent) {
        exit 0
    }

    Show-CompletionMenu -Config $Config
} catch {
    $message = $_.Exception.Message
    Write-InstallerLog -Level "ERROR" -Message $message
    if (-not $Silent) {
        Show-Message -Text ("Installer failed.`r`n`r`n{0}`r`n`r`nDebug log:`r`n{1}" -f $message, $Script:LogPath) -Icon Error
    }
    if ($DebugMode) {
        Write-Host ""
        Write-Host "Debug log: $Script:LogPath" -ForegroundColor Yellow
    }
    exit 1
}
