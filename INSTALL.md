# Watcher — Installation Guide

This guide covers everything needed to get the watcher agent running on a Windows machine.

The release zip contains:

```
watcher-vX.Y.Z.zip
  watcher.exe               the watcher agent binary (API + dashboard embedded)
  installer.exe             experimental launcher (optional)
  install.bat               recommended launcher
  install-debug.bat         debug launcher for visible PowerShell output
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
- GitHub token is optional for public repos, required for private repos

---

## Run the Installer (Interactive CLI)

The recommended installer is an elevated interactive PowerShell flow launched by `install.bat`.

1. Extract the release zip to your preferred directory (e.g., `C:\apps\watcher\`)
2. Double-click **`install.bat`**
3. Choose a preset and confirm the generated install summary

Compatibility options:

- `install-debug.bat` launches the installer with `-NoExit` so the PowerShell window stays open after errors
- `installer.exe` is optional/experimental and not the primary path for now

> **Note:** If you prefer running it from an existing Administrator PowerShell prompt, you can use:
> `Set-ExecutionPolicy Bypass -Scope Process -Force; .\shell\install-watcher.ps1`

Quick presets:

1. **Binary Services**: Chocolatey + NSSM for app binaries managed as Windows services
2. **IIS Static**: IIS hosting + management tools + URL Rewrite
3. **Hybrid**: Binary services + IIS hosting
4. **Full Stack**: Binary services + IIS hosting + ARR reverse proxy

Depending on the selected capabilities, the installer will:

1. Install **Chocolatey** and **NSSM** when binary service support is selected
2. Enable required **IIS** Windows features and install **URL Rewrite** / **ARR** when selected
3. Create the `logs\` directory
4. Generate a default **`.env`** config file (with restricted permissions)
5. Verify outbound HTTPS to github.com
6. Register the watcher agent as a Windows service (`app-watcher`)
7. Start the service and verify the API is responding

After installation, the completion menu lets you open the dashboard, open the debug log, or finish/close the installer.

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
GITHUB_DEPLOY_ENABLED=true
LOG_DIR=D:\apps\watcher\logs
LOG_LEVEL=info
LOG_MAX_SIZE_MB=100
LOG_MAX_BACKUPS=10
LOG_MAX_AGE_DAYS=30
LOG_COMPRESS=true
NSSM_PATH=C:\ProgramData\chocolatey\bin\nssm.exe
DB_PATH=D:\apps\watcher\watcher.db
API_PORT=8080
API_BASE_URL=http://localhost:8080
```

| Variable       | Description                                                  |
| -------------- | ------------------------------------------------------------ |
| `ENVIRONMENT`  | Deployment environment label (used as fallback for GitHub Deployments) |
| `GITHUB_TOKEN` | Global GitHub token fallback. Leave empty for public repos   |
| `GITHUB_DEPLOY_ENABLED` | Enable/disable GitHub Deployment API reporting globally |
| `LOG_DIR`      | Where agent writes its logs                                  |
| `LOG_LEVEL`    | Minimum structured log level: `debug`, `info`, `warn`, or `error` |
| `LOG_MAX_SIZE_MB` | Active `watcher.log` size before rotation (default: `100`) |
| `LOG_MAX_BACKUPS` | Number of rotated logs to retain (default: `10`) |
| `LOG_MAX_AGE_DAYS` | Days to retain rotated logs; `0` disables age cleanup |
| `LOG_COMPRESS` | Compress rotated logs with gzip (default: `true`) |
| `NSSM_PATH`    | Full path to nssm.exe                                        |
| `DB_PATH`      | SQLite database file path                                    |
| `API_PORT`     | Port for the API server and dashboard (default: `8080`)      |
| `API_BASE_URL` | Public URL used for GitHub Deployment status `log_url` links |

### GitHub token setup (recommended)

Use **Fine-grained PAT** when possible:

1. GitHub → **Settings** → **Developer settings** → **Personal access tokens** → **Fine-grained tokens**
2. Select repository access for target repo(s)
3. Grant permissions:
   - `Contents: Read` (required for release inspect/download)
   - `Deployments: Read and write` (required only if GitHub Deployment API reporting is enabled)
4. Save token in either:
   - global `.env` (`GITHUB_TOKEN`), or
   - watcher-level override in dashboard (Watchers wizard / watcher edit)

Classic PAT fallback:
- `repo` scope generally works for private release access.

Troubleshooting `releases API returned HTTP 404`:
- repo has no published release yet,
- token cannot access target private repo,
- repo URL owner/name is wrong,
- org SSO policy has not approved the token.

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
     - or native repo URL: `https://github.com/your-org/your-repo`
   - **Custom GitHub Token (optional in Step 1)**:
     - enable when global token cannot access this repo
     - token is saved as watcher override if provided
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
- **Token precedence**: watcher token override is used first, then global `GITHUB_TOKEN`.

---

## Watching an internal release feed (no GitHub access)

