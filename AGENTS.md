# Watcher — Agent Guidelines

## Project Identity

**Name**: Watcher  
**Module**: `github.com/fanboykun/watcher`  
**Language**: Go + SvelteKit frontend  
**Go module target**: `go 1.25.6`  
**Target OS**: Windows 10/11 and Windows Server 2022 (cross-built from Linux/WSL)  
**Purpose**: Pull-based deployment agent with dashboard, API, rollback, and self-management.

---

## Current Stack

### Backend
- Gin (`github.com/gin-gonic/gin`) for API server
- GORM (`gorm.io/gorm`) for persistence
- SQLite via pure-Go driver (`github.com/glebarez/sqlite`)
- Viper (`github.com/spf13/viper`) for `.env` + env var config loading

### Frontend
- SvelteKit (SPA/static build, SSR off)
- Tailwind CSS + shadcn-svelte components
- Embedded into Go binary via `go:embed`

---

## Runtime Architecture

```text
main.go
  ├─ Load config from .env
  ├─ Open SQLite + auto-migrate
  ├─ Start Gin API server
  └─ Start Agent loop
       ├─ syncWatchers() from DB
       ├─ one goroutine per watcher
       ├─ ticker-based poll cycle per watcher
       ├─ API-triggered immediate checks (checkTrigger chan uint)
       └─ API-triggered watcher syncs (syncTrigger chan struct{})
```

### Agent behavior
- Watchers are not static; they are reloaded from DB when CRUD changes occur.
- A watcher goroutine is restarted when its `updated_at` changes.
- Each watcher run records poll events and updates deploy state in DB.
- Consecutive failures are capped per target version (`maxDeployRetries = 3`) to prevent infinite loops.
- GitHub settings precedence is watcher-first:
  - deployment environment: `watcher.deployment_environment` -> global `ENVIRONMENT`
  - token: `watcher.github_token` -> global `GITHUB_TOKEN`

---

## Core Packages

### `cmd/watcher/main.go`
- Entry point and lifecycle wiring.
- Starts API server and agent concurrently.
- Handles shutdown via `signal.NotifyContext`.

### `internal/agent/watcher.go`
- `Agent`, `RepoWatcher`, and deploy orchestration.
- `WatcherConfigFromDB` maps DB models into runtime config.
- Handles version comparisons, rollback high-watermark checks, and deployment start.
- Optional GitHub Deployment API status reporting.

### `internal/agent/deploy.go`
- Deploy pipeline:
  - extract artifact
  - stop services
  - replace release dir
  - swap `current` junction (`mklink /J` fallback to copy)
  - ensure/start services
  - health checks
  - rollback on failure
- Supports `service_type`:
  - `nssm`: install/update/start via NSSM
  - `static`: no NSSM registration; optional IIS app pool recycle
- Provides version retention utilities:
  - `ListAvailableVersions`
  - `CleanOldReleases`
  - `DeleteVersion`

### `internal/agent/github.go`
- GitHub metadata and artifact download logic.
- Supports:
  - `version.json` release-asset mode
  - native repo mode (`https://github.com/<owner>/<repo>`)
- Private repo support uses GitHub API and token auth.
- Includes GitHub Deployment API calls (`CreateDeployment`, `UpdateDeploymentStatus`).

### `internal/agent/self_update.go`
- Self update check against `WATCHER_REPO_URL`.
- Downloads release zip, extracts new executable, swaps binary.
- On Windows, attempts service restart via NSSM.
- Generates uninstall PowerShell script.

### `internal/agent/state.go`
- DB-backed state manager for watcher deploy lifecycle.
- Records watcher status, deploy logs, poll events, and failure status.
- Trims poll history to last 50 entries per watcher.

### `internal/api/*`
- API routing and handlers.
- SSE streams for deploy logs and agent logs.
- Watcher/service CRUD and operational actions.
- Self-management endpoints exposed under `/api/self/*`.

### `internal/config/config.go`
- Config loading with Viper defaults + `.env` + env override.
- Handles Windows path unescaping edge cases.

### `internal/database/*`
- SQLite connection setup and automigration.
- Data model definitions.

---

## Database Models (Authoritative)

### `Watcher`
- identity/config: `name`, `service_name`, `metadata_url`, `install_dir`
- GitHub overrides: `deployment_environment`, `github_token` (stored; API returns masked status fields)
- polling/deploy knobs: `check_interval_sec`, `download_retries`, `max_kept_versions`, `paused`
- health defaults: `hc_enabled`, `hc_url`, `hc_retries`, `hc_interval_sec`, `hc_timeout_sec`
- runtime state: `current_version`, `max_ignored_version`, `status`, `last_checked`, `last_deployed`, `last_error`
- relations: `services`, `deploy_logs`, `poll_events`

