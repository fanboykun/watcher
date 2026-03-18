# ============================================================
# IIS + ARR REVERSE PROXY SETUP
# Listens on port 8080, round robin to :8000 and :8001
# Run as Administrator on Windows Server 2022# ==============================================================
# 2-setup-iis-arr.ps1
# IIS + ARR reverse proxy setup
# Run as Administrator on Windows Server 2022
# ==============================================================

# ==============================================================
# CONFIGURATION — edit this block to reuse for any project
# ==============================================================

$Config = @{
    # IIS site
    SiteName        = "admin-be-proxy"          # IIS site name
    SiteDir         = "C:\inetpub\admin-be-proxy" # Physical path for IIS site
    ProxyPort       = 8080                      # Public-facing port IIS listens on

    # ARR server farm
    FarmName        = "admin-be-farm"           # ARR upstream farm name
    FarmAlgorithm   = "RoundRobin"              # RoundRobin | WeightedRoundRobin | LeastRequests | WeightedTotalTraffic | IpHash | RequestHash

    # Backend instances (address + port pairs)
    # Add or remove entries here to match your WEB_INSTANCE_COUNT
    BackendServers  = @(
        @{ Address = "localhost"; Port = 8000 },
        @{ Address = "localhost"; Port = 8001 }
    )

    # Rewrite rule
    RewriteRuleName = "ARR-to-farm"             # URL rewrite rule name

    # Firewall
    FirewallRule    = "IIS-ARR-Proxy"           # Firewall rule display name
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
Write-Host "  IIS + ARR REVERSE PROXY SETUP" -ForegroundColor Cyan
Write-Host "  Site       : $($Config.SiteName) on port $($Config.ProxyPort)" -ForegroundColor Gray
Write-Host "  Farm       : $($Config.FarmName) ($($Config.FarmAlgorithm))" -ForegroundColor Gray
Write-Host "  Backends   : $($Config.BackendServers.Count) server(s)" -ForegroundColor Gray
Write-Host "============================================================" -ForegroundColor Cyan


# ── 1. IIS features ───────────────────────────────────────────────────────────
Write-Step "Installing IIS features"

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


# ── 2. URL Rewrite + ARR ──────────────────────────────────────────────────────
Write-Step "Installing URL Rewrite and ARR"

if (Test-Path "C:\Windows\System32\inetsrv\rewrite.dll") {
    Write-Skip "URL Rewrite already installed"
} else {
    choco install urlrewrite -y --force
    Write-OK "URL Rewrite installed"
}

if (Test-Path "C:\Windows\System32\inetsrv\arr.dll") {
    Write-Skip "ARR already installed"
} else {
    choco install iis-arr -y --force
    Write-OK "ARR installed"
}


# ── 3. WebAdministration module ───────────────────────────────────────────────
Write-Step "Loading WebAdministration module"
Import-Module WebAdministration
Write-OK "Loaded"


# ── 4. Enable ARR proxy ───────────────────────────────────────────────────────
Write-Step "Enabling ARR proxy"

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


# ── 5. Server farm ────────────────────────────────────────────────────────────
Write-Step "Creating server farm: $($Config.FarmName)"

$existingFarm = Get-WebConfigurationProperty `
    -PSPath "MACHINE/WEBROOT/APPHOST" `
    -Filter "webFarms/webFarm[@name='$($Config.FarmName)']" `
    -Name "name" -ErrorAction SilentlyContinue

if ($existingFarm) {
    Write-Skip "Farm $($Config.FarmName) already exists"
} else {
    Add-WebConfiguration -PSPath "MACHINE/WEBROOT/APPHOST" -Filter "webFarms" -Value @{ name = $Config.FarmName }
    Write-OK "Farm $($Config.FarmName) created"
}

foreach ($srv in $Config.BackendServers) {
    $existingSrv = Get-WebConfigurationProperty `
        -PSPath "MACHINE/WEBROOT/APPHOST" `
        -Filter "webFarms/webFarm[@name='$($Config.FarmName)']/server[@address='$($srv.Address)']" `
        -Name "address" -ErrorAction SilentlyContinue

    if ($existingSrv) {
        Write-Skip "Server $($srv.Address):$($srv.Port) already in farm"
    } else {
        Add-WebConfiguration `
            -PSPath "MACHINE/WEBROOT/APPHOST" `
            -Filter "webFarms/webFarm[@name='$($Config.FarmName)']" `
            -Value @{ address = $srv.Address; enabled = $true }

        Set-WebConfigurationProperty `
            -PSPath "MACHINE/WEBROOT/APPHOST" `
            -Filter "webFarms/webFarm[@name='$($Config.FarmName)']/server[@address='$($srv.Address)']" `
            -Name "applicationRequestRouting.httpPort" -Value $srv.Port

        Write-OK "Added $($srv.Address):$($srv.Port)"
    }
}