Hosts that cannot reach `github.com` can poll a release feed on company storage instead. Pick the source in **Add Watcher → Release source**, or set `source_type` through the API:

| Source | `source_type` | `metadata_url` |
| --- | --- | --- |
| GitHub (default) | `github` | Repository URL or `version.json` release asset URL |
| Internal release feed over HTTP(S) | `http` | Feed root, e.g. `https://releases.internal/apps/my-project` |
| File share feed | `fs` | Feed directory, e.g. `\\fileserver\releases\my-project` or `D:\releases\my-project` |

The feed layout and publishing order are described in `docs/version-json.md` and `docs/adr/0002-internal-release-feed.md`. A feed watcher follows `latest` or a pinned tag (`releases/<tag>/version.json`), and its **Feed releases** tab lists the feed's `releases.json` for downloading and deploying a specific version. Feed watchers never receive the GitHub token and skip GitHub Deployment reporting; set `GITHUB_DEPLOY_ENABLED=false` on hosts with no GitHub access at all.

```powershell
Invoke-RestMethod -Method POST -Uri "http://localhost:8080/api/watchers" -ContentType "application/json" -Body '{
  "name": "my-project",
  "service_name": "my-project",
  "source_type": "fs",
  "metadata_url": "\\\\fileserver\\releases\\my-project",
  "install_dir": "C:\\apps\\my-project"
}'
```

Polling an unchanged feed is cheap. Watcher revalidates `version.json` with `ETag`/`Last-Modified` (nginx, IIS, MinIO/S3 and Nexus send these by default), and on a file share it compares the file's modification time and size. Such polls show as **Up to date (feed unchanged)**. A manual **Check now** always rereads the manifest.

### Authenticated HTTP(S) feeds

When the feed sits behind Nexus/Artifactory, an authenticating proxy or an object-store gateway, set **Feed credentials** on the watcher (`feed_auth_type` in the API):

| `feed_auth_type` | Sends | Fields |
| --- | --- | --- |
| `bearer` | `Authorization: Bearer <secret>` | `feed_auth_secret` |
| `basic` | `Authorization: Basic <username:secret>` | `feed_auth_username`, `feed_auth_secret` |
| `header` | `<header>: <secret>` (e.g. `X-JFrog-Art-Api`) | `feed_auth_header`, `feed_auth_secret` |

The secret is write-only: the API returns only `has_feed_auth_secret` and a masked value, and a blank `feed_auth_secret` on update keeps the saved one. The credential is sent only to the feed root's scheme, host and port. An `artifact_url` on another host, or a redirect to one, is fetched without it, and an `https` feed's credential is never sent over plain `http`. A `401`/`403` shows as `feed authentication failed` on the poll or deploy.

### File share permissions

Watcher runs under NSSM as **LocalSystem** by default. LocalSystem reaches network shares as the **computer account** (`DOMAIN\HOST$`), not as a user, so:

- Grant the computer account (or the domain group containing your Watcher hosts) **read** access on both the share permissions and the NTFS permissions of the feed directory. If Watcher runs under a dedicated service account instead, grant that account.
- Give **write** access only to the publisher (for example the self-hosted runner's account). Watcher hosts never need to write to the feed.
- Workgroup (non-domain) hosts have no computer account on the file server; run Watcher under a local account that exists with the same password on the file server, or serve the feed over HTTP(S) instead.

When access is missing, the watcher's poll shows `access denied reading \\fileserver\releases\my-project\version.json; grant read access to the account Watcher runs as`. A missing file shows `... not found; check the feed path and that a release has been published`.

---

## Updating the watcher

### Option 1: Web UI (Recommended)
Go to **Settings** in the dashboard and click **Check for Updates**. If a new version is available, click **Update Agent**. The watcher will download the new binary, swap itself, and restart the service automatically.

### Updating from an internal release feed

Hosts without GitHub access can update Watcher from the same kind of feed watchers use. Point `WATCHER_REPO_URL` at the feed root, an `https://` URL or a `\\fileserver\share\path`, and publish Watcher's release zip in that feed with a `watcher` service entry:

```json
{
  "services": {
    "watcher": {
      "version": "v1.9.0",
      "artifact": "watcher-windows-amd64.zip",
      "artifact_url": "releases/v1.9.0/watcher-windows-amd64.zip",
      "sha256": "…"
    }
  }
}
```

The zip is the unchanged GitHub release asset of `fanboykun/watcher`; mirror it into the feed manually or from a self-hosted runner job. The `sha256` is checked before the binary is replaced. An authenticated HTTP(S) feed takes `WATCHER_REPO_AUTH_TYPE` (`bearer`, `basic` or `header`), `WATCHER_REPO_AUTH_USERNAME`, `WATCHER_REPO_AUTH_HEADER` and `WATCHER_REPO_AUTH_SECRET` in `.env`; saving settings from the dashboard keeps them.

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

# Manual rollback is supported via Dashboard (Watcher Detail -> Versions -> Rollback)
# Avoid editing on-disk state files manually; deploy state is DB-backed.

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
