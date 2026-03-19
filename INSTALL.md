# Watcher — Installation Guide

This guide covers everything needed to get the watcher agent running on a fresh Windows Server 2022.

The release zip contains:

```
watcher-vX.Y.Z.zip
  watcher.exe               the watcher agent binary
  shell/
    install-watcher.ps1     bootstrap script (registers watcher as a Windows service)
  config.example.json       example config -- copy to config.json and edit before running
  INSTALL.md                this file
```

---

## Prerequisites

- Windows Server 2022
- RDP access to the server
- A GitHub PAT with `repo` scope (for private repos) or no token needed (public repos)
- Outbound HTTPS access to `github.com` from the server

---

## Part 1 -- Prepare the Windows server

Run all commands in **PowerShell as Administrator**.

### Step 1 -- Install Chocolatey

```powershell
Set-ExecutionPolicy Bypass -Scope Process -Force
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
```

Verify:

```powershell
choco --version
```

---

### Step 2 -- Install NSSM

NSSM manages the watcher and your app services as Windows services:

```powershell
choco install nssm -y
```

Verify:

```powershell
C:\ProgramData\chocolatey\bin\nssm.exe version
```

---

### Step 3 -- Create base directories

```powershell
New-Item -ItemType Directory -Path "D:\apps"         -Force
New-Item -ItemType Directory -Path "D:\apps\watcher" -Force
```

---

### Step 4 -- Verify outbound HTTPS

The watcher polls GitHub over HTTPS. Confirm it can reach GitHub:

```powershell
Invoke-WebRequest -Uri "https://github.com" -UseBasicParsing | Select-Object StatusCode
```

Should return `StatusCode: 200`. If not, check your firewall or proxy settings before continuing.

---

## Part 2 -- Install the watcher

### Step 1 -- Extract the release zip

Extract `watcher-vX.Y.Z.zip` into `D:\apps\watcher\`. You should have:

```
D:\apps\watcher\
  watcher.exe
  shell\
    install-watcher.ps1
  config.example.json
  INSTALL.md
```

---

### Step 2 -- Create `config.json` from the example

```powershell
Copy-Item D:\apps\watcher\config.example.json D:\apps\watcher\config.json
```

Then open and fill in your values:

```json
{
  "environment": "production",
  "github_token": "ghp_your_actual_pat_here",
  "log_dir": "D:\\apps\\watcher\\logs",
  "nssm_path": "C:\\ProgramData\\chocolatey\\bin\\nssm.exe",

  "watchers": [
    {
      "name": "my-service",
      "service_name": "my-service",
      "metadata_url": "https://github.com/your-org/your-repo/releases/latest/download/version.json",
      "check_interval_sec": 60,
      "install_dir": "D:\\apps\\my-service",
      "health_check": {
        "enabled": true,
        "retries": 10,
        "interval_sec": 3,
        "timeout_sec": 5
      },
      "services": [
        {
          "windows_service_name": "my-service-web-1",
          "binary_name": "web.exe",
          "env_file": "D:\\apps\\my-service\\.env.web.1",
          "health_check_url": "http://localhost:8000/health"
        },
        {
          "windows_service_name": "my-service-worker-1",
          "binary_name": "worker.exe",
          "env_file": "D:\\apps\\my-service\\.env.worker.1"
        }
      ]
    }
  ]
}
```

Key fields:

| Field                             | Description                                                                                                                     |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `github_token`                    | PAT with `repo` scope. Leave empty `""` for public repos.                                                                       |
| `metadata_url`                    | URL to `version.json` published by the service repo's release workflow. Always use `.../releases/latest/download/version.json`. |
| `service_name`                    | Must match `APP_NAME` in the service repo's `release.yml`.                                                                      |
| `install_dir`                     | Where the watcher extracts releases for this service.                                                                           |
| `services[].windows_service_name` | The Windows service name. The watcher registers it automatically on first deploy.                                               |
| `services[].binary_name`          | Filename inside the release zip (e.g. `web.exe`).                                                                               |
| `services[].env_file`             | Path to the `.env` file for this service instance. Must exist before first deploy.                                              |

---

### Step 3 -- Create `.env` files for your app services

The watcher deploys binaries but does not manage `.env` files. Create them before the first deploy:

```powershell
New-Item -ItemType Directory -Path "D:\apps\my-service"      -Force
New-Item -ItemType Directory -Path "D:\apps\my-service\logs" -Force

