# Watcher Agent

<p align="center">
  <img src="web/static/watcher.png" alt="Watcher" width="120" height="120">
</p>

[![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Total downloads](https://img.shields.io/github/downloads/fanboykun/watcher/total?label=downloads)](https://github.com/fanboykun/watcher/releases)

Watcher is a pull-based deployment agent for Windows machines.

It runs as a single binary (`watcher.exe`) that hosts:
- a background deployment agent
- a REST API
- an embedded Svelte dashboard

The agent polls GitHub releases, deploys new artifacts, manages services, performs health checks, and records state/history in SQLite.

[![Download Watcher](https://img.shields.io/badge/Download-Watcher-2ea44f?style=for-the-badge&logo=github)](https://github.com/fanboykun/watcher/releases/latest/download/watcher-latest.zip)

---

## Install

Download the latest release zip, extract it on the Windows host, then run `install.bat` to start the interactive installer.

Full installation guide: [INSTALL.md](INSTALL.md)

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
- Per-watcher outbound webhooks with durable delivery history
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

Watcher prefers a `version.json` manifest. Repo asset discovery still exists as a legacy/manual fallback, but new org release workflows should publish `version.json` for every Watcher-managed release.

`metadata_url` accepts either:

- a direct release manifest URL such as `https://github.com/<owner>/<repo>/releases/latest/download/version.json`, or
- a repo URL such as `https://github.com/<owner>/<repo>`. During watcher creation, the UI tries to find `version.json` for the selected release ref first.

Detailed manifest reference: [docs/version-json.md](docs/version-json.md).

### Consumer release requirements

Watcher only picks up a consumer repository release when the app repository follows these rules:

- Publish a GitHub Release, not only a tag. Draft releases are ignored by GitHub's latest-release API.
- Upload the deployable zip as a release asset.
- Upload `version.json` as a release asset for every Watcher-managed release. This is the hard org contract.
- Keep `version.json.services["<service_name>"]` exactly equal to the Watcher `service_name`.
- Set `version` to the exact version Watcher should compare. For monorepos, prefer the full service tag such as `services/prs/v1.2.3`.
- Set `artifact` to the exact release asset filename.
- Set `artifact_url` to the exact GitHub release download URL for that asset. Slash tags such as `services/prs/v1.2.3` are supported.
- Set deployment hints (`app_kind`, `binary_name`, IIS fields, health URL) so the create wizard can build the right service config without guessing.
- Package the zip so Watcher can use it immediately after extraction:
  - `nssm`: the configured `binary_name` must exist at the zip root.
  - `static` / `php` / `aspnet_classic`: the site/app files must exist at the zip root.
- For monorepos, publish a manifest index release, usually tag `latest`, that points each service to its service-scoped release asset.
- Do not publish two service releases concurrently from the same repo; serialize release workflows with GitHub Actions `concurrency`.

### `version.json` shape

```json
{
  "services": {
    "my-app": {
      "version": "v1.2.3",
      "artifact": "my-app-v1.2.3.zip",
      "artifact_url": "https://github.com/<owner>/<repo>/releases/download/v1.2.3/my-app-v1.2.3.zip",
      "published_at": "2026-06-01T04:19:05Z",
      "app_kind": "nssm",
      "windows_service_name": "my-app",
      "binary_name": "my-app.exe",
      "start_arguments": "",
      "env_file": ".env",
      "health_check_url": "http://localhost:8080/health"
    }
  }
}
```

### Supported manifest properties

| Property | Required | Applies to | Meaning |
| --- | --- | --- | --- |
| `version` | yes | all | Version stored in Watcher state and compared on each poll. |
| `artifact` | yes | all | Exact release asset filename. |
| `artifact_url` | yes | all | GitHub release download URL for the deployable artifact. |
| `published_at` | recommended | all | UTC timestamp for display and diagnostics. |
| `app_kind` | recommended | all | `nssm`, `static`, `php`, or `aspnet_classic`. |
| `windows_service_name` | recommended | all | NSSM service name or IIS service identifier/default name. |
| `binary_name` | required for `nssm` | `nssm` | Executable filename expected after extraction. |
| `start_arguments` | optional | `nssm` | Arguments passed to the NSSM application. |
| `env_file` | optional | `nssm` | Relative env file path for managed env content. |
| `health_check_url` | optional | all | Service-level health check URL. |
| `iis_app_pool` | recommended | `static`, `php`, `aspnet_classic` | IIS app pool name. |
| `iis_site_name` | recommended | `static`, `php`, `aspnet_classic` | IIS site name. |
| `iis_managed_runtime` | optional | `aspnet_classic` | IIS runtime such as `v4.0`; leave empty for static/PHP. |
| `public_url` | optional | IIS-hosted apps | URL displayed in the UI and useful for health checks. |

### Artifact expectations

- Zip artifacts should contain deploy binaries/content at the root.
- For `nssm` services, `binary_name` must exist after extraction under `current/`.
- For IIS-hosted services, site files should exist directly under `current/`.

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
- `GET /watchers/:id/webhook-events`
- `GET /watchers/:id/webhook-deliveries`
- `GET /watchers/:id/webhook-deliveries/:deliveryId`
- `POST /watchers/:id/webhook/test`
- `POST /watchers/:id/webhook/resume`

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

## Webhooks

Watcher can emit durable outbound webhook events for:

- `watcher.version_found`
- `watcher.deployment_succeeded`
- `watcher.deployment_failed`
- `watcher.rollback_succeeded`
- `watcher.rollback_failed`
- `service.health_changed`
- `watcher.webhook_test`
- `webhook.delivery_exhausted`

Important behavior:

- Delivery is at-least-once. Deduplicate on `event_id`.
- Each HTTP attempt gets its own `delivery_id`.
- Any `2xx` counts as success.
- Network errors, `429`, and `5xx` retry.
- Other `4xx` responses are recorded as final failures.
- Watcher preserves FIFO delivery order per watcher.
- Paused webhook delivery suppresses new events instead of dropping them.

Detailed contract, event payload fields, and trigger rules:

- [docs/webhooks.md](docs/webhooks.md)

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
  - Watcher-ready release template for a single Go repository with one or more Windows/NSSM binaries
  - emits a flat Windows amd64 zip and `version.json`
- `workflows/release-bun-iis.yml`
  - Watcher-ready release template for a SvelteKit static site served as a static/IIS target
  - emits a static build zip and `version.json`
- `workflows/release-go-monorepo.yml`
  - Watcher-ready release template for one Go service inside a mono repo
  - uses service-scoped tags such as `<service>/v1.2.3` and service-only path triggers
- `workflows/deploy.yml`
  - legacy direct deployment workflow template

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
