# Watcher — Agent Guidelines

## Project Identity

**Name**: Watcher  
**Module**: `github.com/fanboykun/watcher`  
**Language**: Go 1.25 (stdlib only — zero external dependencies)  
**Target OS**: Windows Server 2022 (cross-compiled from Linux)  
**Purpose**: Pull-based deployment agent that runs as a Windows service, polls GitHub Releases, and deploys new versions automatically.

---

## Architecture Overview

```
main.go (entrypoint)
  └── Agent (watcher.go)
        ├── RepoWatcher[0] (goroutine — watcher.go)
        │     ├── GitHubClient   (github.go)   — fetch version.json, download artifacts
        │     ├── Deployer       (deploy.go)   — extract, swap, NSSM, rollback, health
        │     └── StateManager   (state.go)    — version.txt + state.json
        ├── RepoWatcher[1] ...
        └── RepoWatcher[N] ...

Supporting:
  config.go   — Config structs, LoadConfig(), validation
  logger.go   — Structured JSON logger with per-component context
  ticker.go   — Simple ticker helper
```

### Key design decisions

- **One goroutine per watched repo** — each `RepoWatcher` has its own ticker, poll loop, and state
- **Zero dependencies** — only stdlib; no third-party packages
- **Atomic writes** — all state files use write-to-tmp + rename pattern
- **Junction-based swap** — `mklink /J` for zero-downtime; falls back to `copyDir` if junctions fail
- **Auto-registration** — `ensureService()` installs NSSM services on first deploy (no manual setup)
- **Automatic rollback** — if health check fails after deploy, rolls back to previous version

---

## Directory Structure

```
watcher/
  cmd/watcher/main.go           entrypoint — flag parsing, signal handling, starts Agent
  internal/
    config.go                    Config structs + LoadConfig() + validation
    watcher.go                   Agent + RepoWatcher — orchestration and poll loop
    deploy.go                    Deployer — extract, swap junction, NSSM, health, rollback
    github.go                    GitHubClient — version.json + artifact download (public/private)
    github_test.go               Tests for GitHubClient (httptest-based, no mocking libs)
    state.go                     StateManager — version.txt + state.json (atomic writes)
    logger.go                    Structured JSON logger (custom, no slog/zap/zerolog)
    ticker.go                    newTicker() helper
  shell/
    install-watcher.ps1          Bootstrap script — registers watcher as Windows service via NSSM
  workflows/
    release.yml                  Template release workflow for watched app repos
    deploy.yml                   Template CI/CD workflow (SSH-based, legacy approach)
  .github/workflows/
    release.yml                  This repo's own release workflow (builds watcher.exe)
  config.json                    Live config (gitignored, contains real token)
  config.web-only.json           Example config variant (old format, single-watcher)
  config.example.json            Clean example config (committed to git, included in release zip)
  Makefile                       Build, test, package, run targets
  README.md                      Full documentation
  INSTALL.md                     Step-by-step installation guide
  go.mod                         Module definition (no go.sum — no dependencies)
```

---

## Config Structure

The project uses a **multi-watcher** config format. One watcher agent can watch multiple repos:

```
Config (top-level)
  ├── environment        (string, informational)
  ├── github_token       (string, PAT with repo scope)
  ├── log_dir            (string, default: C:\apps\watcher\logs)
  ├── nssm_path          (string, default: C:\ProgramData\chocolatey\bin\nssm.exe)
  └── watchers[]         (array of WatcherConfig)
        ├── name                  (string, log label)
        ├── service_name          (string, MUST match key in version.json)
        ├── metadata_url          (string, URL to version.json)
        ├── check_interval_sec    (int, default: 60)
        ├── download_retries      (int, default: 3)
        ├── install_dir           (string, e.g. D:\apps\my-service)
        ├── health_check
        │     ├── enabled         (bool)
        │     ├── url             (string, default health endpoint)
        │     ├── retries         (int, default: 10)
        │     ├── interval_sec    (int, default: 3)
        │     └── timeout_sec     (int, default: 5)
        └── services[]
              ├── windows_service_name  (string, NSSM service name)
              ├── binary_name           (string, filename in zip)
              ├── env_file              (string, path to .env)
              └── health_check_url      (string, per-service override)
```

