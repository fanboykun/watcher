# Watcher — Agent Guidelines

## Project Identity

**Name**: Watcher  
**Module**: `github.com/fanboykun/watcher`  
**Language**: Go 1.21+ with SvelteKit frontend  
**Dependencies**: Gin (HTTP), GORM + pure-Go SQLite, godotenv; SvelteKit + Tailwind + shadcn-svelte  
**Target OS**: Windows 10/11 / Windows Server 2022 (cross-compiled from Linux/WSL)  
**Purpose**: Pull-based deployment agent with web dashboard. Runs as a Windows service, polls GitHub Releases, deploys new versions automatically, and provides a management UI.

---

## Architecture Overview

```
main.go (entrypoint)
  ├── API Server (Gin)
  │     ├── /api/*        REST endpoints (handlers.go, service_handlers.go, system_handlers.go)
  │     └── /*            Embedded SvelteKit SPA (via go:embed + NoRoute fallback)
  ├── Agent (watcher.go)
  │     ├── RepoWatcher[0] (goroutine)
  │     │     ├── GitHubClient   (github.go)   — fetch version.json, download artifacts
  │     │     ├── Deployer       (deploy.go)   — extract, swap, NSSM, rollback, health
  │     │     └── StateManager   (state.go)    — deploy status via SQLite
  │     ├── RepoWatcher[1] ...
  │     └── RepoWatcher[N] ...
  └── SQLite Database (GORM)
        ├── watchers        — watched repo configurations
        ├── services        — NSSM service definitions
        ├── deploy_logs     — deploy history with version, status, duration
        └── health_events   — health check results
```

### Key design decisions

- **One goroutine per watched repo** — each `RepoWatcher` has its own ticker, poll loop, and state
- **SQLite via pure-Go driver** — `github.com/glebarez/sqlite` (no CGO needed for cross-compilation)
- **Embedded SPA** — SvelteKit dashboard bundled into the binary via `go:embed`
- **Single binary** — one `.exe` serves API + dashboard + runs the agent
- **Junction-based swap** — `mklink /J` for zero-downtime; falls back to `copyDir` if junctions fail
- **Auto-registration** — `ensureService()` installs NSSM services on first deploy
- **Automatic rollback** — if health check fails after deploy, rolls back to previous version
- **API-triggered checks** — dashboard can trigger immediate polls via a channel to the agent

---

## Directory Structure

```
watcher/
  cmd/watcher/main.go              entrypoint — flag parsing, signal handling, starts Agent + API
  internal/
    agent/
      watcher.go                    Agent + RepoWatcher — orchestration and poll loop
      deploy.go                     Deployer — extract, swap junction, NSSM, health, rollback
      github.go                     GitHubClient — version.json + artifact download (public/private)
      github_test.go                Tests for GitHubClient (httptest-based)
      state.go                      StateManager — deploy state tracking via SQLite
      logger.go                     Structured JSON logger (custom, no slog/zap/zerolog)
      ticker.go                     newTicker() helper
    api/
      router.go                     Gin router — API routes + embedded SPA serving
      handlers.go                   Watcher CRUD + deploy log handlers
      service_handlers.go           Service management (start/stop/restart/health/logs)
      system_handlers.go            System status + agent log tail
      dto.go                        Request/response DTOs
    config/
      config.go                     LoadConfig() from .env via godotenv
    database/
      database.go                   SQLite via GORM (pure-Go, no CGO)
      models.go                     Watcher, Service, DeployLog, HealthEvent models
  web/
    embed.go                        go:embed all:build — bundles SPA into binary
    build/                          SvelteKit build output (gitignored, built by `make build-web`)
    src/
      routes/                       SvelteKit pages (dashboard, watchers, services, logs)
      lib/api.ts                    Typed API client
      lib/components/ui/            shadcn-svelte components
  install.bat                       Wrapper to auto-elevate and launch the GUI wizard
  shell/
    install-watcher.ps1             GUI Bootstrap — installs dependencies, registers service
  workflows/
    release.yml                     Template release workflow for watched app repos
  .github/workflows/
    release.yml                     This repo's release workflow (builds SPA + Go binary)
  .env.example                      Example config (committed to git)
  .env                              Live config (gitignored, contains real token)
  Makefile                          Build, test, dev, package targets
  README.md                         Full documentation
  INSTALL.md                        Step-by-step installation guide
  go.mod / go.sum                   Go module definition
```

---

## Config

Configuration is loaded from a `.env` file via `godotenv`:

```env
ENVIRONMENT=production
GITHUB_TOKEN=ghp_xxx
LOG_DIR=D:\apps\watcher\logs
NSSM_PATH=C:\ProgramData\chocolatey\bin\nssm.exe
DB_PATH=D:\apps\watcher\watcher.db
API_PORT=8080
```

Watchers and services are stored in the **SQLite database**, managed via the dashboard or REST API. There is no `config.json` anymore.

---

## Database Models

```
Watcher
  ├── id, name, service_name, metadata_url
  ├── check_interval_sec, download_retries, install_dir
  ├── hc_enabled, hc_url, hc_retries, hc_interval_sec, hc_timeout_sec
  ├── current_version, status, last_checked, last_deployed, last_error
  └── has many Services, DeployLogs

Service
  ├── id, watcher_id (FK)
  ├── windows_service_name, binary_name, env_file, health_check_url
  └── has many HealthEvents

DeployLog
  ├── id, watcher_id (FK)
  ├── version, from_version, status, error
  ├── started_at, finished_at, duration_ms

HealthEvent
  ├── id, service_id (FK)
  ├── status (healthy/unhealthy), status_code, response_time_ms, error
  ├── checked_at
```