# ── 6. Load balancing algorithm ───────────────────────────────────────────────
Write-Step "Setting load balancing: $($Config.FarmAlgorithm)"

Set-WebConfigurationProperty `
    -PSPath "MACHINE/WEBROOT/APPHOST" `
    -Filter "webFarms/webFarm[@name='$($Config.FarmName)']/applicationRequestRouting/protocol" `
    -Name "loadBalancing.algorithm" -Value $Config.FarmAlgorithm

Write-OK "$($Config.FarmAlgorithm) set"


# ── 7. IIS site ───────────────────────────────────────────────────────────────
Write-Step "Creating IIS site: $($Config.SiteName) on port $($Config.ProxyPort)"

if (-not (Test-Path $Config.SiteDir)) {
    New-Item -ItemType Directory -Path $Config.SiteDir -Force | Out-Null
    Write-OK "Created $($Config.SiteDir)"
} else {
    Write-Skip "$($Config.SiteDir) exists"
}

$existingSite = Get-Website -Name $Config.SiteName -ErrorAction SilentlyContinue
if ($existingSite) {
    Write-Skip "Site $($Config.SiteName) already exists"
} else {
    New-Website -Name $Config.SiteName -PhysicalPath $Config.SiteDir -Port $Config.ProxyPort -Force | Out-Null
    Write-OK "Site $($Config.SiteName) created"
}

Start-Website -Name $Config.SiteName -ErrorAction SilentlyContinue
Write-OK "Site started"


# ── 8. URL Rewrite rule ───────────────────────────────────────────────────────
Write-Step "Adding rewrite rule: $($Config.RewriteRuleName)"

$existingRule = Get-WebConfigurationProperty `
    -PSPath "IIS:\Sites\$($Config.SiteName)" `
    -Filter "system.webServer/rewrite/rules/rule[@name='$($Config.RewriteRuleName)']" `
    -Name "name" -ErrorAction SilentlyContinue

if ($existingRule) {
    Write-Skip "Rewrite rule already exists"
} else {
    Add-WebConfigurationProperty `
        -PSPath "IIS:\Sites\$($Config.SiteName)" `
        -Filter "system.webServer/rewrite/rules" `
        -Name "." -Value @{ name = $Config.RewriteRuleName; stopProcessing = "True" }

    Set-WebConfigurationProperty `
        -PSPath "IIS:\Sites\$($Config.SiteName)" `
        -Filter "system.webServer/rewrite/rules/rule[@name='$($Config.RewriteRuleName)']/match" `
        -Name "url" -Value "(.*)"

    Set-WebConfigurationProperty `
        -PSPath "IIS:\Sites\$($Config.SiteName)" `
        -Filter "system.webServer/rewrite/rules/rule[@name='$($Config.RewriteRuleName)']/action" `
        -Name "type" -Value "Rewrite"

    Set-WebConfigurationProperty `
        -PSPath "IIS:\Sites\$($Config.SiteName)" `
        -Filter "system.webServer/rewrite/rules/rule[@name='$($Config.RewriteRuleName)']/action" `
        -Name "url" -Value "http://$($Config.FarmName)/{R:1}"

    Write-OK "Rewrite rule added"
}


