# Watcher Agent

[![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Watcher is a pull-based deployment agent for Windows machines.

It runs as a single binary (`watcher.exe`) that hosts:
- a background deployment agent
- a REST API
- an embedded Svelte dashboard

The agent polls GitHub releases, deploys new artifacts, manages services, performs health checks, and records state/history in SQLite.

---

## What It Does

- Pull-based deploys from GitHub Releases (no inbound SSH required)
- Multiple watchers (one poll loop per watcher)
- Service types:
  - `nssm` binaries (managed start/stop/restart)
  - `static` apps (IIS app pool recycle support)
- Automatic rollback on failed post-deploy health checks
- Manual rollback with high-watermark pinning (`max_ignored_version`)
- Poll-event history (`new_release`, `no_update`, `skipped`, `error`, etc.)
- Optional GitHub Deployment API status reporting
- Per-watcher GitHub override settings:
  - `deployment_environment` (fallback: global `ENVIRONMENT`)
  - `github_token` (fallback: global `GITHUB_TOKEN`)
- Self-management endpoints (version, update-check, update, uninstall script)
- Embedded Svelte SPA dashboard served by the same Go process

---

## Runtime Architecture

```text
cmd/watcher/main.go
  ├─ load .env config (Viper)
  ├─ open SQLite (GORM + glebarez/sqlite)
  ├─ start Gin API server
  └─ run Agent
       ├─ sync watchers from DB
       ├─ one goroutine per watcher
       ├─ immediate check trigger channel (API -> agent)
       └─ sync trigger channel (API -> agent)
```

Each watcher loop:
1. Fetch release metadata from GitHub
2. Compare remote version vs `current_version`
3. Enforce rollback high-watermark (`max_ignored_version`)
4. Deploy if needed:
   - download artifact (retry with backoff)
   - extract to `releases/<version>`
   - stop services
   - swap `current` junction (`mklink /J`, copy fallback)
   - ensure service registration (NSSM for `nssm` type)
   - start services / recycle IIS app pools
   - health checks
   - rollback on failure
5. Persist state and deploy logs in SQLite

---

## Metadata Sources

`metadata_url` accepts either:
- a release `version.json` URL (classic flow), or
- a repo URL like `https://github.com/<owner>/<repo>` (native repo mode)

### `version.json` shape

```json
{
  "services": {
    "<service_name>": {
      "version": "v1.2.3",
      "artifact": "my-app-v1.2.3.zip",
      "artifact_url": "https://github.com/<owner>/<repo>/releases/download/v1.2.3/my-app-v1.2.3.zip",
      "published_at": "2024-01-15T10:00:00Z"
    }
  }
}
```

### Artifact expectations

- Zip artifacts should contain deploy binaries/content.
- For `nssm` services, `binary_name` must exist after extraction under `current/`.

---

## API Summary

Base path: `/api`

### System
- `GET /status`
- `GET /logs`
- `GET /logs/stream`
- `POST /github/inspect`

### Watchers
- `GET /watchers`
- `POST /watchers`
- `GET /watchers/:id`
- `PUT /watchers/:id`
- `DELETE /watchers/:id`
- `GET /watchers/:id/services`
- `POST /watchers/:id/services`
- `PUT /watchers/:id/services/:sid`
- `DELETE /watchers/:id/services/:sid`
- `GET /watchers/:id/deploys`
- `GET /watchers/:id/deploys/:did`
- `GET /watchers/:id/deploys/:did/stream`
- `GET /watchers/:id/events`
- `GET /watchers/:id/polls`
- `POST /watchers/:id/check`
- `POST /watchers/:id/redeploy`
- `GET /watchers/:id/versions`
- `POST /watchers/:id/rollback`
- `POST /watchers/:id/resume`
- `DELETE /watchers/:id/versions/:version`

### Services (flat)
- `GET /services`
- `GET /services/:id`
- `POST /services/:id/start`
- `POST /services/:id/stop`
- `POST /services/:id/restart`
- `PUT /services/:id/env`
- `GET /services/:id/health`
- `GET /services/:id/health/history`
- `GET /services/:id/logs`
- `GET /services/:id/deploys`

### Self
- `GET /self/version`
- `GET /self/config`
- `PUT /self/config`
- `GET /self/update-check`
- `POST /self/update`
- `POST /self/restart`
- `POST /self/uninstall`

---

## Dashboard Routes

- `/` Dashboard
- `/watchers`
- `/watchers/:id`
- `/services`
- `/services/:id`
- `/polling`
- `/logs`
- `/settings`

---

## Config (`.env`)

Example is in `.env.example`.

```env
ENVIRONMENT=production
GITHUB_TOKEN=
LOG_DIR=D:\apps\watcher\logs
NSSM_PATH=C:\ProgramData\chocolatey\bin\nssm.exe
DB_PATH=D:\apps\watcher\watcher.db
API_PORT=8080
API_BASE_URL=
WATCHER_REPO_URL=https://github.com/fanboykun/watcher
```

Notes:
- `GITHUB_TOKEN` is required for private repos.
- `API_BASE_URL` enables GitHub Deployment API `log_url` linking.
- `WATCHER_REPO_URL` is used by self-update check/update.
- `GITHUB_DEPLOY_ENABLED=true|false` toggles GitHub Deployment API reporting globally.
- Watcher-level config can override deploy environment/token per repo:
  - `deployment_environment` -> used first for GitHub Deployments environment
  - `github_token` -> used first for GitHub metadata/artifact/deployment API calls
  - if empty, watcher falls back to global `.env` values.

---

## GitHub Token Guide

Use a token if:
- repo is private, or
- you use GitHub Deployment API status reporting.

### Option A — Fine-grained PAT (recommended)

1. GitHub → **Settings** → **Developer settings** → **Personal access tokens** → **Fine-grained tokens**
2. Select repository access for the repos watcher needs.
3. Grant minimum permissions:
   - **Contents: Read** (required for release metadata/assets)
   - **Deployments: Read and write** (required only if using GitHub Deployment API status updates)
4. Generate token and store it in:
   - global `.env` as `GITHUB_TOKEN`, or
   - watcher override in UI (Watchers → Add/Edit watcher).

### Option B — Classic PAT

- For private repos, `repo` scope is typically sufficient.
- If deployment status calls are used, ensure deployment-related repo operations are allowed by org/repo policy.

### Common `404` causes during inspect/deploy

- no published release exists (`/releases/latest` needs a published, non-draft release),
- token does not have access to the target private repo,
- wrong owner/repo URL,
- org SSO/policy not approved for the token.

---

## Data Model

Tables auto-migrated on startup:
- `watchers`
- `services`
- `deploy_logs`
- `health_events`
- `poll_events`

Highlights:
- Watcher state fields: `current_version`, `max_ignored_version`, `status`, `last_checked`, `last_deployed`, `last_error`
- Service fields include `service_type`, `binary_name`, `env_file`, `health_check_url`, `iis_app_pool`, `iis_site_name`, `public_url`, `env_content`
- Deploy log includes `github_deployment_id` and raw text `logs`

---

## Build and Dev

### Requirements
- Go `1.25.x` (module currently uses `go 1.25.6`)
- Bun (for web build)

### Common commands

```bash
make help
make dev
make build-web
make build
make test
make test-verbose
make test-github
make run
make package
make clean
```

`make build` builds the web app first, then embeds `web/build` into the Go binary.

---

## CI / Release Files

- `.github/workflows/release.yml`
  - releases the watcher itself
- `workflows/release-go-nssm.yml`
  - template for Go/NSSM app repos watched by watcher
- `workflows/release-bun-iis.yml`
  - template for static/Bun app repos watched by watcher
- `workflows/deploy.yml`
  - additional deployment workflow template

---

## Install

See `INSTALL.md`.

Typical flow on Windows:
1. Extract release zip
2. Run `install.bat`
3. Complete wizard (`shell/install-watcher.ps1`)
4. Open dashboard on `http://localhost:<API_PORT>`

---

## Important Operational Notes

- API has no built-in auth; deploy behind a trusted network.
- `service_name` must match metadata service key for `version.json` flow.
- `binary_name` must match the extracted file for `nssm` services.
- Manual rollback sets `max_ignored_version`; auto-deploy ignores versions `<=` that value until resumed or a newer version appears.
- After repeated failures for the same target version, auto deploy is suspended for that version until manual redeploy.