> **Important**: `config.web-only.json` uses an **older flat format** (single watcher, no `watchers[]` array). The current code expects the multi-watcher format.

---

## Deploy Flow

```
1. Poll version.json from GitHub (API for private, direct for public)
2. Compare remote version vs local version.txt
3. If mismatch → start deploy:
   a. Download artifact zip (with retry + exponential backoff)
   b. Extract to releases/<version>/
   c. Stop all NSSM services
   d. Swap current/ junction → releases/<version>/
   e. ensureService() — install via NSSM if first deploy, or update binary path
   f. Start all NSSM services
   g. Health check each service (if enabled)
   h. On failure: rollback to previous version (reverse of above)
   i. Write version.txt + state.json
4. Clean up downloaded zip
```

---

## GitHub API Strategy

The `GitHubClient` handles both public and private repos transparently:

- **Public repos** (`github_token` empty): Direct HTTP GET to the releases URL
- **Private repos** (`github_token` set): Uses GitHub API to find release assets by name, then downloads via asset API URL with `Accept: application/octet-stream`

URL parsing functions:
- `parseGitHubURL()` — extracts `owner/repo` from metadata URL
- `parseArtifactURL()` — extracts `owner/repo/assetName` from artifact download URL

---

## Code Conventions

### Go style
- **Package**: All Go code lives in `internal/` (single flat package, no sub-packages)
- **Naming**: Standard Go conventions (camelCase unexported, PascalCase exported)
- **Error handling**: Wrap all errors with `fmt.Errorf("context: %w", err)` using `%w` for unwrapping
- **Logging**: Use structured key-value pairs: `log.Info("message", "key1", val1, "key2", val2)`
- **Constructor pattern**: `NewXxx()` functions return struct pointers
- **No interfaces**: Concrete types only (no DI/IoC patterns)

### Logger
- Custom structured JSON logger (not slog/zap/zerolog)
- Per-component context via `log.WithComponent("name")`
- Writes to both stdout and file via `io.MultiWriter`
- Error values are automatically converted to strings in log output

### State management
- `version.txt` — single line, current deployed version (e.g. `v1.2.0\n`)
- `state.json` — structured status with `current_version`, `status`, `last_checked`, `last_deployed`, `last_error`
- Status enum: `unknown` | `deploying` | `healthy` | `failed` | `rollback`
- All writes use atomic tmp+rename pattern

### Windows-specific
- Junctions via `cmd /C mklink /J <target> <src>` with fallback to directory copy
- NSSM for service management (install, set, start, stop, status)
- 2-second sleep after stop/start to let services settle
- Service existence check via `nssm status <name>` exit code + output parsing

---

## Testing Patterns

- Tests are in `internal/github_test.go` (same package, white-box)
- Uses `net/http/httptest` for HTTP mocking (no third-party mock libs)
- Test helper: `newTestClient()` — creates GitHubClient with test server, overrides `apiBase`
- Test helper: `makeTestZip()` — creates in-memory zip files
- Table-driven tests for URL parsing functions
- All tests use `t.TempDir()` for filesystem artifacts
- Run tests: `make test` or `go test ./internal/ -count=1`
- Run specific: `make test-github` (URL parsing + metadata + download tests)

---

## Build & Release

### Local development
```bash
make build         # Cross-compile watcher.exe for Windows (bin/watcher.exe)
make test          # Run all tests
make test-verbose  # Run tests with -v
make test-github   # Run only GitHub client tests
make run           # Run locally (uses config.json, native OS — won't work for NSSM)
make package       # Build + zip with install-watcher.ps1 + config.example.json + INSTALL.md
make clean         # Remove bin/
make info          # Print Go environment
```

### CI/CD release (`.github/workflows/release.yml`)
Triggered on push to `main` (specific paths only). Two-job pipeline:

