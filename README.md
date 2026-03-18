# Watcher Agent

A pull-based deployment agent for Go services on private Windows servers with no inbound network access.

The watcher runs as a Windows service, polls GitHub Releases every N seconds, and deploys new versions automatically — no SSH, no VPN, no push required.

---

## How it works

```
Developer
  └── git tag v1.2.0 && git push --tags
        ↓
GitHub Actions (release.yml)
  └── builds web.exe + worker.exe
  └── zips into admin-be-v1.2.0.zip
  └── creates GitHub Release
  └── uploads admin-be-v1.2.0.zip + version.json
        ↓  (publicly or privately accessible over HTTPS)
version.json
  {
    "services": {
      "admin-be": {
        "version": "v1.2.0",
        "artifact_url": "https://github.com/.../releases/download/v1.2.0/admin-be-v1.2.0.zip"
      }
    }
  }
        ↑  (polls every 60s via HTTPS using GitHub PAT)
Watcher Agent  [Windows Service — private server]
  └── reads version.txt → "v1.1.0"
  └── sees target "v1.2.0" → mismatch
  └── downloads admin-be-v1.2.0.zip
  └── extracts to releases/v1.2.0/
  └── stops NSSM services
  └── swaps current/ junction → releases/v1.2.0/
  └── starts NSSM services
  └── polls /health until 200
  └── on failure: rolls back to releases/v1.1.0/
  └── writes version.txt → "v1.2.0"
  └── updates state.json
```

---

## Project structure

```
watcher/
  cmd/watcher/main.go         entrypoint, signal handling, poll loop
  internal/
    config.go                 config loader + validation
    github.go                 fetch version.json, download artifact (private repo support)
    deploy.go                 extract zip, swap junction, NSSM service management, rollback
    state.go                  atomic read/write of version.txt + state.json
    watcher.go                main orchestration loop
    logger.go                 structured JSON logger
  config.json                 example config (web + worker)
  config.web-only.json        example config (web only — per-server customization)
  install-watcher.ps1         first-time Windows service bootstrap script
  go.mod
```

---

## Building

```bash
# Cross-compile for Windows from Linux/macOS
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o watcher.exe ./cmd/watcher

# Build on Windows
go build -o watcher.exe ./cmd/watcher
```

---

## First-time server setup (via RDP)

1. Build `watcher.exe` (cross-compile from CI or local machine)
2. RDP into the server
3. Copy `watcher.exe` to `C:\apps\watcher\`
4. Copy and fill in `config.json` to `C:\apps\watcher\config.json`
5. Run as Administrator:

```powershell
powershell -ExecutionPolicy Bypass -File C:\apps\watcher\install-watcher.ps1
```

That's it. The watcher will run forever, restart on crash (via NSSM), and deploy new versions automatically.

---

## Configuration

Each server has its own `config.json`. The key per-server field is `services` — different servers can manage different subsets of services.

```json
{
  "service_name": "admin-be",
  "environment": "production",
  "install_dir": "C:\\apps\\admin-be",
  "check_interval_sec": 60,
  "metadata_url": "https://github.com/your-org/your-repo/releases/latest/download/version.json",
  "github_token": "ghp_xxxxxxxxxxxxxxxxxxxx",
  "download_retries": 3,
  "health_check": {
    "enabled": true,
    "retries": 10,
    "interval_sec": 3,
    "timeout_sec": 5
  },
  "services": [
    {
      "windows_service_name": "admin-be-web-1",
      "binary_name": "web.exe",
      "env_file": "C:\\admin-be\\.env.web.1",
      "health_check_url": "http://localhost:8000/health"
    },
    {
      "windows_service_name": "admin-be-worker-1",
      "binary_name": "worker.exe",
      "env_file": "C:\\admin-be\\.env.worker.1"
    }
  ]
}
```

| Field                             | Description                                                   |
| --------------------------------- | ------------------------------------------------------------- |
| `service_name`                    | Must match the key in `version.json`                          |
| `environment`                     | Human-readable label for this server                          |
| `install_dir`                     | Base directory — releases, state, version.txt live here       |
| `check_interval_sec`              | Poll frequency (default: 60)                                  |
| `metadata_url`                    | Use `releases/latest/download/version.json` for always-latest |
| `github_token`                    | GitHub PAT with `repo` scope (private repo)                   |
| `download_retries`                | Retry attempts with exponential backoff (default: 3)          |
| `health_check.enabled`            | Set `false` to skip post-deploy validation                    |
| `services[].windows_service_name` | Windows service managed by NSSM                               |
| `services[].binary_name`          | Filename inside the release zip                               |
| `services[].env_file`             | Path to the .env file for this service instance               |
| `services[].health_check_url`     | Overrides top-level health check URL for this service         |

---

## Disk layout after first deploy

```
C:\apps\admin-be\
  current\              <- junction pointing to active release
    web.exe
    worker.exe
  releases\
    v1.1.0\             <- previous (kept for rollback)
      web.exe
      worker.exe
    v1.2.0\             <- current
      web.exe
      worker.exe
  version.txt           <- "v1.2.0"
  state.json            <- status, timestamps, last error
  logs\
    watcher.out.log
    watcher.err.log
```

---

## state.json

Written after every check and deploy:

```json
{
  "current_version": "v1.2.0",
  "status": "healthy",
  "last_checked": "2024-01-15T10:01:00Z",
  "last_deployed": "2024-01-15T10:00:15Z",
  "last_error": ""
}
```

Status values: `unknown` | `deploying` | `healthy` | `failed` | `rollback`

---

## Rollback

Rollback is automatic when a health check fails after deploy.
For manual rollback, edit `version.txt` and restart the watcher:

```powershell
nssm stop app-watcher
Set-Content C:\apps\admin-be\version.txt "v1.1.0"
nssm start app-watcher
```

The watcher will detect the mismatch and re-deploy `v1.1.0`.

---

## GitHub token (security note)

The PAT is stored in `config.json`. Protect the file with restricted Windows ACLs:

```powershell
icacls C:\apps\watcher\config.json /inheritance:r
icacls C:\apps\watcher\config.json /grant "SYSTEM:(F)"
icacls C:\apps\watcher\config.json /grant "BUILTIN\Administrators:(F)"
```

The token needs `repo` scope (or `contents:read` for fine-grained PATs) to download private release assets.
