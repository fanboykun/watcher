# ==============================================================
# install-watcher.ps1
# Interactive bootstrap: user selects features via terminal menu,
# then configures installation paths via a GUI wizard.
# Run as Administrator on Windows 10/11 or Windows Server 2022.
# ==============================================================

param(
    [switch]$Silent  # Skip all prompts and use config block defaults (CI use)
)

$ErrorActionPreference = 'Stop'

# ==============================================================
# DEFAULTS -- used by wizard pre-fill and silent mode
# ==============================================================
$Defaults = @{
    ServiceName = "app-watcher"
    InstallDir  = "C:\apps\watcher"
    LogDir      = "C:\apps\watcher\logs"
    NssmPath    = "C:\ProgramData\chocolatey\bin\nssm.exe"
    DBPath      = "C:\apps\watcher\watcher.db"
    APIPort     = "8080"
    GitHubToken = ""
}
# ==============================================================

# ── Helpers ───────────────────────────────────────────────────
function Write-Step { param($msg) Write-Host "`n>>> $msg" -ForegroundColor Cyan }
function Write-OK   { param($msg) Write-Host "  OK: $msg" -ForegroundColor Green }
function Write-Skip { param($msg) Write-Host "  SKIP: $msg (already done)" -ForegroundColor Yellow }
function Write-Fail {
    param($msg)
    Write-Host "  FAIL: $msg" -ForegroundColor Red
    if (-not $Silent) {
        Write-Host "`nPress any key to exit..." -ForegroundColor Gray
        $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
    }
}