---

## Deploy Flow

```
1. Poll version.json from GitHub (API for private, direct for public)
2. Compare remote version vs database current_version
3. If mismatch → start deploy:
   a. Record DeployLog (status: deploying)
   b. Download artifact zip (with retry + exponential backoff)
   c. Extract to releases/<version>/
   d. Stop all NSSM services
   e. Swap current/ junction → releases/<version>/
   f. ensureService() — install via NSSM if first deploy, or update binary path
   g. Start all NSSM services
   h. Health check each service (if enabled)
   i. On failure: rollback to previous version (reverse of above)
   j. Update DeployLog (status: healthy/failed, duration_ms)
   k. Update Watcher state in database
4. Clean up downloaded zip
```

---

## Code Conventions

### Go style
- **Packages**: `internal/agent/`, `internal/api/`, `internal/config/`, `internal/database/`
- **Naming**: Standard Go conventions (camelCase unexported, PascalCase exported)
- **Error handling**: Wrap all errors with `fmt.Errorf("context: %w", err)` using `%w` for unwrapping
- **Logging**: Use structured key-value pairs: `log.Info("message", "key1", val1, "key2", val2)`
- **Constructor pattern**: `NewXxx()` functions return struct pointers

### Logger
- Custom structured JSON logger (not slog/zap/zerolog)
- Per-component context via `log.WithComponent("name")`
- Writes to both stdout and file via `io.MultiWriter`

### SPA Embedding
- `web/embed.go` uses `//go:embed all:build` to bundle the SvelteKit output
- `router.go` serves SPA via `NoRoute` handler with fallback to `index.html`
- Static assets served with correct MIME types via custom `contentType()` function
- During dev, Vite proxies `/api` to the Go backend (see `vite.config.ts`)

### Frontend
- SvelteKit with `adapter-static` (SPA mode, SSR disabled)
- Tailwind CSS + shadcn-svelte components
- Typed API client in `src/lib/api.ts`
- All routes under `src/routes/` (dashboard, watchers, services, logs)

---

## Testing Patterns

- Tests are in `internal/agent/github_test.go` (same package, white-box)
- Uses `net/http/httptest` for HTTP mocking (no third-party mock libs)
- Test helper: `newTestClient()` — creates GitHubClient with test server
- Test helper: `makeTestZip()` — creates in-memory zip files
- Table-driven tests for URL parsing functions
- All tests use `t.TempDir()` for filesystem artifacts
- Run: `make test` or `make test-github`

---

## Build & Release

### Local development
```bash
make dev           # Air hot-reload for Go backend
make build-web     # Build SvelteKit SPA only
make build         # Build SPA + cross-compile watcher.exe for Windows
make test          # Run all tests
make test-verbose  # Run tests with -v
make test-github   # Run only GitHub client tests
make package       # Build + zip for distribution
make clean         # Remove bin/ and web/build/
make info          # Print Go environment
```

### CI/CD release (`.github/workflows/release.yml`)
Triggered on push to `main` (specific paths). Two-job pipeline:

1. **Semantic Version Tag** — analyzes commit messages, determines bump type, pushes tag
2. **Build & Package Release**:
   - Sets up Go + Bun
   - Builds SvelteKit SPA (`bun install + bun run build`)
   - Cross-compiles Go with embedded SPA (`CGO_ENABLED=0 GOOS=windows`)
   - Assembles release zip (watcher.exe + shell/ + .env.example + INSTALL.md)
   - Generates version.json
   - Creates GitHub Release

### Commit message conventions

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

**Fallback**: If only skip-pattern commits but important files changed, defaults to patch.

---

## Disk Layout on Windows

```
D:\apps\watcher\                    ← watcher agent home
  watcher.exe                       ← single binary (agent + API + dashboard)
  .env                              ← SECURED (SYSTEM + Admins only)
  watcher.db                        ← SQLite database
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
  state.json                        ← legacy deploy status (kept for file-based state)
  .env.web.1                        ← service instance env file (not managed by watcher)
  logs\
    <service>-web-1.out.log
```

---

## Critical Contracts

### Between release.yml (app repo) and watcher database

| `release.yml` env | Watcher field | Rule |
|---|---|---|
| `APP_NAME` | `watcher.service_name` | Exact string match — this is the key in version.json |
| `WEB_BINARY` | `service.binary_name` | Exact filename inside the zip |
| `WORKER_BINARY` | `service.binary_name` | Exact filename inside the zip |

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

- `.env` contains a GitHub PAT — file permissions restricted by install script
- The PAT needs `repo` scope for private repos, or `contents:read` for fine-grained PATs
- `.env` files are gitignored
- API has no authentication (assumed internal network use)

---

## Common Gotchas

1. **`APP_NAME` must match `service_name`** — if these are out of sync, the watcher silently fails to find the service in version.json
2. **Binary names must match** — `binary_name` in database must exactly match the filename inside the release zip
3. **`.env` files must exist before first deploy** — the watcher deploys binaries but does NOT manage `.env` files for watched services
4. **Windows junctions** — `mklink /J` requires the target directory to exist; the fallback `copyDir` is slower but always works
5. **Health check URLs** — per-service URL overrides the watcher-level URL; if neither is set and `enabled=true`, health checks are silently skipped
6. **NSSM service names** — once registered, changing the name in the database won't unregister the old service
7. **Pure-Go SQLite** — uses `github.com/glebarez/sqlite` (no CGO). Enables cross-compilation with `CGO_ENABLED=0`
8. **SPA must be built before Go** — `web/build/` must exist for `go:embed`. Use `make build` which handles this automatically
