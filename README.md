# Watcher Agent

A pull-based deployment agent for Go services on private Windows servers with no inbound network access.

The watcher runs as a Windows service, polls GitHub Releases on a configurable interval, and deploys new versions automatically — no SSH, no VPN, no push access required.

---

## Table of contents

- [How it works](#how-it-works)
- [Requirements for watched services](#requirements-for-watched-services)
- [Project structure](#project-structure)
- [Installation](#installation)
- [Triggering a release](#triggering-a-release)
- [Rollback](#rollback)
- [Disk layout](#disk-layout)
- [state.json reference](#statejson-reference)
- [Logs](#logs)
- [config.json reference](#configjson-reference)

---

## Requirements for watched services

> **Any service managed by this watcher must use the same `release.yml` workflow.**

The watcher is tightly coupled to the release format that `release.yml` produces. If a service repo uses a different release workflow or a different artifact structure, the watcher will fail to deploy it.

### What `release.yml` must produce

Every watched service repo must publish a GitHub Release containing exactly these two assets:

**1. `version.json`** — the version metadata file the watcher polls:

```json
{
  "services": {
    "<APP_NAME>": {
      "version": "v1.2.0",
      "artifact": "my-service-v1.2.0.zip",
      "artifact_url": "https://github.com/your-org/my-service/releases/download/v1.2.0/my-service-v1.2.0.zip",
      "published_at": "2024-01-15T10:00:00Z"
    }
  }
}
```

**2. `<APP_NAME>-<version>.zip`** — a flat zip archive containing the binaries:

```
my-service-v1.2.0.zip
  ├── web.exe       ← at root level, no subdirectory
  └── worker.exe    ← at root level, no subdirectory
```

> Binaries **must be at the root** of the zip. The watcher extracts directly into `releases/<version>/` and expects `releases/<version>/web.exe` — not `releases/<version>/dist/web.exe`.

### How to set up `release.yml` in a new service repo

1. Copy `release.yml` from this repo into `.github/workflows/release.yml` of the target service repo
2. Edit only the config block at the top of the file:

```yaml
env:
  APP_NAME:
    my-service # must be unique across all watched services
    # must match "service_name" in watcher's config.json
  GO_VERSION: "1.21"
  GO_PRIVATE: github.com/your-org/*
  WEB_BINARY: my-service.exe # must match "binary_name" in watcher's config.json
  WORKER_BINARY: "" # leave empty if no worker binary
  WEB_ENTRY: cmd/api/main.go
  WORKER_ENTRY: "" # leave empty if no worker
```

3. Add the same secrets to the repo's `production` environment:
   - `GH_ACCESS_TOKEN` — PAT for pulling private Go packages
   - `GITHUB_TOKEN` — automatically available, used for creating releases

### The contract between `release.yml` and `config.json`

These values must stay in sync between the service repo's `release.yml` and the watcher's `config.json`:

| `release.yml`   | `config.json`             | Must match                                             |
| --------------- | ------------------------- | ------------------------------------------------------ |
| `APP_NAME`      | `watchers[].service_name` | Exact string match — this is the key in `version.json` |
| `WEB_BINARY`    | `services[].binary_name`  | Exact filename inside the zip                          |
| `WORKER_BINARY` | `services[].binary_name`  | Exact filename inside the zip                          |

If these are out of sync the watcher will either fail to find the service in `version.json`, or fail to find the binary inside the zip after extraction.

---

## How it works

```
Developer
  └── git commit -m "feat: add new endpoint"
  └── git push origin main
        ↓
GitHub Actions (release.yml in app repo)
  └── detects feat: → minor bump
  └── pushes tag v1.2.0
  └── builds web.exe + worker.exe for Windows
  └── packages into admin-be-v1.2.0.zip
  └── creates GitHub Release v1.2.0
  └── uploads admin-be-v1.2.0.zip + version.json as release assets
        ↓  (accessible over HTTPS using GitHub PAT)
version.json
  {
    "services": {
      "admin-be": {
        "version": "v1.2.0",
        "artifact_url": "https://github.com/.../releases/download/v1.2.0/admin-be-v1.2.0.zip"
      }
    }
  }
        ↑  (polls every 60s via HTTPS)
Watcher Agent  [Windows Service on private server]
  └── reads local version.txt → "v1.1.0"
  └── sees remote target "v1.2.0" → mismatch detected
  └── downloads admin-be-v1.2.0.zip
  └── extracts to D:\apps\admin-be\releases\v1.2.0\
  └── stops NSSM services
  └── swaps D:\apps\admin-be\current\ → releases\v1.2.0\
  └── starts NSSM services
  └── polls /health until HTTP 200
  └── on failure: automatically rolls back to releases\v1.1.0\
  └── writes version.txt → "v1.2.0"
  └── updates state.json → status: healthy
```

---

## Project structure

```
watcher/
  cmd/watcher/main.go         entrypoint — signal handling, starts Agent
  internal/
    config.go                 config loader + validation
    github.go                 fetch version.json, download artifact (private repo)
    deploy.go                 extract zip, swap junction, NSSM management, rollback
    state.go                  atomic read/write of version.txt + state.json
    watcher.go                Agent + RepoWatcher — one goroutine per watched repo
    logger.go                 structured JSON logger with per-component context
    ticker.go                 ticker helper
  config.json                 example config
  install-watcher.ps1         first-time Windows service bootstrap
  go.mod
```

## Installation

For step-by-step instructions on how to set up the server, install the watcher, and add new services, see:

**[INSTALL.md](./INSTALL.md)**

The release zip already includes `INSTALL.md` alongside `watcher.exe` and `shell/install-watcher.ps1` so you have everything in one place after downloading.

---

---

## Triggering a release

The watcher uses **auto-tagging via semantic versioning**. You never create tags manually — just push a commit to `main` with a recognized message pattern and the release workflow handles the rest.

### How it works

When a push lands on `main`, the `release.yml` workflow:

1. Reads all commit messages since the last tag
2. Classifies each commit and determines the highest bump type
3. If any releasable commits exist, bumps the version, pushes a new tag, builds `watcher.exe`, packages the zip, and creates a GitHub Release
4. If no releasable commits exist (e.g. only `chore:` or `docs:`), the workflow exits early with no release

### Commit patterns

**Conventional commits (spec):**

| Pattern                                 | Bump     | Example                                     |
| --------------------------------------- | -------- | ------------------------------------------- |
| `feat: <msg>`                           | minor    | `feat: add health check endpoint`           |
| `fix: <msg>`                            | patch    | `fix: nil pointer on startup`               |
| `perf: <msg>`                           | patch    | `perf: reduce polling overhead`             |
| `refactor: <msg>`                       | patch    | `refactor: split deploy logic`              |
| `feat!: <msg>`                          | major    | `feat!: redesign config format`             |
| `BREAKING CHANGE: <msg>`                | major    | `BREAKING CHANGE: drop config.json support` |
| `chore: / docs: / ci: / style: / test:` | **skip** | no release triggered                        |

**Shortcut patterns (forgiving aliases):**

| Pattern                         | Bump  | Example                               |
| ------------------------------- | ----- | ------------------------------------- |
| `minor: <msg>`                  | minor | `minor: bump go version`              |
| `patch: <msg>`                  | patch | `patch: update deps`                  |
| `major: <msg>`                  | major | `major: new config format`            |
| `bump: minor`                   | minor | `bump: minor`                         |
| `bump: patch`                   | patch | `bump: patch`                         |
| `bump: major`                   | major | `bump: major`                         |
| `release: <msg>`                | patch | `release: deploy hotfix`              |
| `release(minor): <msg>`         | minor | `release(minor): new watcher feature` |
| `release(major): <msg>`         | major | `release(major): breaking change`     |
| `[release]` anywhere in message | patch | `update readme [release]`             |
| `[release:minor]` anywhere      | minor | `update readme [release:minor]`       |
| `[release:major]` anywhere      | major | `update readme [release:major]`       |

**Unrecognized patterns are skipped** — if none of your commits since the last tag match any pattern above, no release is created. This is intentional: it prevents accidental releases from noise commits.

### Version bump priority

When multiple commits exist since the last tag, the highest bump wins:

```
major > minor > patch > skip
```

For example, if you push three commits:

```
chore: update dependencies    -> skip
fix: handle timeout errors    -> patch
feat: add multi-repo support  -> minor
```

The result is a **minor** bump.

### Manual trigger

If you forgot the right pattern or need to force a specific bump, go to:

**Actions → Release Watcher → Run workflow** → set `force_bump` to `patch`, `minor`, or `major`

This bypasses commit analysis entirely and creates a release with the specified bump.

### Skip release entirely

Add `[skip ci]` anywhere in a commit message to skip the workflow completely:

```bash
git commit -m "chore: internal notes [skip ci]"
```

---

## Rollback

Rollback is automatic when a health check fails after deploy. The watcher swaps `current/` back to the previous release and restarts services without any intervention.

For manual rollback, stop the watcher, edit `version.txt`, then restart:

```powershell
nssm stop app-watcher
Set-Content D:\apps\admin-be\version.txt "v1.1.0"
nssm start app-watcher
```

The watcher detects the mismatch and re-deploys `v1.1.0` on the next tick.

---

## Disk layout

After the first deploy, the install directory looks like this:

```
D:\apps\admin-be\
  current\                  ← junction pointing to active release
    web.exe
    worker.exe
  releases\
    v1.1.0\                 ← previous version (kept for rollback)
      web.exe
      worker.exe
    v1.2.0\                 ← current version
      web.exe
      worker.exe
  .env.web.1
  .env.web.2
  .env.worker.1
  .env.worker.2
  version.txt               ← "v1.2.0"
  state.json                ← deployment status
  logs\
    admin-be-web-1.out.log
    admin-be-web-1.err.log
    ...

D:\apps\watcher\
  watcher.exe
  config.json
  install-watcher.ps1
  logs\
    watcher.out.log
    watcher.err.log
```

---

## state.json reference

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

| Status      | Meaning                                       |
| ----------- | --------------------------------------------- |
| `unknown`   | Watcher has never deployed to this server     |
| `deploying` | Deploy in progress                            |
| `healthy`   | Last deploy succeeded and health check passed |
| `failed`    | Deploy failed — see `last_error`              |
| `rollback`  | Rolled back to a previous version             |

---

## Logs

All logs are structured JSON written to `D:\apps\watcher\logs\watcher.out.log`.

```json
{"time":"2024-01-15T10:00:00Z","level":"INFO","component":"admin-be","msg":"check cycle","fields":{"metadata_url":"https://..."}}
{"time":"2024-01-15T10:00:01Z","level":"INFO","component":"admin-be","msg":"version mismatch, deploying","fields":{"from":"v1.1.0","to":"v1.2.0"}}
{"time":"2024-01-15T10:00:15Z","level":"INFO","component":"admin-be","msg":"health check passed","fields":{"service":"admin-be-web-1","attempt":2}}
{"time":"2024-01-15T10:00:17Z","level":"INFO","component":"admin-be","msg":"deploy complete","fields":{"version":"v1.2.0"}}
```

The `component` field always matches the `name` set in `config.json` — useful for filtering logs when watching multiple repos.

---

## config.json reference

| Field                                        | Required | Default                                  | Description                                       |
| -------------------------------------------- | -------- | ---------------------------------------- | ------------------------------------------------- |
| `environment`                                | no       | —                                        | Human-readable server label (logs only)           |
| `github_token`                               | yes      | —                                        | PAT with `repo` scope for private repos           |
| `log_dir`                                    | no       | `D:\apps\watcher\logs`                   | Where watcher writes its own logs                 |
| `nssm_path`                                  | no       | `C:\ProgramData\chocolatey\bin\nssm.exe` | Full path to nssm.exe                             |
| `watchers[].name`                            | no       | same as `service_name`                   | Label used in log output                          |
| `watchers[].service_name`                    | yes      | —                                        | Must match `APP_NAME` in the repo's `release.yml` |
| `watchers[].metadata_url`                    | yes      | —                                        | URL to `version.json` on GitHub Releases          |
| `watchers[].check_interval_sec`              | no       | 60                                       | Poll frequency in seconds                         |
| `watchers[].install_dir`                     | yes      | —                                        | Base directory for releases on this machine       |
| `watchers[].download_retries`                | no       | 3                                        | Retry attempts with exponential backoff           |
| `watchers[].health_check.enabled`            | no       | false                                    | Set `true` to validate after deploy               |
| `watchers[].health_check.url`                | no       | —                                        | Default health endpoint (overridable per service) |
| `watchers[].health_check.retries`            | no       | 10                                       | Attempts before rollback                          |
| `watchers[].health_check.interval_sec`       | no       | 3                                        | Seconds between retries                           |
| `watchers[].health_check.timeout_sec`        | no       | 5                                        | Per-request HTTP timeout                          |
| `watchers[].services[].windows_service_name` | yes      | —                                        | NSSM-managed Windows service name                 |
| `watchers[].services[].binary_name`          | yes      | —                                        | Filename inside the release zip                   |
| `watchers[].services[].env_file`             | yes      | —                                        | Path to the `.env` file for this instance         |
| `watchers[].services[].health_check_url`     | no       | —                                        | Overrides watcher-level health check URL          |