# ── Admin check ───────────────────────────────────────────────
if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    if (-not $Silent) {
        Write-Host "Elevating to Administrator..." -ForegroundColor Yellow
        try {
            Start-Process powershell -ArgumentList "-NoProfile -ExecutionPolicy Bypass -File `"$PSCommandPath`"" -Verb RunAs
            exit 0
        } catch {
            Write-Fail "Failed to elevate to Administrator. Please right-click and 'Run as Administrator'."
            exit 1
        }
    } else {
        Write-Fail "Run as Administrator"
        exit 1
    }
}

# ── Detect OS type ────────────────────────────────────────────
$isServer = (Get-CimInstance Win32_OperatingSystem).ProductType -ne 1
$osLabel  = if ($isServer) { "Windows Server" } else { "Windows Desktop" }

# Resolve script location to auto-find watcher.exe
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ParentDir = Split-Path -Parent $ScriptDir


# ==============================================================
# STAGE 1 -- FEATURE SELECTION (terminal menu)
# ==============================================================
$installNSSM = $false
$installIIS  = $false
$installARR  = $false

if ($Silent) {
    # Silent mode: install everything
    $installNSSM = $true
    $installIIS  = $true
    $installARR  = $false
} else {
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
}


# ==============================================================
# STAGE 2 -- CONFIGURATION WIZARD (GUI)
# ==============================================================
function Show-Wizard {
    Add-Type -AssemblyName System.Windows.Forms
    Add-Type -AssemblyName System.Drawing
    [System.Windows.Forms.Application]::EnableVisualStyles()
    [System.Windows.Forms.Application]::SetCompatibleTextRenderingDefault($false)

    function New-Label {
        param($Text, $X, $Y, $Width = 160, $Height = 20)
        $l = New-Object System.Windows.Forms.Label
        $l.Text = $Text; $l.Location = New-Object System.Drawing.Point($X, $Y)
        $l.Size = New-Object System.Drawing.Size($Width, $Height)
        $l.Font = New-Object System.Drawing.Font("Segoe UI", 9)
        return $l
    }

    function New-TextBox {
        param($Text, $X, $Y, $Width = 280)
        $t = New-Object System.Windows.Forms.TextBox
        $t.Text = $Text; $t.Location = New-Object System.Drawing.Point($X, $Y)
        $t.Size = New-Object System.Drawing.Size($Width, 24)
        $t.Font = New-Object System.Drawing.Font("Segoe UI", 9)
        return $t
    }

    function New-BrowseFolder {
        param($Btn, $Tb)
        $Btn.Add_Click({
            $d = New-Object System.Windows.Forms.FolderBrowserDialog
            $d.SelectedPath = $Tb.Text
            if ($d.ShowDialog() -eq "OK") { $Tb.Text = $d.SelectedPath }
        })
    }

    function New-StatusLabel {
        param($X, $Y, $Width = 400)
        $l = New-Object System.Windows.Forms.Label
        $l.Text = ""; $l.Location = New-Object System.Drawing.Point($X, $Y)
        $l.Size = New-Object System.Drawing.Size($Width, 16)
        $l.Font = New-Object System.Drawing.Font("Segoe UI", 8)
        $l.ForeColor = [System.Drawing.Color]::FromArgb(180, 0, 0)
        return $l
    }

    # ── Form ──────────────────────────────────────────────────
    $form = New-Object System.Windows.Forms.Form
    $form.Text            = "Watcher -- Configuration"
    $form.Size            = New-Object System.Drawing.Size(520, 600)
    $form.StartPosition   = "CenterScreen"
    $form.FormBorderStyle = "FixedDialog"
    $form.MaximizeBox     = $false
    $form.MinimizeBox     = $false
    $form.BackColor       = [System.Drawing.Color]::White

    # Header
    $header = New-Object System.Windows.Forms.Panel
    $header.Size      = New-Object System.Drawing.Size(520, 64)
    $header.Location  = New-Object System.Drawing.Point(0, 0)
    $header.BackColor = [System.Drawing.Color]::FromArgb(24, 24, 24)
    $form.Controls.Add($header)

    $ht = New-Object System.Windows.Forms.Label
    $ht.Text = "Configure Installation"; $ht.Location = New-Object System.Drawing.Point(20, 10)
    $ht.Size = New-Object System.Drawing.Size(460, 26)
    $ht.Font = New-Object System.Drawing.Font("Segoe UI", 13)
    $ht.ForeColor = [System.Drawing.Color]::White
    $header.Controls.Add($ht)

    $hs = New-Object System.Windows.Forms.Label
    $hs.Text = "Set your paths, service name, port, and token then click Install."
    $hs.Location = New-Object System.Drawing.Point(20, 38)
    $hs.Size = New-Object System.Drawing.Size(460, 18)
    $hs.Font = New-Object System.Drawing.Font("Segoe UI", 9)
    $hs.ForeColor = [System.Drawing.Color]::FromArgb(170, 170, 170)
    $header.Controls.Add($hs)

    $y = 80

    # Install directory
    $form.Controls.Add((New-Label "Install directory" 20 $y))
    $tbInstall = New-TextBox $Defaults.InstallDir 20 ($y + 22)
    $form.Controls.Add($tbInstall)
    $btnBI = New-Object System.Windows.Forms.Button
    $btnBI.Text = "..."; $btnBI.Location = New-Object System.Drawing.Point(305, ($y+22))
    $btnBI.Size = New-Object System.Drawing.Size(32, 24); $btnBI.FlatStyle = "Flat"
    New-BrowseFolder $btnBI $tbInstall
    $form.Controls.Add($btnBI)
    $lblInstallErr = New-StatusLabel 20 ($y + 50)
    $form.Controls.Add($lblInstallErr)
    $y += 74

    # Log directory
    $form.Controls.Add((New-Label "Log directory" 20 $y))
    $tbLog = New-TextBox $Defaults.LogDir 20 ($y + 22)
    $form.Controls.Add($tbLog)
    $btnBL = New-Object System.Windows.Forms.Button
    $btnBL.Text = "..."; $btnBL.Location = New-Object System.Drawing.Point(305, ($y+22))
    $btnBL.Size = New-Object System.Drawing.Size(32, 24); $btnBL.FlatStyle = "Flat"
    New-BrowseFolder $btnBL $tbLog
    $form.Controls.Add($btnBL)
    $y += 56

    # Service name
    $form.Controls.Add((New-Label "Windows service name" 20 $y))
    $tbService = New-TextBox $Defaults.ServiceName 20 ($y + 22)
    $form.Controls.Add($tbService)
    $lblSvcErr = New-StatusLabel 20 ($y + 50)
    $form.Controls.Add($lblSvcErr)
    $y += 74

    # API port
    $form.Controls.Add((New-Label "API / dashboard port" 20 $y))
    $tbPort = New-TextBox $Defaults.APIPort 20 ($y + 22) 80
    $form.Controls.Add($tbPort)
    $lblPortStatus = New-StatusLabel 108 ($y + 26) 280
    $form.Controls.Add($lblPortStatus)

    $tbPort.Add_TextChanged({
        $p = $tbPort.Text.Trim()
        if ($p -match '^\d+$' -and [int]$p -ge 1 -and [int]$p -le 65535) {
            $inUse = netstat -ano 2>$null | Select-String ":$p\s" | Select-String "LISTEN"
            if ($inUse) {
                $lblPortStatus.Text = "Port $p is already in use"
                $lblPortStatus.ForeColor = [System.Drawing.Color]::FromArgb(200, 100, 0)
            } else {
                $lblPortStatus.Text = "Port $p is available"
                $lblPortStatus.ForeColor = [System.Drawing.Color]::FromArgb(0, 130, 0)
            }
        } else {
            $lblPortStatus.Text = "Enter a number between 1 and 65535"
            $lblPortStatus.ForeColor = [System.Drawing.Color]::FromArgb(180, 0, 0)
        }
    })
    $y += 56

    # NSSM path (only shown if NSSM was selected)
    $tbNssm = $null
    $lblNssmErr = $null
    if ($installNSSM) {
        $form.Controls.Add((New-Label "NSSM path" 20 $y))
        $tbNssm = New-TextBox $Defaults.NssmPath 20 ($y + 22)
        $form.Controls.Add($tbNssm)
        $btnBN = New-Object System.Windows.Forms.Button
        $btnBN.Text = "..."; $btnBN.Location = New-Object System.Drawing.Point(305, ($y+22))
        $btnBN.Size = New-Object System.Drawing.Size(32, 24); $btnBN.FlatStyle = "Flat"
        $btnBN.Add_Click({
            $d = New-Object System.Windows.Forms.OpenFileDialog
            $d.Filter = "NSSM executable|nssm.exe"
            if ($d.ShowDialog() -eq "OK") { $tbNssm.Text = $d.FileName }
        })
        $form.Controls.Add($btnBN)
        $lblNssmErr = New-StatusLabel 20 ($y + 50)
        $form.Controls.Add($lblNssmErr)
        $y += 74
    }

    # GitHub token
    $form.Controls.Add((New-Label "GitHub token (PAT)" 20 $y))
    $tokenNote = New-Object System.Windows.Forms.Label
    $tokenNote.Text = "Required for private repos. Leave empty for public repos."
    $tokenNote.Location = New-Object System.Drawing.Point(20, ($y + 16))
    $tokenNote.Size = New-Object System.Drawing.Size(440, 16)
    $tokenNote.Font = New-Object System.Drawing.Font("Segoe UI", 8)
    $tokenNote.ForeColor = [System.Drawing.Color]::FromArgb(110, 110, 110)
    $form.Controls.Add($tokenNote)
    $tbToken = New-Object System.Windows.Forms.TextBox
    $tbToken.Text = $Defaults.GitHubToken
    $tbToken.Location = New-Object System.Drawing.Point(20, ($y + 34))
    $tbToken.Size = New-Object System.Drawing.Size(270, 24)
    $tbToken.Font = New-Object System.Drawing.Font("Segoe UI", 9)
    $tbToken.PasswordChar = [char]0x2022
    $form.Controls.Add($tbToken)
    $chkShow = New-Object System.Windows.Forms.CheckBox
    $chkShow.Text = "Show"; $chkShow.Location = New-Object System.Drawing.Point(296, ($y + 35))
    $chkShow.Size = New-Object System.Drawing.Size(60, 20)
    $chkShow.Add_CheckedChanged({
        $tbToken.PasswordChar = if ($chkShow.Checked) { [char]0 } else { [char]0x2022 }
    })
    $form.Controls.Add($chkShow)
    $y += 64

    # Separator
    $sep = New-Object System.Windows.Forms.Panel
    $sep.Size = New-Object System.Drawing.Size(480, 1)
    $sep.Location = New-Object System.Drawing.Point(20, ($y + 8))
    $sep.BackColor = [System.Drawing.Color]::FromArgb(220, 220, 220)
    $form.Controls.Add($sep)

    # Feature summary label
    $featureSummary = @()
    if ($installNSSM) { $featureSummary += "NSSM" }
    if ($installIIS)  { $featureSummary += "IIS" }
    if ($installARR)  { $featureSummary += "ARR" }
    $fLabel = New-Object System.Windows.Forms.Label
    $fLabel.Text = "Installing: " + ($featureSummary -join " + ")
    $fLabel.Location = New-Object System.Drawing.Point(20, ($y + 18))
    $fLabel.Size = New-Object System.Drawing.Size(260, 20)
    $fLabel.Font = New-Object System.Drawing.Font("Segoe UI", 9)
    $fLabel.ForeColor = [System.Drawing.Color]::FromArgb(0, 130, 0)
    $form.Controls.Add($fLabel)

    # Buttons
    $btnInstall = New-Object System.Windows.Forms.Button
    $btnInstall.Text = "Install"
    $btnInstall.Location = New-Object System.Drawing.Point(300, ($y + 14))
    $btnInstall.Size = New-Object System.Drawing.Size(90, 32)
    $btnInstall.BackColor = [System.Drawing.Color]::FromArgb(24, 24, 24)
    $btnInstall.ForeColor = [System.Drawing.Color]::White
    $btnInstall.FlatStyle = "Flat"
    $form.Controls.Add($btnInstall)

    $btnCancel = New-Object System.Windows.Forms.Button
    $btnCancel.Text = "Cancel"
    $btnCancel.Location = New-Object System.Drawing.Point(398, ($y + 14))
    $btnCancel.Size = New-Object System.Drawing.Size(80, 32)
    $btnCancel.FlatStyle = "Flat"
    $btnCancel.Add_Click({ $form.DialogResult = "Cancel"; $form.Close() })
    $form.Controls.Add($btnCancel)

    # Adjust form height to content
    $form.ClientSize = New-Object System.Drawing.Size(500, ($y + 60))

    # Validation
    $btnInstall.Add_Click({
        $ok = $true

        $lblInstallErr.Text = ""
        if ([string]::IsNullOrWhiteSpace($tbInstall.Text)) {
            $lblInstallErr.Text = "Install directory is required"
            $ok = $false
        }

        $lblSvcErr.ForeColor = [System.Drawing.Color]::FromArgb(180, 0, 0)
        $lblSvcErr.Text = ""
        if ([string]::IsNullOrWhiteSpace($tbService.Text)) {
            $lblSvcErr.Text = "Service name is required"
            $ok = $false
        } elseif ($tbService.Text -match '[\\/:*?"<>|]') {
            $lblSvcErr.Text = "Service name contains invalid characters"
            $ok = $false
        } else {
            $existing = Get-Service $tbService.Text -ErrorAction SilentlyContinue
            if ($existing) {
                $lblSvcErr.Text = "Service already exists -- will be updated"
                $lblSvcErr.ForeColor = [System.Drawing.Color]::FromArgb(200, 100, 0)
            }
        }

        if ($null -ne $lblNssmErr) {
            $lblNssmErr.Text = ""
            if ([string]::IsNullOrWhiteSpace($tbNssm.Text)) {
                $lblNssmErr.Text = "NSSM path is required"
                $ok = $false
            } elseif (-not (Test-Path $tbNssm.Text)) {
                $lblNssmErr.Text = "Not found -- will install via Chocolatey"
                $lblNssmErr.ForeColor = [System.Drawing.Color]::FromArgb(200, 100, 0)
            }
        }

        $p = $tbPort.Text.Trim()
        if (-not ($p -match '^\d+$') -or [int]$p -lt 1 -or [int]$p -gt 65535) {
            $lblPortStatus.Text = "Enter a valid port number"
            $lblPortStatus.ForeColor = [System.Drawing.Color]::FromArgb(180, 0, 0)
            $ok = $false
        }

        if ($ok) { $form.DialogResult = "OK"; $form.Close() }
    })

    $form.AcceptButton = $btnInstall
    $form.CancelButton = $btnCancel
    $tbPort.Text = $tbPort.Text  # trigger port check on load

    if ($form.ShowDialog() -ne "OK") {
        Write-Host "Installation cancelled." -ForegroundColor Yellow
        exit 0
    }

    return @{
        ServiceName = $tbService.Text.Trim()
        InstallDir  = $tbInstall.Text.Trim().TrimEnd("\")
        LogDir      = $tbLog.Text.Trim().TrimEnd("\")
        NssmPath    = if ($tbNssm) { $tbNssm.Text.Trim() } else { $Defaults.NssmPath }
        APIPort     = [int]$tbPort.Text.Trim()
        GitHubToken = $tbToken.Text
    }
}

# Collect config
if ($Silent) {
    $Config = @{
        ServiceName = $Defaults.ServiceName
        InstallDir  = $Defaults.InstallDir
        LogDir      = $Defaults.LogDir
        NssmPath    = $Defaults.NssmPath
        APIPort     = [int]$Defaults.APIPort
        GitHubToken = $Defaults.GitHubToken
    }
} else {
    $Config = Show-Wizard
}

# Derive paths
$Config.WatcherExe   = Join-Path $Config.InstallDir "watcher.exe"
$Config.EnvFile      = Join-Path $Config.InstallDir ".env"
$Config.DBPath       = Join-Path $Config.InstallDir "watcher.db"
$Config.RestartDelay = 5000

Write-Host ""
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "  STARTING INSTALLATION" -ForegroundColor Cyan
Write-Host "  Service  : $($Config.ServiceName)" -ForegroundColor Gray
Write-Host "  Dir      : $($Config.InstallDir)" -ForegroundColor Gray
Write-Host "  API Port : $($Config.APIPort)" -ForegroundColor Gray
Write-Host "============================================================" -ForegroundColor Cyan


# ==============================================================
# [1] Chocolatey
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
        $env:PATH = [System.Environment]::GetEnvironmentVariable("PATH","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("PATH","User")
        if (-not (Get-Command choco -ErrorAction SilentlyContinue)) { Write-Fail "Chocolatey installation failed"; exit 1 }
        Write-OK "Chocolatey installed (version $(choco --version))"
    }
} else {
    Write-Step "[1] Chocolatey -- skipped (not needed)"
}


# ==============================================================
# [2] NSSM
# ==============================================================
if ($installNSSM) {
    Write-Step "[2] NSSM"
    if (Test-Path $Config.NssmPath) {
        Write-Skip "NSSM already installed at $($Config.NssmPath)"
    } else {
        Write-Host "  Installing NSSM via Chocolatey..." -ForegroundColor Yellow
        choco install nssm -y --force
        if (-not (Test-Path $Config.NssmPath)) { Write-Fail "NSSM installation failed -- not found at $($Config.NssmPath)"; exit 1 }
        Write-OK "NSSM installed"
    }
} else {
    Write-Step "[2] NSSM -- skipped"
}


# ==============================================================
# [3] IIS features
# ==============================================================
if ($installIIS) {
    Write-Step "[3] IIS features"

    if ($isServer) {
        $iisFeatures = @(
            "Web-Server","Web-WebServer","Web-Common-Http","Web-Default-Doc",
            "Web-Static-Content","Web-Http-Errors","Web-Http-Redirect",
            "Web-Health","Web-Http-Logging","Web-Request-Monitor","Web-Http-Tracing",
            "Web-Performance","Web-Stat-Compression","Web-Dyn-Compression",
            "Web-Security","Web-Filtering","Web-Mgmt-Tools","Web-Mgmt-Console",
            "Web-Scripting-Tools"
        )
        foreach ($f in $iisFeatures) {
            if ((Get-WindowsFeature -Name $f).Installed) { Write-Skip $f }
            else { Install-WindowsFeature -Name $f | Out-Null; Write-OK "$f installed" }
        }
    } else {
        $iisFeatures = @(
            "IIS-WebServerRole","IIS-WebServer","IIS-CommonHttpFeatures",
            "IIS-DefaultDocument","IIS-StaticContent","IIS-HttpErrors",
            "IIS-HttpRedirect","IIS-HealthAndDiagnostics","IIS-HttpLogging",
            "IIS-RequestMonitor","IIS-HttpTracing","IIS-Performance",
            "IIS-HttpCompressionStatic","IIS-HttpCompressionDynamic",
            "IIS-Security","IIS-RequestFiltering","IIS-ManagementConsole",
            "IIS-ManagementScriptingTools"
        )
        foreach ($f in $iisFeatures) {
            $state = Get-WindowsOptionalFeature -Online -FeatureName $f -ErrorAction SilentlyContinue
            if ($state -and $state.State -eq "Enabled") { Write-Skip $f }
            else { Enable-WindowsOptionalFeature -Online -FeatureName $f -All -NoRestart | Out-Null; Write-OK "$f enabled" }
        }
    }

    Write-Step "[3b] URL Rewrite"
    if (Test-Path "C:\Windows\System32\inetsrv\rewrite.dll") {
        Write-Skip "URL Rewrite already installed"
    } else {
        choco install urlrewrite -y --force
        Write-OK "URL Rewrite installed"
    }
} else {
    Write-Step "[3] IIS -- skipped"
}


# ==============================================================
# [4] ARR
# ==============================================================
if ($installARR) {
    Write-Step "[4] ARR (Application Request Routing)"
    if (Test-Path "C:\Windows\System32\inetsrv\arr.dll") {
        Write-Skip "ARR already installed"
    } else {
        choco install iis-arr -y --force
        Write-OK "ARR installed"
    }

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
    Write-Step "[4] ARR -- skipped"
}


# ==============================================================
# [5] Create directories
# ==============================================================
Write-Step "[5] Creating directories"

@($Config.InstallDir, $Config.LogDir) | ForEach-Object {
    if (Test-Path $_) { Write-Skip $_ }
    else { New-Item -ItemType Directory -Path $_ -Force | Out-Null; Write-OK "Created $_" }
}


# ==============================================================
# [6] Copy watcher.exe if needed
# ==============================================================
Write-Step "[6] Preflight checks"

$sourceExe = Join-Path $ParentDir "watcher.exe"
if (-not (Test-Path $Config.WatcherExe) -and (Test-Path $sourceExe)) {
    Copy-Item $sourceExe $Config.WatcherExe
    Write-OK "Copied watcher.exe to $($Config.InstallDir)"
}

if (-not (Test-Path $Config.WatcherExe)) {
    Write-Fail "watcher.exe not found at $($Config.WatcherExe)"
    Write-Host "  Expected: $sourceExe" -ForegroundColor Yellow
    exit 1
}
Write-OK "watcher.exe found"


# ==============================================================
# [7] Write .env file
# ==============================================================
Write-Step "[7] Environment config (.env)"

if (Test-Path $Config.EnvFile) {
    Write-Skip ".env already exists at $($Config.EnvFile) -- not overwriting"
    Write-Host "  To reconfigure, delete .env and re-run this script" -ForegroundColor Yellow
} else {
    $nssmLine = if ($installNSSM) { "NSSM_PATH=$($Config.NssmPath)" } else { "# NSSM_PATH= (NSSM not installed)" }
    $envContent = @"
ENVIRONMENT=production
GITHUB_TOKEN=$($Config.GitHubToken)
LOG_DIR=$($Config.LogDir)
$nssmLine
DB_PATH=$($Config.DBPath)
API_PORT=$($Config.APIPort)
"@
    [System.IO.File]::WriteAllText($Config.EnvFile, $envContent, [System.Text.Encoding]::ASCII)
    Write-OK ".env created at $($Config.EnvFile)"
    if ([string]::IsNullOrWhiteSpace($Config.GitHubToken)) {
        Write-Host "  GITHUB_TOKEN is empty -- required for private repos" -ForegroundColor Yellow
    }
}


# ==============================================================
# [8] Secure .env
# ==============================================================
Write-Step "[8] Securing .env permissions"

icacls $Config.EnvFile /inheritance:r | Out-Null
icacls $Config.EnvFile /grant "SYSTEM:(F)" | Out-Null
icacls $Config.EnvFile /grant "BUILTIN\Administrators:(F)" | Out-Null
Write-OK ".env restricted to SYSTEM and Administrators only"


# ==============================================================
# [9] Outbound HTTPS
# ==============================================================
Write-Step "[9] Outbound HTTPS to github.com"

try {
    $resp = Invoke-WebRequest -Uri "https://github.com" -UseBasicParsing -TimeoutSec 10
    if ($resp.StatusCode -eq 200) { Write-OK "github.com reachable (HTTP $($resp.StatusCode))" }
    else { Write-Fail "github.com returned HTTP $($resp.StatusCode)"; exit 1 }
} catch {
    Write-Fail "Cannot reach github.com -- check firewall or proxy settings"
    Write-Host "  Error: $_" -ForegroundColor Red
    exit 1
}


# ==============================================================
# [10] Register watcher NSSM service
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
        Write-Host "  Updating existing service"
        $out = & $Config.NssmPath set $Config.ServiceName Application `"$($Config.WatcherExe)`" 2>&1
        Write-Host "  NSSM output: $out"
    } else {
        Write-Host "  Registering new service: $($Config.ServiceName)"
        $out = & $Config.NssmPath install $Config.ServiceName `"$($Config.WatcherExe)`" 2>&1
        Write-Host "  NSSM output: $out"
        $created = Get-Service $Config.ServiceName -ErrorAction SilentlyContinue
        if (-not $created) {
            Write-Fail "NSSM install ran but service was not created"
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

    Write-Step "[11] Starting $($Config.ServiceName)"
    & $Config.NssmPath start $Config.ServiceName
    Start-Sleep 4

    $svc = Get-Service $Config.ServiceName -ErrorAction SilentlyContinue
    if ($svc -and $svc.Status -eq "Running") {
        Write-OK "Service is running"
    } else {
        Write-Fail "Service did not start -- check logs at $($Config.LogDir)"
        exit 1
    }
} else {
    Write-Step "[10] NSSM service registration -- skipped (NSSM not selected)"
    Write-Host "  Run watcher.exe manually or register it as a service separately" -ForegroundColor Yellow
}


# ==============================================================
# [12] Verify API
# ==============================================================
Write-Step "[12] Verifying API is responding"

Start-Sleep 2
try {
    $r = Invoke-WebRequest -Uri "http://localhost:$($Config.APIPort)/api/status" -UseBasicParsing -TimeoutSec 5
    Write-OK "API is up (HTTP $($r.StatusCode))"
} catch {
    Write-Host "  WARN: API not responding yet -- may still be starting up" -ForegroundColor Yellow
}


# ==============================================================
# DONE
# ==============================================================
Write-Host ""
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "  WATCHER INSTALLED SUCCESSFULLY" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Dashboard  : http://localhost:$($Config.APIPort)" -ForegroundColor Green
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

try {
    Start-Process "http://localhost:$($Config.APIPort)" -ErrorAction SilentlyContinue
} catch {}