### `Service`
- ownership: `watcher_id`
- mode: `service_type` (`nssm` or `static`)
- NSSM fields: `windows_service_name`, `binary_name`, `env_file`
- health: `health_check_url`
- static/IIS fields: `iis_app_pool`, `iis_site_name`
- extras: `public_url`, `env_content`

### `DeployLog`
- `watcher_id`, `version`, `from_version`, `status`, `error`
- `duration_ms`, `logs`, `github_deployment_id`
- `started_at`, `completed_at`

### `HealthEvent`
- `service_id`, `status`, `http_status`, `error`, `checked_at`

### `PollEvent`
- `watcher_id`, `checked_at`, `status`, `remote_version`, `error`

---

## API Surface (Current)

### System
- `GET /api/status`
- `GET /api/logs`
- `GET /api/logs/stream`
- `POST /api/github/inspect`

### Services (flat)
- `GET /api/services`
- `GET /api/services/:id`
- `POST /api/services/:id/start`
- `POST /api/services/:id/stop`
- `POST /api/services/:id/restart`
- `PUT /api/services/:id/env`
- `GET /api/services/:id/health`
- `GET /api/services/:id/health/history`
- `GET /api/services/:id/logs`
- `GET /api/services/:id/deploys`

### Watchers
- `GET /api/watchers`
- `POST /api/watchers`
- `GET /api/watchers/:id`
- `PUT /api/watchers/:id`
- `DELETE /api/watchers/:id`
- `GET /api/watchers/:id/services`
- `POST /api/watchers/:id/services`
- `PUT /api/watchers/:id/services/:sid`
- `DELETE /api/watchers/:id/services/:sid`
- `GET /api/watchers/:id/deploys`
- `GET /api/watchers/:id/deploys/:did`
- `GET /api/watchers/:id/deploy/stream`
- `GET /api/watchers/:id/polls`
- `POST /api/watchers/:id/check`
- `POST /api/watchers/:id/redeploy`
- `GET /api/watchers/:id/versions`
- `POST /api/watchers/:id/rollback`
- `POST /api/watchers/:id/resume`
- `DELETE /api/watchers/:id/versions/:version`

### Self-management
- `GET /api/self/version`
- `GET /api/self/update-check`
- `POST /api/self/update`
- `POST /api/self/uninstall`

---

## Frontend Information

### Routes
- `/` dashboard summary
- `/watchers` list/create
- `/watchers/[id]` watcher detail: tabs for overview/services/deploys/polls/versions
- `/services` flat service list
- `/services/[id]` service detail: health/log/env/deploy tabs
- `/polling` watcher polling operations
- `/logs` agent log tail
- `/settings` self version/update/uninstall actions

### API client
- `web/src/lib/api.ts` contains typed API calls used by all pages.

---

## Configuration

Loaded from `.env` (plus environment overrides).

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
- `API_BASE_URL` is required for GitHub Deployment API log URLs.
- Empty `GITHUB_TOKEN` is allowed for public repositories.

---

## Directory Map

```text
cmd/watcher/main.go
internal/
  agent/
    watcher.go
    deploy.go
    github.go
    self_update.go
    state.go
    logger.go
    ticker.go
    *_test.go
  api/
    router.go
    handlers.go
    service_handlers.go
    system_handlers.go
    dto.go
  config/config.go
  database/
    database.go
    models.go
web/
  embed.go
  src/
    lib/api.ts
    routes/**
shell/install-watcher.ps1
install.bat
.github/workflows/release.yml
workflows/release-go-nssm.yml
workflows/release-bun-iis.yml
workflows/deploy.yml
```

---

## Build, Test, Release

### Local commands
- `make dev`
- `make build-web`
- `make build`
- `make test`
- `make test-verbose`
- `make test-github`
- `make run`
- `make package`

### CI
- Watcher release pipeline: `.github/workflows/release.yml`
- App repo templates: `workflows/release-go-nssm.yml`, `workflows/release-bun-iis.yml`

---

## Critical Contracts and Gotchas

1. `service_name` must match metadata key (for `version.json` flow).
2. For `nssm` services, `binary_name` must match extracted filename exactly.
3. `max_ignored_version` blocks redeploy of versions `<=` pin after rollback.
4. Watcher auto-sync depends on DB row `updated_at`; handlers already bump it on service changes.
5. `mklink /J` is preferred for `current`; copy fallback is slower.
6. Health check URL precedence: service-level URL overrides watcher-level URL.
7. API has no authentication layer; treat deployment as internal/trusted only.
8. `web/build/` must exist before Go build so `go:embed all:build` succeeds.

---

## Testing Coverage Notes

Current tests are concentrated in `internal/agent`:
- GitHub client behaviors (`github_test.go`)
- deploy retention/version listing (`deploy_test.go`)
- self-update helpers/version compare (`self_update_test.go`)

There is no broad integration test suite for API handlers or end-to-end deploy orchestration yet.