1. **Semantic Version Tag** — analyzes commit messages since last tag, determines bump type, pushes new git tag
2. **Build & Package Release** — compiles for Windows, assembles release zip (watcher.exe + shell/ + config.example.json + INSTALL.md), generates version.json, creates GitHub Release

### Commit message conventions
The release workflow uses conventional commits with extended patterns:

| Pattern | Bump |
|---------|------|
| `feat:` | minor |
| `fix:` / `perf:` / `refactor:` | patch |
| `feat!:` / `BREAKING CHANGE` | major |
| `chore:` / `docs:` / `ci:` / `style:` / `test:` / `build:` | skip |
| `minor:` / `patch:` / `major:` | respective |
| `bump: minor` / `patch` / `major` | respective |
| `release:` / `[release]` | patch |
| `release(minor):` / `[release:minor]` | minor |
| `release(major):` / `[release:major]` | major |

### Workflow templates (`workflows/` directory)
- `release.yml` — Template for watched app repos. Copy into `.github/workflows/release.yml` of target repos. Produces the `version.json` + artifact zip that the watcher expects.
- `deploy.yml` — Legacy SSH-based deploy workflow (alternative to watcher-based deploys)

---

## Disk Layout on Windows Server

```
D:\apps\watcher\                    ← watcher agent home
  watcher.exe
  config.json                       ← SECURED (SYSTEM + Admins only)
  shell\install-watcher.ps1
  logs\
    watcher.out.log                 ← NSSM stdout redirect
    watcher.err.log                 ← NSSM stderr redirect

D:\apps\<service>\                  ← per-service install directory
  current\                          ← junction → releases/<version>/
  releases\
    v1.0.0\                         ← previous version (kept for rollback)
    v1.1.0\                         ← current version
  version.txt                       ← "v1.1.0"
  state.json                        ← deploy status
  .env.web.1                        ← service instance env file (not managed by watcher)
  .env.worker.1
  logs\
    <service>-web-1.out.log
    <service>-web-1.err.log
```

---

## Critical Contracts

### Between release.yml (app repo) and config.json (watcher)

| `release.yml` env | `config.json` field | Rule |
|---|---|---|
| `APP_NAME` | `watchers[].service_name` | Exact string match — this is the key in version.json |
| `WEB_BINARY` | `services[].binary_name` | Exact filename inside the zip |
| `WORKER_BINARY` | `services[].binary_name` | Exact filename inside the zip |

### version.json format (must match `VersionMetadata` struct)
```json
{
  "services": {
    "<service_name>": {
      "version": "v1.2.0",
      "artifact": "<name>-v1.2.0.zip",
      "artifact_url": "https://github.com/<owner>/<repo>/releases/download/v1.2.0/<name>-v1.2.0.zip",
      "published_at": "2024-01-15T10:00:00Z"
    }
  }
}
```

### Zip artifact format
Binaries MUST be at the root of the zip (not nested in subdirectories).

---

## Security Notes

- `config.json` contains a GitHub PAT — file permissions should be restricted (the install script handles this)
- The PAT needs `repo` scope for private repos, or `contents:read` for fine-grained PATs
- Config files with real tokens are gitignored (`config*.json` except `config.example.json`)
- `.env` files are also gitignored

---

## Common Gotchas

1. **`config.web-only.json` is outdated** — uses old single-watcher flat format, not the current `watchers[]` array format
2. **`APP_NAME` must match `service_name`** — if these are out of sync, the watcher silently fails to find the service in version.json
3. **Binary names must match** — `binary_name` in config must exactly match the filename inside the release zip
4. **`.env` files must exist before first deploy** — the watcher deploys binaries but does NOT manage `.env` files
5. **Windows junctions** — `mklink /J` requires the target directory to exist; the fallback `copyDir` is slower but always works
6. **Health check URLs** — per-service URL overrides the watcher-level URL; if neither is set and `enabled=true`, health checks are silently skipped for that service
7. **NSSM service names** — once registered, changing the name in config won't unregister the old service