# ── 9. Firewall ───────────────────────────────────────────────────────────────
Write-Step "Opening firewall port $($Config.ProxyPort)"

$fwRule = Get-NetFirewallRule -DisplayName $Config.FirewallRule -ErrorAction SilentlyContinue
if ($fwRule) {
    Write-Skip "Firewall rule already exists"
} else {
    New-NetFirewallRule `
        -Name        $Config.FirewallRule `
        -DisplayName $Config.FirewallRule `
        -Enabled     True `
        -Direction   Inbound `
        -Protocol    TCP `
        -Action      Allow `
        -LocalPort   $Config.ProxyPort | Out-Null
    Write-OK "Firewall rule created for port $($Config.ProxyPort)"
}


# ── 10. Restart IIS ───────────────────────────────────────────────────────────
Write-Step "Restarting IIS"
iisreset /restart | Out-Null
Write-OK "IIS restarted"


# ── Done ──────────────────────────────────────────────────────────────────────
Write-Host "`n============================================================" -ForegroundColor Cyan
Write-Host "  IIS + ARR SETUP COMPLETE" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Traffic flow:" -ForegroundColor Yellow
Write-Host "  :$($Config.ProxyPort) -> IIS -> ARR ($($Config.FarmAlgorithm))"
foreach ($srv in $Config.BackendServers) {
    Write-Host "    -> $($srv.Address):$($srv.Port)"
}
Write-Host ""
Write-Host "Next: run 3-setup-services.ps1" -ForegroundColor Yellow
Write-Host ""
# ============================================================

$ErrorActionPreference = 'Stop'

function Write-Step { param($msg) Write-Host "`n>>> $msg" -ForegroundColor Cyan }
function Write-OK   { param($msg) Write-Host "  OK: $msg" -ForegroundColor Green }
function Write-Skip { param($msg) Write-Host "  SKIP: $msg (already done)" -ForegroundColor Yellow }
function Write-Fail { param($msg) Write-Host "  FAIL: $msg" -ForegroundColor Red }

# Must run as admin
if (-not ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Fail "Please run this script as Administrator"
    exit 1
}

Write-Host "`n============================================================" -ForegroundColor Cyan
Write-Host "  IIS + ARR REVERSE PROXY SETUP" -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan


# ============================================================
# STEP 1 - Install IIS with required features
# ============================================================
Write-Step "Installing IIS and required features"

$iisFeatures = @(
    "Web-Server",
    "Web-WebServer",
    "Web-Common-Http",
    "Web-Default-Doc",
    "Web-Static-Content",
    "Web-Http-Errors",
    "Web-Http-Redirect",
    "Web-Health",
    "Web-Http-Logging",
    "Web-Request-Monitor",
    "Web-Http-Tracing",
    "Web-Performance",
    "Web-Stat-Compression",
    "Web-Dyn-Compression",
    "Web-Security",
    "Web-Filtering",
    "Web-Mgmt-Tools",
    "Web-Mgmt-Console",
    "Web-Scripting-Tools"
)

foreach ($feature in $iisFeatures) {
    $state = Get-WindowsFeature -Name $feature
    if ($state.Installed) {
        Write-Skip "$feature"
    } else {
        Install-WindowsFeature -Name $feature | Out-Null
        Write-OK "$feature installed"
    }
}


# ============================================================
# STEP 2 - Install Web Platform Installer + ARR + URL Rewrite
# ============================================================
Write-Step "Installing ARR and URL Rewrite via Chocolatey"

# URL Rewrite
$urlRewritePath = "C:\Windows\System32\inetsrv\rewrite.dll"
if (Test-Path $urlRewritePath) {
    Write-Skip "URL Rewrite already installed"
} else {
    choco install urlrewrite -y --force
    Write-OK "URL Rewrite installed"
}

# ARR
$arrPath = "C:\Windows\System32\inetsrv\arr.dll"
if (Test-Path $arrPath) {
    Write-Skip "ARR already installed"
} else {
    choco install iis-arr -y --force
    Write-OK "ARR installed"
}


# ============================================================
# STEP 3 - Load WebAdministration module
# ============================================================
Write-Step "Loading WebAdministration module"

Import-Module WebAdministration
Write-OK "WebAdministration module loaded"


# ============================================================
# STEP 4 - Enable ARR Proxy
# ============================================================
Write-Step "Enabling ARR Proxy"

$arrProxyEnabled = Get-WebConfigurationProperty `
    -PSPath "MACHINE/WEBROOT/APPHOST" `
    -Filter "system.webServer/proxy" `
    -Name "enabled" `
    -ErrorAction SilentlyContinue

if ($arrProxyEnabled.Value -eq $true) {
    Write-Skip "ARR proxy already enabled"
} else {
    Set-WebConfigurationProperty `
        -PSPath "MACHINE/WEBROOT/APPHOST" `
        -Filter "system.webServer/proxy" `
        -Name "enabled" `
        -Value "True"
    Write-OK "ARR proxy enabled"
}


# ============================================================
# STEP 5 - Create Server Farm (upstream group)
# ============================================================
Write-Step "Creating Server Farm: admin-be-farm"

$farmName = "admin-be-farm"
$existingFarm = Get-WebConfigurationProperty `
    -PSPath "MACHINE/WEBROOT/APPHOST" `
    -Filter "webFarms/webFarm[@name='$farmName']" `
    -Name "name" `
    -ErrorAction SilentlyContinue

if ($existingFarm) {
    Write-Skip "Server farm $farmName already exists"
} else {
    # Add the farm
    Add-WebConfiguration -PSPath "MACHINE/WEBROOT/APPHOST" -Filter "webFarms" -Value @{name=$farmName}
    Write-OK "Server farm $farmName created"
}

# Add servers to the farm (port 8000 and 8001)
$servers = @(
    @{ address = "localhost"; port = 8000 },
    @{ address = "localhost"; port = 8001 }
)

foreach ($srv in $servers) {
    $existingSrv = Get-WebConfigurationProperty `
        -PSPath "MACHINE/WEBROOT/APPHOST" `
        -Filter "webFarms/webFarm[@name='$farmName']/server[@address='$($srv.address)']" `
        -Name "address" `
        -ErrorAction SilentlyContinue

    if ($existingSrv) {
        Write-Skip "Server $($srv.address):$($srv.port) already in farm"
    } else {
        Add-WebConfiguration `
            -PSPath "MACHINE/WEBROOT/APPHOST" `
            -Filter "webFarms/webFarm[@name='$farmName']" `
            -Value @{ address = $srv.address; enabled = $true }

        Set-WebConfigurationProperty `
            -PSPath "MACHINE/WEBROOT/APPHOST" `
            -Filter "webFarms/webFarm[@name='$farmName']/server[@address='$($srv.address)']" `
            -Name "applicationRequestRouting.httpPort" `
            -Value $srv.port

        Write-OK "Added $($srv.address):$($srv.port) to farm"
    }
}


# ============================================================
# STEP 6 - Set Round Robin load balancing
# ============================================================
Write-Step "Setting Round Robin load balancing"

Set-WebConfigurationProperty `
    -PSPath "MACHINE/WEBROOT/APPHOST" `
    -Filter "webFarms/webFarm[@name='$farmName']/applicationRequestRouting/protocol" `
    -Name "loadBalancing.algorithm" `
    -Value "RoundRobin"

Write-OK "Round Robin set on farm $farmName"


# ============================================================
# STEP 7 - Create IIS Site on port 8080
# ============================================================
Write-Step "Creating IIS Site on port 8080"

$siteName = "admin-be-proxy"
$sitePath = "C:\inetpub\admin-be-proxy"

# Create physical directory for the site
if (-not (Test-Path $sitePath)) {
    New-Item -ItemType Directory -Path $sitePath -Force | Out-Null
    Write-OK "Created site directory: $sitePath"
} else {
    Write-Skip "Site directory already exists"
}

$existingSite = Get-Website -Name $siteName -ErrorAction SilentlyContinue
if ($existingSite) {
    Write-Skip "IIS site $siteName already exists"
} else {
    New-Website `
        -Name $siteName `
        -PhysicalPath $sitePath `
        -Port 8080 `
        -Force | Out-Null
    Write-OK "IIS site $siteName created on port 8080"
}

# Make sure site is started
Start-Website -Name $siteName -ErrorAction SilentlyContinue
Write-OK "Site $siteName started"


# ============================================================
# STEP 8 - Add URL Rewrite rule to forward to farm
# ============================================================
Write-Step "Adding URL Rewrite rule to route traffic to farm"

$ruleName = "ARR-to-admin-be-farm"
$existingRule = Get-WebConfigurationProperty `
    -PSPath "IIS:\Sites\$siteName" `
    -Filter "system.webServer/rewrite/rules/rule[@name='$ruleName']" `
    -Name "name" `
    -ErrorAction SilentlyContinue

if ($existingRule) {
    Write-Skip "Rewrite rule already exists"
} else {
    Add-WebConfigurationProperty `
        -PSPath "IIS:\Sites\$siteName" `
        -Filter "system.webServer/rewrite/rules" `
        -Name "." `
        -Value @{
            name           = $ruleName
            stopProcessing = "True"
        }

    Set-WebConfigurationProperty `
        -PSPath "IIS:\Sites\$siteName" `
        -Filter "system.webServer/rewrite/rules/rule[@name='$ruleName']/match" `
        -Name "url" -Value "(.*)"

    Set-WebConfigurationProperty `
        -PSPath "IIS:\Sites\$siteName" `
        -Filter "system.webServer/rewrite/rules/rule[@name='$ruleName']/action" `
        -Name "type" -Value "Rewrite"

    Set-WebConfigurationProperty `
        -PSPath "IIS:\Sites\$siteName" `
        -Filter "system.webServer/rewrite/rules/rule[@name='$ruleName']/action" `
        -Name "url" -Value "http://$farmName/{R:1}"

    Write-OK "Rewrite rule added"
}


# ============================================================
# STEP 9 - Open Firewall port 8080
# ============================================================
Write-Step "Opening Firewall port 8080"

$fwRule = Get-NetFirewallRule -DisplayName "IIS-ARR-8080" -ErrorAction SilentlyContinue
if ($fwRule) {
    Write-Skip "Firewall rule IIS-ARR-8080 already exists"
} else {
    New-NetFirewallRule `
        -Name "IIS-ARR-8080" `
        -DisplayName "IIS-ARR-8080" `
        -Enabled True `
        -Direction Inbound `
        -Protocol TCP `
        -Action Allow `
        -LocalPort 8080 | Out-Null
    Write-OK "Firewall rule created for port 8080"
}


# ============================================================
# STEP 10 - Restart IIS to apply all changes
# ============================================================
Write-Step "Restarting IIS"

iisreset /restart | Out-Null
Write-OK "IIS restarted"


# ============================================================
# DONE
# ============================================================
Write-Host "`n============================================================" -ForegroundColor Cyan
Write-Host "  IIS + ARR SETUP COMPLETE" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Traffic flow:" -ForegroundColor Yellow
Write-Host "  Incoming :8080 -> IIS -> ARR -> Round Robin"
Write-Host "                                  |- localhost:8000 (admin-be)"
Write-Host "                                  |- localhost:8001 (admin-be)"
Write-Host ""
Write-Host "Next steps:" -ForegroundColor Yellow
Write-Host "  1. Make sure your Go services are running on ports 8000 and 8001"
Write-Host "  2. Update your GitHub Actions workflow to run 2 service instances"
Write-Host "  3. Test: curl http://<your-server-ip>:8080"
Write-Host ""