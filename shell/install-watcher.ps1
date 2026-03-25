# ==============================================================
# install-watcher.ps1
# Interactive bootstrap: configures installation via a GUI wizard.
# Run as Administrator on Windows 10/11 or Windows Server 2022.
# ==============================================================

param(
    [switch]$Silent,
    [switch]$DebugMode
)

$ErrorActionPreference = "Stop"

# ==============================================================
# DEFAULTS -- used by wizard pre-fill and silent mode
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
$ScriptDir     = Split-Path -Parent $MyInvocation.MyCommand.Path
$ParentDir     = Split-Path -Parent $ScriptDir
$Script:IsServer = (Get-CimInstance Win32_OperatingSystem).ProductType -ne 1
$Script:LogPath  = Join-Path $env:TEMP ("watcher-installer-" + (Get-Date -Format "yyyyMMdd-HHmmss") + ".log")
$Script:ProgressUi = $null
$Script:WinFormsInitialized = $false

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

    if ($Silent -or $DebugMode) {
        switch ($Level.ToUpperInvariant()) {
            "ERROR" { Write-Host $line -ForegroundColor Red }
            "WARN"  { Write-Host $line -ForegroundColor Yellow }
            "OK"    { Write-Host $line -ForegroundColor Green }
            "STEP"  { Write-Host $line -ForegroundColor Cyan }
            default { Write-Host $line }
        }
    }

    if ($Script:ProgressUi -and $Script:ProgressUi.Form -and -not $Script:ProgressUi.Form.IsDisposed) {
        $Script:ProgressUi.LogBox.AppendText($line + [Environment]::NewLine)
        $Script:ProgressUi.LogBox.SelectionStart = $Script:ProgressUi.LogBox.TextLength
        $Script:ProgressUi.LogBox.ScrollToCaret()
        $Script:ProgressUi.StatusLabel.Text = $line
        $Script:ProgressUi.Form.Refresh()
        [System.Windows.Forms.Application]::DoEvents()
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

    Add-Type -AssemblyName System.Windows.Forms
    $iconValue = [System.Windows.Forms.MessageBoxIcon]::$Icon
    $buttons = [System.Windows.Forms.MessageBoxButtons]::OK
    [void][System.Windows.Forms.MessageBox]::Show($Text, $Title, $buttons, $iconValue)
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

    if ($Script:ProgressUi -and $Script:ProgressUi.Form -and -not $Script:ProgressUi.Form.IsDisposed) {
        $valueCopy = $Value
        $statusCopy = $Status
        if ($valueCopy -lt $Script:ProgressUi.ProgressBar.Minimum) {
            $valueCopy = $Script:ProgressUi.ProgressBar.Minimum
        }
        if ($valueCopy -gt $Script:ProgressUi.ProgressBar.Maximum) {
            $valueCopy = $Script:ProgressUi.ProgressBar.Maximum
        }
        $Script:ProgressUi.ProgressBar.Value = $valueCopy
        $Script:ProgressUi.StatusLabel.Text = $statusCopy
        $Script:ProgressUi.Form.Refresh()
        [System.Windows.Forms.Application]::DoEvents()
    }
}

