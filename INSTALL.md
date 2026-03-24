# Watcher — Installation Guide

This guide covers everything needed to get the watcher agent running on a Windows machine.

The release zip contains:

```
watcher-vX.Y.Z.zip
  watcher.exe               the watcher agent binary (API + dashboard embedded)
  install.bat               wrapper script to launch GUI wizard
  shell/
    install-watcher.ps1     bootstrap script (installs dependencies, registers service)
  .env.example              example config — copy to .env and edit
  INSTALL.md                this file
```

---

## Prerequisites

- Windows 10/11 or Windows Server 2022
- Administrator access
- Outbound HTTPS access to `github.com`
- A GitHub PAT with `repo` scope (for private repos) or no token needed (public repos)

---

## Run the Installer (GUI Wizard)

The installation is driven by a clean, interactive Windows GUI that automatically requests Administrator privileges and handles execution policies.

1. Extract the release zip to your preferred directory (e.g., `C:\apps\watcher\`)
2. Double-click **`install.bat`**

> **Note:** If you prefer running it from an existing Administrator PowerShell prompt, you can use:
> `Set-ExecutionPolicy Bypass -Scope Process -Force; .\shell\install-watcher.ps1`

The GUI wizard will ask you to select an **Installation Profile**:

1. **Binary services only**: Installs Chocolatey + NSSM
2. **Static sites only**: Enables IIS + URL Rewrite
3. **Both**: Installs NSSM + IIS + URL Rewrite
4. **Full stack**: Installs everything above plus Application Request Routing (ARR) for reverse proxying

Depending on your profile selection, the wizard will:

1. Install **Chocolatey** and **NSSM**
2. Enable **IIS** Windows features, download and install **URL Rewrite** / **ARR**
3. Create the `logs\` directory
4. Generate a default **`.env`** config file (with restricted permissions)
5. Verify outbound HTTPS to github.com
6. Register the watcher agent as a Windows service (`app-watcher`)
7. Start the service and verify the API is responding

After installation, the dashboard will open automatically in your default browser at **http://localhost:8080** (or your chosen port).

---

## Manual install (step by step)

### Step 1 — Install Chocolatey & NSSM (for binary services)

If you plan to manage background binaries:

```powershell
Set-ExecutionPolicy Bypass -Scope Process -Force
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))

choco install nssm -y
```

### Step 2 — Enable IIS (for static sites)

If you plan to deploy frontend static sites, enable IIS (from an Admin PowerShell):

**On Windows Desktop (10/11):**
```powershell
Enable-WindowsOptionalFeature -Online -FeatureName IIS-WebServerRole, IIS-WebServer, IIS-CommonHttpFeatures, IIS-StaticContent -All
```

**On Windows Server:**
```powershell
Install-WindowsFeature -Name Web-Server -IncludeManagementTools
```

### Step 3 — Extract release

Extract `watcher-vX.Y.Z.zip` to `D:\apps\watcher\`.

### Step 4 — Create `.env`

```powershell
Copy-Item D:\apps\watcher\.env.example D:\apps\watcher\.env
notepad D:\apps\watcher\.env
```

Fill in your values:

```env
ENVIRONMENT=production
GITHUB_TOKEN=ghp_your_pat_here
LOG_DIR=D:\apps\watcher\logs
NSSM_PATH=C:\ProgramData\chocolatey\bin\nssm.exe
DB_PATH=D:\apps\watcher\watcher.db
API_PORT=8080
```

| Variable       | Description                                                  |
| -------------- | ------------------------------------------------------------ |
| `ENVIRONMENT`  | Label for this environment (informational only)              |
| `GITHUB_TOKEN` | PAT with `repo` scope. Leave empty for public repos          |
| `LOG_DIR`      | Where agent writes its logs                                  |
| `NSSM_PATH`    | Full path to nssm.exe                                        |
| `DB_PATH`      | SQLite database file path                                    |
| `API_PORT`     | Port for the API server and dashboard (default: `8080`)      |

### Step 5 — Secure `.env` permissions

```powershell
icacls D:\apps\watcher\.env /inheritance:r
icacls D:\apps\watcher\.env /grant "SYSTEM:(F)"
icacls D:\apps\watcher\.env /grant "BUILTIN\Administrators:(F)"
```

### Step 6 — Register as Windows service

```powershell
$nssm = "C:\ProgramData\chocolatey\bin\nssm.exe"

& $nssm install app-watcher "D:\apps\watcher\watcher.exe"
& $nssm set app-watcher AppParameters "-config `"D:\apps\watcher\.env`""
& $nssm set app-watcher AppDirectory "D:\apps\watcher"
& $nssm set app-watcher Start SERVICE_AUTO_START
& $nssm set app-watcher AppStdout "D:\apps\watcher\logs\watcher.out.log"
& $nssm set app-watcher AppStderr "D:\apps\watcher\logs\watcher.err.log"
& $nssm set app-watcher AppRotateFiles 1
& $nssm set app-watcher AppRotateOnline 1
& $nssm set app-watcher AppRestartDelay 5000
```