notepad D:\apps\my-service\.env.web.1
notepad D:\apps\my-service\.env.worker.1
```

Example `.env` content:

```env
PORT=8000
DB_HOST=localhost
DB_PORT=5432
DB_NAME=mydb
DB_USER=myuser
DB_PASSWORD=secret
```

> `PORT` must match the `health_check_url` port for that service instance.

---

### Step 4 -- Edit `shell\install-watcher.ps1`

Open `D:\apps\watcher\shell\install-watcher.ps1` and update the `$Config` block at the top to match your installation path:

```powershell
$Config = @{
    ServiceName  = "app-watcher"
    InstallDir   = "D:\apps\watcher"
    WatcherExe   = "D:\apps\watcher\watcher.exe"
    ConfigFile   = "D:\apps\watcher\config.json"
    LogDir       = "D:\apps\watcher\logs"
    NssmPath     = "C:\ProgramData\chocolatey\bin\nssm.exe"
    RestartDelay = 5000
}
```

---

### Step 5 -- Run the install script

```powershell
powershell -ExecutionPolicy Bypass -File D:\apps\watcher\shell\install-watcher.ps1
```

The script will:

1. Install Chocolatey if missing
2. Install NSSM if missing
3. Verify `watcher.exe` and `config.json` exist
4. Verify outbound HTTPS to `github.com`
5. Create log directories
6. Restrict `config.json` permissions (protects your GitHub token)
7. Register `app-watcher` as a Windows service via NSSM
8. Start the service and confirm it is running

---

### Step 6 -- Verify the watcher is running

```powershell
# Check service status
Get-Service app-watcher

# Watch live logs
Get-Content D:\apps\watcher\logs\watcher.out.log -Wait
```

Expected output on first run:

```json
{"time":"...","level":"INFO","component":"agent","msg":"watcher agent starting"}
{"time":"...","level":"INFO","component":"agent","msg":"config loaded","fields":{"watchers":1}}
{"time":"...","level":"INFO","component":"my-service","msg":"check cycle"}
{"time":"...","level":"INFO","component":"my-service","msg":"version mismatch, deploying","fields":{"from":"","to":"v1.0.0"}}
{"time":"...","level":"INFO","component":"my-service","msg":"service not registered, installing via NSSM","fields":{"name":"my-service-web-1"}}
{"time":"...","level":"INFO","component":"my-service","msg":"deploy complete","fields":{"version":"v1.0.0"}}
```

The watcher registers NSSM services automatically on first deploy -- no manual setup required.

---

## Part 3 -- Adding a new service

### Step 1 -- Ensure the service repo has a `release.yml` workflow

The service repo must use the same `release.yml` workflow as this watcher repo. Copy it into `.github/workflows/release.yml` of the target repo and update the config block:

```yaml
env:
  APP_NAME: my-new-service # must match service_name in watcher config.json
  GO_VERSION: "1.21"
  WEB_BINARY: api.exe
  WORKER_BINARY: worker.exe
  WEB_ENTRY: cmd/api/main.go
  WORKER_ENTRY: cmd/worker/main.go
```

Trigger the first release using a recognized commit pattern:

```bash
git commit -m "feat: initial release"
git push origin main
```

The workflow auto-tags and publishes a release. Recognized patterns:

| Pattern                          | Bump                  |
| -------------------------------- | --------------------- |
| `feat: <msg>`                    | minor                 |
| `fix: / perf: / refactor: <msg>` | patch                 |
| `feat!:` or `BREAKING CHANGE`    | major                 |
| `minor: / patch: / major: <msg>` | respective            |
| `bump: minor / patch / major`    | respective            |
| `[release]` anywhere in message  | patch                 |
| `chore: / docs: / ci:`           | **skip** (no release) |

If you need to force a bump regardless of commit messages, go to **Actions → Release → Run workflow** and set `force_bump`.

---

### Step 2 -- Create `.env` files on the server

```powershell
New-Item -ItemType Directory -Path "D:\apps\my-new-service"      -Force
New-Item -ItemType Directory -Path "D:\apps\my-new-service\logs" -Force

notepad D:\apps\my-new-service\.env.1
```

---

### Step 3 -- Add a new entry to `config.json`

Open `D:\apps\watcher\config.json` and append to the `watchers` array:

```json
{
  "name": "my-new-service",
  "service_name": "my-new-service",
  "metadata_url": "https://github.com/your-org/my-new-service/releases/latest/download/version.json",
  "check_interval_sec": 60,
  "install_dir": "D:\\apps\\my-new-service",
  "health_check": {
    "enabled": true,
    "retries": 10,
    "interval_sec": 3,
    "timeout_sec": 5
  },
  "services": [
    {
      "windows_service_name": "my-new-service-api-1",
      "binary_name": "api.exe",
      "env_file": "D:\\apps\\my-new-service\\.env.1",
      "health_check_url": "http://localhost:9000/health"
    }
  ]
}
```

---

### Step 4 -- Restart the watcher

```powershell
nssm restart app-watcher
```

The watcher will immediately start polling the new service repo.

---

## Useful commands

```powershell
# Watcher service management
Get-Service app-watcher
nssm start   app-watcher
nssm stop    app-watcher
nssm restart app-watcher
nssm remove  app-watcher confirm

# Live logs
Get-Content D:\apps\watcher\logs\watcher.out.log -Wait

# Force manual rollback of a service
nssm stop app-watcher
Set-Content D:\apps\my-service\version.txt "v1.0.0"
nssm start app-watcher
```