function Get-IISFeatureList {
    if ($Script:IsServer) {
        return @(
            "Web-Server","Web-WebServer","Web-Common-Http","Web-Default-Doc",
            "Web-Static-Content","Web-Http-Errors","Web-Http-Redirect",
            "Web-Health","Web-Http-Logging","Web-Request-Monitor","Web-Http-Tracing",
            "Web-Performance","Web-Stat-Compression","Web-Dyn-Compression",
            "Web-Security","Web-Filtering","Web-Mgmt-Tools","Web-Mgmt-Console",
            "Web-Scripting-Tools"
        )
    }

    return @(
        "IIS-WebServerRole","IIS-WebServer","IIS-CommonHttpFeatures",
        "IIS-DefaultDocument","IIS-StaticContent","IIS-HttpErrors",
        "IIS-HttpRedirect","IIS-HealthAndDiagnostics","IIS-HttpLogging",
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
    Invoke-ExternalCommand -FilePath $Config.NssmPath -Arguments @("start", $Config.ServiceName) -Description "nssm start"
    Start-Sleep -Seconds 4

    $service = Get-Service $Config.ServiceName -ErrorAction SilentlyContinue
    if (-not $service -or $service.Status -ne "Running") {
        Fail-Install ("Service {0} did not start. Check logs in {1}" -f $Config.ServiceName, $Config.LogDir)
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
# GUI WIZARD
# ==============================================================
function Initialize-WinForms {
    if ($Script:WinFormsInitialized) {
        return
    }

    Add-Type -AssemblyName System.Windows.Forms
    Add-Type -AssemblyName System.Drawing
    [System.Windows.Forms.Application]::EnableVisualStyles()
    [System.Windows.Forms.Application]::SetCompatibleTextRenderingDefault($false)
    $Script:WinFormsInitialized = $true
}

function New-Label {
    param($Text, $X, $Y, $Width = 160, $Height = 20)
    $label = New-Object System.Windows.Forms.Label
    $label.Text = $Text
    $label.Location = New-Object System.Drawing.Point($X, $Y)
    $label.Size = New-Object System.Drawing.Size($Width, $Height)
    $label.Font = New-Object System.Drawing.Font("Segoe UI", 9)
    return $label
}

function New-TextBox {
    param($Text, $X, $Y, $Width = 280)
    $textBox = New-Object System.Windows.Forms.TextBox
    $textBox.Text = $Text
    $textBox.Location = New-Object System.Drawing.Point($X, $Y)
    $textBox.Size = New-Object System.Drawing.Size($Width, 24)
    $textBox.Font = New-Object System.Drawing.Font("Segoe UI", 9)
    return $textBox
}

function New-StatusLabel {
    param($X, $Y, $Width = 400)
    $label = New-Object System.Windows.Forms.Label
    $label.Text = ""
    $label.Location = New-Object System.Drawing.Point($X, $Y)
    $label.Size = New-Object System.Drawing.Size($Width, 16)
    $label.Font = New-Object System.Drawing.Font("Segoe UI", 8)
    $label.ForeColor = [System.Drawing.Color]::FromArgb(180, 0, 0)
    return $label
}

function Show-Wizard {
    Initialize-WinForms

    $form = New-Object System.Windows.Forms.Form
    $form.Text = "Watcher - Installation Wizard"
    $form.Size = New-Object System.Drawing.Size(640, 720)
    $form.StartPosition = "CenterScreen"
    $form.FormBorderStyle = "FixedDialog"
    $form.MaximizeBox = $false
    $form.MinimizeBox = $false
    $form.BackColor = [System.Drawing.Color]::FromArgb(248, 246, 240)

    $clientWidth = 620
    $contentTop = 96
    $contentHeight = 520
    $footerTop = 622
    $buttonTop = 634
    $clientHeight = 678

    $header = New-Object System.Windows.Forms.Panel
    $header.Size = New-Object System.Drawing.Size(640, 86)
    $header.Location = New-Object System.Drawing.Point(0, 0)
    $header.BackColor = [System.Drawing.Color]::FromArgb(29, 53, 87)
    $form.Controls.Add($header)

    $title = New-Object System.Windows.Forms.Label
    $title.Text = "Configure Installation"
    $title.Location = New-Object System.Drawing.Point(20, 10)
    $title.Size = New-Object System.Drawing.Size(520, 28)
    $title.Font = New-Object System.Drawing.Font("Segoe UI Semibold", 15)
    $title.ForeColor = [System.Drawing.Color]::White
    $header.Controls.Add($title)

    $stepLabel = New-Object System.Windows.Forms.Label
    $stepLabel.Text = "Step 1 of 2"
    $stepLabel.Location = New-Object System.Drawing.Point(540, 12)
    $stepLabel.Size = New-Object System.Drawing.Size(80, 20)
    $stepLabel.TextAlign = "MiddleRight"
    $stepLabel.Font = New-Object System.Drawing.Font("Segoe UI", 8)
    $stepLabel.ForeColor = [System.Drawing.Color]::FromArgb(214, 224, 237)
    $header.Controls.Add($stepLabel)

    $subtitle = New-Object System.Windows.Forms.Label
    $subtitle.Location = New-Object System.Drawing.Point(20, 38)
    $subtitle.Size = New-Object System.Drawing.Size(560, 18)
    $subtitle.Font = New-Object System.Drawing.Font("Segoe UI", 9)
    $subtitle.ForeColor = [System.Drawing.Color]::FromArgb(214, 224, 237)
    $header.Controls.Add($subtitle)

    $subtitle2 = New-Object System.Windows.Forms.Label
    $subtitle2.Location = New-Object System.Drawing.Point(20, 58)
    $subtitle2.Size = New-Object System.Drawing.Size(560, 18)
    $subtitle2.Font = New-Object System.Drawing.Font("Segoe UI", 8)
    $subtitle2.ForeColor = [System.Drawing.Color]::FromArgb(186, 202, 221)
    $header.Controls.Add($subtitle2)

    function New-SectionPanel {
        param($Title, $Body, $X, $Y, $Width, $Height)

        $panel = New-Object System.Windows.Forms.Panel
        $panel.Location = New-Object System.Drawing.Point($X, $Y)
        $panel.Size = New-Object System.Drawing.Size($Width, $Height)
        $panel.BackColor = [System.Drawing.Color]::White
        $panel.BorderStyle = "FixedSingle"

        $titleLabel = New-Object System.Windows.Forms.Label
        $titleLabel.Text = $Title
        $titleLabel.Location = New-Object System.Drawing.Point(14, 12)
        $titleLabel.Size = New-Object System.Drawing.Size(($Width - 28), 22)
        $titleLabel.Font = New-Object System.Drawing.Font("Segoe UI Semibold", 10)
        $titleLabel.ForeColor = [System.Drawing.Color]::FromArgb(34, 40, 49)
        $panel.Controls.Add($titleLabel)

        $bodyLabel = New-Object System.Windows.Forms.Label
        $bodyLabel.Text = $Body
        $bodyLabel.Location = New-Object System.Drawing.Point(14, 34)
        $bodyLabel.Size = New-Object System.Drawing.Size(($Width - 28), 32)
        $bodyLabel.Font = New-Object System.Drawing.Font("Segoe UI", 8)
        $bodyLabel.ForeColor = [System.Drawing.Color]::FromArgb(105, 112, 120)
        $panel.Controls.Add($bodyLabel)

        return $panel
    }

    function New-CapabilityCheckBox {
        param($Text, $X, $Y, $Width = 520)

        $check = New-Object System.Windows.Forms.CheckBox
        $check.Text = $Text
        $check.Location = New-Object System.Drawing.Point($X, $Y)
        $check.Size = New-Object System.Drawing.Size($Width, 22)
        $check.Font = New-Object System.Drawing.Font("Segoe UI", 9)
        return $check
    }

    $page1 = New-Object System.Windows.Forms.Panel
    $page1.Location = New-Object System.Drawing.Point(0, $contentTop)
    $page1.Size = New-Object System.Drawing.Size($clientWidth, $contentHeight)
    $page1.BackColor = $form.BackColor
    $form.Controls.Add($page1)

    $page2 = New-Object System.Windows.Forms.Panel
    $page2.Location = New-Object System.Drawing.Point(0, $contentTop)
    $page2.Size = New-Object System.Drawing.Size($clientWidth, $contentHeight)
    $page2.BackColor = $form.BackColor
    $page2.Visible = $false
    $form.Controls.Add($page2)

    $presetPanel = New-SectionPanel "Quick Presets" "Start from a recommended machine profile, then adjust the capabilities below." 20 6 590 118
    $page1.Controls.Add($presetPanel)

    $presetButtons = @()
    $presetSpecs = @(
        @{ Text = "Binary Services"; X = 14; Y = 70; W = 130 },
        @{ Text = "IIS Static"; X = 152; Y = 70; W = 110 },
        @{ Text = "Hybrid"; X = 270; Y = 70; W = 90 },
        @{ Text = "Full Stack"; X = 368; Y = 70; W = 100 }
    )
    foreach ($spec in $presetSpecs) {
        $btn = New-Object System.Windows.Forms.Button
        $btn.Text = $spec.Text
        $btn.Location = New-Object System.Drawing.Point($spec.X, $spec.Y)
        $btn.Size = New-Object System.Drawing.Size($spec.W, 30)
        $btn.FlatStyle = "Flat"
        $btn.BackColor = [System.Drawing.Color]::FromArgb(244, 241, 232)
        $presetPanel.Controls.Add($btn)
        $presetButtons += $btn
    }

    $presetHint = New-Object System.Windows.Forms.Label
    $presetHint.Text = "Selected preset: Custom"
    $presetHint.Location = New-Object System.Drawing.Point(480, 76)
    $presetHint.Size = New-Object System.Drawing.Size(96, 20)
    $presetHint.Font = New-Object System.Drawing.Font("Segoe UI", 8)
    $presetHint.ForeColor = [System.Drawing.Color]::FromArgb(105, 112, 120)
    $presetPanel.Controls.Add($presetHint)

    $capPanel = New-SectionPanel "Windows Capabilities" "Choose what this host should support. ARR automatically requires IIS hosting." 20 138 590 186
    $page1.Controls.Add($capPanel)

    $chkNSSM = New-CapabilityCheckBox "Binary services: install NSSM service management" 14 74
    $chkNSSM.Checked = [bool]$Defaults.InstallNSSM
    $capPanel.Controls.Add($chkNSSM)

    $capNssmHint = New-Object System.Windows.Forms.Label
    $capNssmHint.Text = "Needed when Watcher should run or manage Windows services for app binaries."
    $capNssmHint.Location = New-Object System.Drawing.Point(34, 95)
    $capNssmHint.Size = New-Object System.Drawing.Size(520, 18)
    $capNssmHint.Font = New-Object System.Drawing.Font("Segoe UI", 8)
    $capNssmHint.ForeColor = [System.Drawing.Color]::FromArgb(105, 112, 120)
    $capPanel.Controls.Add($capNssmHint)

    $chkIIS = New-CapabilityCheckBox "IIS hosting: enable IIS features, management tools, and URL Rewrite" 14 118
    $chkIIS.Checked = [bool]$Defaults.InstallIIS
    $capPanel.Controls.Add($chkIIS)

    $capIISHint = New-Object System.Windows.Forms.Label
    $capIISHint.Text = "Use for static sites today, and for future IIS-based workloads such as PHP."
    $capIISHint.Location = New-Object System.Drawing.Point(34, 139)
    $capIISHint.Size = New-Object System.Drawing.Size(520, 18)
    $capIISHint.Font = New-Object System.Drawing.Font("Segoe UI", 8)
    $capIISHint.ForeColor = [System.Drawing.Color]::FromArgb(105, 112, 120)
    $capPanel.Controls.Add($capIISHint)

    $chkARR = New-CapabilityCheckBox "Reverse proxy: install ARR and enable IIS proxy support" 14 162
    $chkARR.Checked = [bool]$Defaults.InstallARR
    $capPanel.Controls.Add($chkARR)

    $summaryPanel = New-SectionPanel "Install Summary" "A quick read of what this installer is going to prepare on this machine." 20 338 590 126
    $page1.Controls.Add($summaryPanel)
    $lblCapabilitySummary = New-Object System.Windows.Forms.Label
    $lblCapabilitySummary.Location = New-Object System.Drawing.Point(14, 64)
    $lblCapabilitySummary.Size = New-Object System.Drawing.Size(560, 44)
    $lblCapabilitySummary.Font = New-Object System.Drawing.Font("Segoe UI", 8)
    $lblCapabilitySummary.ForeColor = [System.Drawing.Color]::FromArgb(64, 72, 83)
    $summaryPanel.Controls.Add($lblCapabilitySummary)

    $pathsPanel = New-SectionPanel "Watcher Configuration" "These values are written into the Watcher install and used by the bootstrap process." 20 6 590 488
    $page2.Controls.Add($pathsPanel)

    $pathsPanel.Controls.Add((New-Label "Install directory" 14 74 160 20))
    $tbInstall = New-TextBox $Defaults.InstallDir 14 96 470
    $pathsPanel.Controls.Add($tbInstall)
    $btnBrowseInstall = New-Object System.Windows.Forms.Button
    $btnBrowseInstall.Text = "..."
    $btnBrowseInstall.Location = New-Object System.Drawing.Point(492, 96)
    $btnBrowseInstall.Size = New-Object System.Drawing.Size(32, 24)
    $btnBrowseInstall.FlatStyle = "Flat"
    $btnBrowseInstall.Add_Click({
        $dialog = New-Object System.Windows.Forms.FolderBrowserDialog
        $dialog.SelectedPath = $tbInstall.Text
        if ($dialog.ShowDialog() -eq "OK") {
            $tbInstall.Text = $dialog.SelectedPath
        }
    })
    $pathsPanel.Controls.Add($btnBrowseInstall)
    $lblInstallErr = New-StatusLabel 14 124 520
    $pathsPanel.Controls.Add($lblInstallErr)

    $pathsPanel.Controls.Add((New-Label "Log directory" 14 150 160 20))
    $tbLog = New-TextBox $Defaults.LogDir 14 172 470
    $pathsPanel.Controls.Add($tbLog)
    $btnBrowseLog = New-Object System.Windows.Forms.Button
    $btnBrowseLog.Text = "..."
    $btnBrowseLog.Location = New-Object System.Drawing.Point(492, 172)
    $btnBrowseLog.Size = New-Object System.Drawing.Size(32, 24)
    $btnBrowseLog.FlatStyle = "Flat"
    $btnBrowseLog.Add_Click({
        $dialog = New-Object System.Windows.Forms.FolderBrowserDialog
        $dialog.SelectedPath = $tbLog.Text
        if ($dialog.ShowDialog() -eq "OK") {
            $tbLog.Text = $dialog.SelectedPath
        }
    })
    $pathsPanel.Controls.Add($btnBrowseLog)

    $pathsPanel.Controls.Add((New-Label "Watcher Windows service name" 14 218 210 20))
    $tbService = New-TextBox $Defaults.ServiceName 14 240 240
    $pathsPanel.Controls.Add($tbService)
    $lblSvcErr = New-StatusLabel 14 268 520
    $pathsPanel.Controls.Add($lblSvcErr)

    $pathsPanel.Controls.Add((New-Label "API / dashboard port" 14 294 180 20))
    $tbPort = New-TextBox $Defaults.APIPort 14 316 80
    $pathsPanel.Controls.Add($tbPort)
    $lblPortStatus = New-StatusLabel 108 320 360
    $pathsPanel.Controls.Add($lblPortStatus)
    $tbPort.Add_TextChanged({
        $portText = $tbPort.Text.Trim()
        if ($portText -match "^\d+$" -and [int]$portText -ge 1 -and [int]$portText -le 65535) {
            $inUse = netstat -ano 2>$null | Select-String ":$portText\s" | Select-String "LISTEN"
            if ($inUse) {
                $lblPortStatus.Text = "Port $portText is already in use"
                $lblPortStatus.ForeColor = [System.Drawing.Color]::FromArgb(200, 100, 0)
            } else {
                $lblPortStatus.Text = "Port $portText is available"
                $lblPortStatus.ForeColor = [System.Drawing.Color]::FromArgb(0, 130, 0)
            }
        } else {
            $lblPortStatus.Text = "Enter a number between 1 and 65535"
            $lblPortStatus.ForeColor = [System.Drawing.Color]::FromArgb(180, 0, 0)
        }
    })

    $lblNssm = New-Label "NSSM path" 14 352 160 20
    $pathsPanel.Controls.Add($lblNssm)
    $tbNssm = New-TextBox $Defaults.NssmPath 14 374 470
    $pathsPanel.Controls.Add($tbNssm)
    $btnBrowseNssm = New-Object System.Windows.Forms.Button
    $btnBrowseNssm.Text = "..."
    $btnBrowseNssm.Location = New-Object System.Drawing.Point(492, 374)
    $btnBrowseNssm.Size = New-Object System.Drawing.Size(32, 24)
    $btnBrowseNssm.FlatStyle = "Flat"
    $btnBrowseNssm.Add_Click({
        $dialog = New-Object System.Windows.Forms.OpenFileDialog
        $dialog.Filter = "NSSM executable|nssm.exe"
        if ($dialog.ShowDialog() -eq "OK") {
            $tbNssm.Text = $dialog.FileName
        }
    })
    $pathsPanel.Controls.Add($btnBrowseNssm)
    $lblNssmErr = New-StatusLabel 14 402 520
    $pathsPanel.Controls.Add($lblNssmErr)

    $pathsPanel.Controls.Add((New-Label "GitHub token (PAT)" 14 428 160 20))
    $tokenNote = New-Object System.Windows.Forms.Label
    $tokenNote.Text = "Required for private repos. Leave empty for public repos."
    $tokenNote.Location = New-Object System.Drawing.Point(14, 444)
    $tokenNote.Size = New-Object System.Drawing.Size(520, 16)
    $tokenNote.Font = New-Object System.Drawing.Font("Segoe UI", 8)
    $tokenNote.ForeColor = [System.Drawing.Color]::FromArgb(110, 110, 110)
    $pathsPanel.Controls.Add($tokenNote)

    $tbToken = New-Object System.Windows.Forms.TextBox
    $tbToken.Text = $Defaults.GitHubToken
    $tbToken.Location = New-Object System.Drawing.Point(14, 466)
    $tbToken.Size = New-Object System.Drawing.Size(270, 24)
    $tbToken.Font = New-Object System.Drawing.Font("Segoe UI", 9)
    $tbToken.PasswordChar = [char]0x2022
    $pathsPanel.Controls.Add($tbToken)

    $chkShowToken = New-Object System.Windows.Forms.CheckBox
    $chkShowToken.Text = "Show"
    $chkShowToken.Location = New-Object System.Drawing.Point(290, 468)
    $chkShowToken.Size = New-Object System.Drawing.Size(60, 20)
    $chkShowToken.Add_CheckedChanged({
        $tbToken.PasswordChar = if ($chkShowToken.Checked) { [char]0 } else { [char]0x2022 }
    })
    $pathsPanel.Controls.Add($chkShowToken)

    function Apply-CapabilityPreset {
        param([string]$PresetName)

        switch ($PresetName) {
            "Binary Services" {
                $chkNSSM.Checked = $true
                $chkIIS.Checked = $false
                $chkARR.Checked = $false
            }
            "IIS Static" {
                $chkNSSM.Checked = $false
                $chkIIS.Checked = $true
                $chkARR.Checked = $false
            }
            "Hybrid" {
                $chkNSSM.Checked = $true
                $chkIIS.Checked = $true
                $chkARR.Checked = $false
            }
            "Full Stack" {
                $chkNSSM.Checked = $true
                $chkIIS.Checked = $true
                $chkARR.Checked = $true
            }
        }
        $presetHint.Text = "Selected preset: $PresetName"
    }

    function Sync-CapabilityUi {
        if ($chkARR.Checked -and -not $chkIIS.Checked) {
            $chkIIS.Checked = $true
        }

        $needsNssm = $chkNSSM.Checked
        $tbNssm.Enabled = $needsNssm
        $btnBrowseNssm.Enabled = $needsNssm
        if ($needsNssm) {
            $lblNssm.ForeColor = [System.Drawing.Color]::Black
        } else {
            $lblNssm.ForeColor = [System.Drawing.Color]::Gray
            $lblNssmErr.Text = ""
        }

        $summaryParts = @()
        if ($chkNSSM.Checked) { $summaryParts += "NSSM for binary services" }
        if ($chkIIS.Checked)  { $summaryParts += "IIS + URL Rewrite" }
        if ($chkARR.Checked)  { $summaryParts += "ARR reverse proxy" }
        if ($summaryParts.Count -eq 0) { $summaryParts += "Watcher only (no optional Windows components selected)" }
        $lblCapabilitySummary.Text = ($summaryParts -join "   |   ")
    }

    foreach ($btn in $presetButtons) {
        $btn.Add_Click({
            Apply-CapabilityPreset $this.Text
            Sync-CapabilityUi
        })
    }

    $chkNSSM.Add_CheckedChanged({ $presetHint.Text = "Selected preset: Custom"; Sync-CapabilityUi })
    $chkIIS.Add_CheckedChanged({ $presetHint.Text = "Selected preset: Custom"; Sync-CapabilityUi })
    $chkARR.Add_CheckedChanged({ $presetHint.Text = "Selected preset: Custom"; Sync-CapabilityUi })

    switch ([int]$Defaults.Profile) {
        1 { Apply-CapabilityPreset "IIS Static" }
        2 { Apply-CapabilityPreset "Hybrid" }
        3 { Apply-CapabilityPreset "Full Stack" }
        default { Apply-CapabilityPreset "Binary Services" }
    }
    if (-not $Defaults.InstallNSSM -or $Defaults.InstallIIS -or $Defaults.InstallARR) {
        $chkNSSM.Checked = [bool]$Defaults.InstallNSSM
        $chkIIS.Checked  = [bool]$Defaults.InstallIIS
        $chkARR.Checked  = [bool]$Defaults.InstallARR
        $presetHint.Text = "Selected preset: Custom"
    }
    Sync-CapabilityUi

    $sep = New-Object System.Windows.Forms.Panel
    $sep.Size = New-Object System.Drawing.Size(590, 1)
    $sep.Location = New-Object System.Drawing.Point(20, $footerTop)
    $sep.BackColor = [System.Drawing.Color]::FromArgb(220, 220, 220)
    $form.Controls.Add($sep)

    $btnBack = New-Object System.Windows.Forms.Button
    $btnBack.Text = "Back"
    $btnBack.Location = New-Object System.Drawing.Point(324, $buttonTop)
    $btnBack.Size = New-Object System.Drawing.Size(80, 34)
    $btnBack.FlatStyle = "Flat"
    $btnBack.Enabled = $false
    $form.Controls.Add($btnBack)

    $btnNext = New-Object System.Windows.Forms.Button
    $btnNext.Text = "Next"
    $btnNext.Location = New-Object System.Drawing.Point(410, $buttonTop)
    $btnNext.Size = New-Object System.Drawing.Size(96, 34)
    $btnNext.BackColor = [System.Drawing.Color]::FromArgb(29, 53, 87)
    $btnNext.ForeColor = [System.Drawing.Color]::White
    $btnNext.FlatStyle = "Flat"
    $form.Controls.Add($btnNext)

    $btnInstall = New-Object System.Windows.Forms.Button
    $btnInstall.Text = "Install"
    $btnInstall.Location = New-Object System.Drawing.Point(410, $buttonTop)
    $btnInstall.Size = New-Object System.Drawing.Size(96, 34)
    $btnInstall.BackColor = [System.Drawing.Color]::FromArgb(29, 53, 87)
    $btnInstall.ForeColor = [System.Drawing.Color]::White
    $btnInstall.FlatStyle = "Flat"
    $btnInstall.Visible = $false
    $form.Controls.Add($btnInstall)

    $btnCancel = New-Object System.Windows.Forms.Button
    $btnCancel.Text = "Cancel"
    $btnCancel.Location = New-Object System.Drawing.Point(516, $buttonTop)
    $btnCancel.Size = New-Object System.Drawing.Size(80, 34)
    $btnCancel.FlatStyle = "Flat"
    $btnCancel.Add_Click({
        $form.DialogResult = "Cancel"
        $form.Close()
    })
    $form.Controls.Add($btnCancel)

    function Set-WizardStep {
        param([int]$Step)

        $page1.Visible = ($Step -eq 1)
        $page2.Visible = ($Step -eq 2)
        $btnBack.Enabled = ($Step -gt 1)
        $btnNext.Visible = ($Step -eq 1)
        $btnInstall.Visible = ($Step -eq 2)
        $stepLabel.Text = "Step $Step of 2"

        if ($Step -eq 1) {
            $subtitle.Text = "Pick the Windows capabilities this machine needs."
            $subtitle2.Text = "Presets are shortcuts. You can fine-tune each checkbox below."
            $form.AcceptButton = $btnNext
        } else {
            $subtitle.Text = "Configure where Watcher will be installed and how it should start."
            $subtitle2.Text = "These values are written into the install and used by the bootstrap process."
            $form.AcceptButton = $btnInstall
        }
    }

    $btnBack.Add_Click({ Set-WizardStep 1 })
    $btnNext.Add_Click({ Set-WizardStep 2 })

    $btnInstall.Add_Click({
        $ok = $true

        $lblInstallErr.Text = ""
        $lblSvcErr.Text = ""
        $lblSvcErr.ForeColor = [System.Drawing.Color]::FromArgb(180, 0, 0)
        $lblNssmErr.Text = ""
        $lblNssmErr.ForeColor = [System.Drawing.Color]::FromArgb(180, 0, 0)

        if ([string]::IsNullOrWhiteSpace($tbInstall.Text)) {
            $lblInstallErr.Text = "Install directory is required"
            $ok = $false
        }

        if ([string]::IsNullOrWhiteSpace($tbService.Text)) {
            $lblSvcErr.Text = "Service name is required"
            $ok = $false
        } elseif ($tbService.Text -match '[\\/:*?"<>|]') {
            $lblSvcErr.Text = "Service name contains invalid characters"
            $ok = $false
        } else {
            $existing = Get-Service $tbService.Text -ErrorAction SilentlyContinue
            if ($existing) {
                $lblSvcErr.Text = "Service already exists and will be updated"
                $lblSvcErr.ForeColor = [System.Drawing.Color]::FromArgb(200, 100, 0)
            }
        }

        if ($chkNSSM.Checked) {
            if ([string]::IsNullOrWhiteSpace($tbNssm.Text)) {
                $lblNssmErr.Text = "NSSM path is required"
                $ok = $false
            } elseif (-not (Test-Path $tbNssm.Text)) {
                $lblNssmErr.Text = "Not found locally; installer will try Chocolatey"
                $lblNssmErr.ForeColor = [System.Drawing.Color]::FromArgb(200, 100, 0)
            }
        }

        $portText = $tbPort.Text.Trim()
        if (-not ($portText -match "^\d+$") -or [int]$portText -lt 1 -or [int]$portText -gt 65535) {
            $lblPortStatus.Text = "Enter a valid port number"
            $lblPortStatus.ForeColor = [System.Drawing.Color]::FromArgb(180, 0, 0)
            $ok = $false
        }

        if ($ok) {
            $form.Tag = @{
                Profile     = $null
                InstallNSSM = $chkNSSM.Checked
                InstallIIS  = $chkIIS.Checked
                InstallARR  = $chkARR.Checked
                ServiceName = $tbService.Text.Trim()
                InstallDir  = $tbInstall.Text.Trim().TrimEnd("\")
                LogDir      = $tbLog.Text.Trim().TrimEnd("\")
                NssmPath    = $tbNssm.Text.Trim()
                APIPort     = [int]$tbPort.Text.Trim()
                GitHubToken = $tbToken.Text
            }
            $form.DialogResult = "OK"
            $form.Close()
        }
    })

    Set-WizardStep 1

    $form.CancelButton = $btnCancel
    $tbPort.Text = $tbPort.Text
    $form.ClientSize = New-Object System.Drawing.Size($clientWidth, $clientHeight)

    if ($form.ShowDialog() -ne "OK") {
        Write-Info "Installation cancelled."
        return $null
    }

    return $form.Tag
}

function Show-ProgressWindow {
    param($Config)

    Initialize-WinForms
    try {
        if (-not (Test-Path $Script:LogPath)) {
            New-Item -ItemType File -Path $Script:LogPath -Force | Out-Null
        }
    } catch {}

    $form = New-Object System.Windows.Forms.Form
    $form.Text = "Watcher - Installing"
    $form.Size = New-Object System.Drawing.Size(760, 560)
    $form.StartPosition = "CenterScreen"
    $form.FormBorderStyle = "FixedDialog"
    $form.MaximizeBox = $false
    $form.MinimizeBox = $true
    $form.BackColor = [System.Drawing.Color]::White

    $title = New-Object System.Windows.Forms.Label
    $title.Text = "Installing Watcher"
    $title.Location = New-Object System.Drawing.Point(20, 18)
    $title.Size = New-Object System.Drawing.Size(300, 28)
    $title.Font = New-Object System.Drawing.Font("Segoe UI", 14)
    $form.Controls.Add($title)

    $statusLabel = New-Object System.Windows.Forms.Label
    $statusLabel.Text = "Preparing installation..."
    $statusLabel.Location = New-Object System.Drawing.Point(20, 52)
    $statusLabel.Size = New-Object System.Drawing.Size(700, 20)
    $statusLabel.Font = New-Object System.Drawing.Font("Segoe UI", 9)
    $form.Controls.Add($statusLabel)

    $progressBar = New-Object System.Windows.Forms.ProgressBar
    $progressBar.Location = New-Object System.Drawing.Point(20, 80)
    $progressBar.Size = New-Object System.Drawing.Size(700, 18)
    $progressBar.Minimum = 0
    $progressBar.Maximum = 100
    $progressBar.Value = 0
    $form.Controls.Add($progressBar)

    $logBox = New-Object System.Windows.Forms.TextBox
    $logBox.Location = New-Object System.Drawing.Point(20, 114)
    $logBox.Size = New-Object System.Drawing.Size(700, 350)
    $logBox.Multiline = $true
    $logBox.ScrollBars = "Vertical"
    $logBox.ReadOnly = $true
    $logBox.Font = New-Object System.Drawing.Font("Consolas", 9)
    $logBox.BackColor = [System.Drawing.Color]::FromArgb(250, 250, 250)
    $form.Controls.Add($logBox)

    $logHint = New-Object System.Windows.Forms.Label
    $logHint.Text = "Debug log: $Script:LogPath"
    $logHint.Location = New-Object System.Drawing.Point(20, 474)
    $logHint.Size = New-Object System.Drawing.Size(520, 18)
    $logHint.Font = New-Object System.Drawing.Font("Segoe UI", 8)
    $logHint.ForeColor = [System.Drawing.Color]::FromArgb(110, 110, 110)
    $form.Controls.Add($logHint)

    $btnOpenLog = New-Object System.Windows.Forms.Button
    $btnOpenLog.Text = "Open Debug Log"
    $btnOpenLog.Location = New-Object System.Drawing.Point(20, 498)
    $btnOpenLog.Size = New-Object System.Drawing.Size(120, 30)
    $btnOpenLog.FlatStyle = "Flat"
    $btnOpenLog.Add_Click({
        if (Test-Path $Script:LogPath) {
            Start-Process -FilePath "notepad.exe" -ArgumentList @($Script:LogPath)
        } else {
            Show-Message -Text ("Debug log file not found:`r`n`r`n{0}" -f $Script:LogPath) -Icon Warning
        }
    })
    $form.Controls.Add($btnOpenLog)

    $btnOpenDashboard = New-Object System.Windows.Forms.Button
    $btnOpenDashboard.Text = "Open Dashboard"
    $btnOpenDashboard.Location = New-Object System.Drawing.Point(520, 498)
    $btnOpenDashboard.Size = New-Object System.Drawing.Size(120, 30)
    $btnOpenDashboard.FlatStyle = "Flat"
    $btnOpenDashboard.Enabled = $false
    $btnOpenDashboard.Add_Click({
        Start-Process ("http://localhost:{0}" -f $Config.APIPort) -ErrorAction SilentlyContinue
    })
    $form.Controls.Add($btnOpenDashboard)

    $btnClose = New-Object System.Windows.Forms.Button
    $btnClose.Text = "Close"
    $btnClose.Location = New-Object System.Drawing.Point(650, 498)
    $btnClose.Size = New-Object System.Drawing.Size(70, 30)
    $btnClose.FlatStyle = "Flat"
    $btnClose.Enabled = $false
    $btnClose.Add_Click({
        $form.DialogResult = "OK"
        $form.Close()
    })
    $form.Controls.Add($btnClose)

    $Script:ProgressUi = @{
        Form           = $form
        StatusLabel    = $statusLabel
        ProgressBar    = $progressBar
        LogBox         = $logBox
        OpenLogButton  = $btnOpenLog
        OpenDashButton = $btnOpenDashboard
        CloseButton    = $btnClose
    }

    $form.Add_Shown({
        try {
            Invoke-Installation -Config $Config
            $summary = Get-InstallSummary -Config $Config
            $Script:ProgressUi.StatusLabel.Text = "Installation completed successfully."
            $Script:ProgressUi.OpenDashButton.Enabled = $true
            $Script:ProgressUi.CloseButton.Enabled = $true
            $Script:ProgressUi.LogBox.AppendText([Environment]::NewLine + $summary + [Environment]::NewLine)
        } catch {
            $errorText = $_.Exception.Message
            if ([string]::IsNullOrWhiteSpace($errorText)) {
                $errorText = "Installation failed, but no error message was returned."
            }
            $detail = ($_ | Out-String).Trim()
            Write-InstallerLog -Level "ERROR" -Message ("Installation failed in progress window: {0}" -f $errorText)
            if (-not [string]::IsNullOrWhiteSpace($detail)) {
                Write-InstallerLog -Level "ERROR" -Message $detail
            }
            $Script:ProgressUi.ProgressBar.Value = 100
            $Script:ProgressUi.StatusLabel.Text = "Installation failed. Review the debug log for details."
            $Script:ProgressUi.LogBox.AppendText([Environment]::NewLine + "ERROR: " + $errorText + [Environment]::NewLine)
            if (-not [string]::IsNullOrWhiteSpace($detail)) {
                $Script:ProgressUi.LogBox.AppendText($detail + [Environment]::NewLine)
            }
            $Script:ProgressUi.CloseButton.Enabled = $true
            Show-Message -Text ("Installation failed.`r`n`r`n{0}`r`n`r`nDebug log:`r`n{1}" -f $errorText, $Script:LogPath) -Icon Error
        }
    })

    [void]$form.ShowDialog()
    if ($DebugMode) {
        exit 0
    }
}

# ==============================================================
# ENTRYPOINT
# ==============================================================
try {
    if ($Silent) {
        $Config = $Defaults.Clone()
    } else {
        $Config = Show-Wizard
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

    if ($Silent) {
        Invoke-Installation -Config $Config
        Write-Info ""
        Write-Info (Get-InstallSummary -Config $Config)
        exit 0
    }

    Show-ProgressWindow -Config $Config
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