### Step 7 — Start the service

```powershell
nssm start app-watcher
```

### Step 8 — Verify

```powershell
Get-Service app-watcher
Invoke-WebRequest -Uri "http://localhost:8080/api/status" -UseBasicParsing
```

Open **http://localhost:8080** in a browser.

---

## Adding a watched repo

After installation, use the **dashboard** at `http://localhost:8080`:

1. Go to **Watchers** → click **Add Watcher**
2. Fill in:
   - **Name**: display name (e.g. `my-project`)
   - **Service Name**: must match `APP_NAME` in the repo's `release.yml`
   - **Metadata URL**: `https://github.com/your-org/your-repo/releases/latest/download/version.json`
   - **Install Dir**: e.g. `C:\apps\my-project`
   - **Check Interval**: poll frequency in seconds (default: 60)
3. After creating the watcher, click into it and **Add Service**:
   - **Service Type**: Choose either **Binary (NSSM)** or **Static Site (IIS)**
   - **Service Identifier**: The name used for the Windows Service or IIS site.
   - For **Binary**: supply the **Binary Name** (e.g., `web.exe`) and optional **Env File**.
   - For **Static Site**: supply the **IIS App Pool Name** and **IIS Site Name**.
   - **Health Check URL**: Optional ping after deployment (e.g. `http://localhost:3000/health`)

Alternatively, use the REST API:

```powershell
# Create a watcher
Invoke-RestMethod -Method POST -Uri "http://localhost:8080/api/watchers" -ContentType "application/json" -Body '{
  "name": "my-project",
  "service_name": "my-project",
  "metadata_url": "https://github.com/your-org/your-repo/releases/latest/download/version.json",
  "install_dir": "C:\\apps\\my-project",
  "check_interval_sec": 60,
  "hc_enabled": true
}'

# Add a binary (NSSM) service to the watcher
Invoke-RestMethod -Method POST -Uri "http://localhost:8080/api/watchers/1/services" -ContentType "application/json" -Body '{
  "service_type": "nssm",
  "windows_service_name": "my-api",
  "binary_name": "api.exe",
  "env_file": "C:\\apps\\my-project\\.env"
}'

# Add a static (IIS) service to the watcher
Invoke-RestMethod -Method POST -Uri "http://localhost:8080/api/watchers/1/services" -ContentType "application/json" -Body '{
  "service_type": "static",
  "windows_service_name": "my-frontend-service",
  "iis_app_pool": "my-frontend-pool",
  "iis_site_name": "my-frontend"
}'
```

> **Important**: 
> - **For NSSM Services**: Create the `.env` files for your app services before the first deploy. The watcher deploys binaries but does NOT manage `.env` files.
> - **For IIS Services**: Ensure the IIS Site and App Pool are created in IIS Manager and pointed to the `C:\apps\my-project\current` junction folder before the first payload deploys.

---

## Updating the watcher

### Option 1: Web UI (Recommended)
Go to **Settings** in the dashboard and click **Check for Updates**. If a new version is available, click **Update Agent**. The watcher will download the new binary, swap itself, and restart the service automatically.

### Option 2: Manual
1. Download the new release zip
2. Stop the service: `nssm stop app-watcher`
3. Replace `watcher.exe` in `D:\apps\watcher\`
4. Start the service: `nssm start app-watcher`

The SQLite database and `.env` are preserved across updates.

---

## Useful commands

```powershell
# Service management
Get-Service app-watcher
nssm start   app-watcher
nssm stop    app-watcher
nssm restart app-watcher
nssm remove  app-watcher confirm    # uninstall

# Live logs
Get-Content D:\apps\watcher\logs\watcher.out.log -Wait

# Force manual rollback of a watched service
# Note: This manually changes version.txt, but doesn't set the High-Watermark Pin in the DB.
# It is recommended to use the Dashboard for rollbacks.
nssm stop app-watcher
Set-Content D:\apps\my-service\version.txt "v1.0.0"
nssm start app-watcher

# Test without NSSM (run directly)
.\watcher.exe -config .env
```

---

## Troubleshooting

| Problem | Check |
|---------|-------|
| Service won't start | `Get-Content D:\apps\watcher\logs\watcher.err.log` |
| Can't reach dashboard | Verify `API_PORT` in `.env`, check firewall |
| Deploy fails | Check dashboard → Watcher Detail → Deploy History |
| Health check fails | Verify `health_check_url` is correct, service is binding to the right port |
| NSSM not found | Verify `NSSM_PATH` in `.env` matches actual install location |
