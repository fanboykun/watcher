# Agent module

`internal/agent` owns the background runtime that discovers versions, deploys
artifacts, manages Windows services, compensates failed deployments, and records
the resulting state. HTTP request handling and database schema ownership remain
outside this package.

## Runtime flow

```text
Agent.Run
  -> syncWatchers
     -> one RepoWatcher per database watcher
        -> RepoWatcher.Run
           -> fetch metadata for the configured release ref
           -> compare remote, current, and rollback high-watermark versions
           -> enforce the per-version retry limit
           -> RepoWatcher.deploy
              -> create GitHub Deployment status when enabled
              -> download artifact
              -> Deployer.Deploy
              -> persist the successful version and attempt state
```

The supervisor restarts a watcher goroutine when its database `updated_at`
changes and stops it when the row is removed. API-triggered checks share the
same `RepoWatcher.Run` path as scheduled checks.

## Deployment and compensation

The deployment pipeline keeps preparation before service disruption:

1. Extract the artifact to a temporary staging directory.
2. Validate every required NSSM binary.
3. Write release-scoped managed configuration and capture its private version snapshot.
4. Stop configured services.
5. Promote the staged directory into `releases/<encoded-version>`.
6. Activate the release through the `current` junction or copy fallback.
7. Ensure NSSM or IIS registration and start/recycle services.
8. Run configured health checks.
9. Remove the promotion backup and enforce release retention.

If failure occurs before activation, stopped services are restarted against the
existing release. If failure occurs after activation, compensation first tries
a real rollback to the recorded previous version. When no valid previous
version exists, it restores the promotion backup for the same version. These
outcomes are different: only a successful version rollback produces a rollback
attempt and rollback version in persisted state.

Manual rollback validates the retained target before stopping services. If its
startup or health check fails, Watcher restores the original release and reports
both failures when restoration also fails.

## NSSM lifecycle contract

`service_manager.go` is the only owner of NSSM lifecycle interpretation. It
handles every SCM state returned by `nssm status`:

| State | Start behavior | Stop behavior |
| --- | --- | --- |
| `SERVICE_STOPPED` | issue `start`, then wait | already stopped |
| `SERVICE_START_PENDING` | wait for running | wait for running, then stop |
| `SERVICE_STOP_PENDING` | wait for stopped, then start | wait for stopped |
| `SERVICE_RUNNING` | already running | issue `stop`, then wait |
| `SERVICE_CONTINUE_PENDING` | wait for running | wait for running, then stop |
| `SERVICE_PAUSE_PENDING` | wait for paused, then continue | wait for paused, then stop |
| `SERVICE_PAUSED` | issue `continue`, then wait | issue `stop`, then wait |

An NSSM command can return a non-zero exit code while the SCM is still moving
through a valid transitional state. Lifecycle commands therefore verify status
before declaring failure. A terminal `SERVICE_STOPPED` during startup fails
immediately instead of waiting for the full timeout.

## File ownership

| File | Responsibility |
| --- | --- |
| `agent.go` | Watcher goroutine supervision and database reconciliation. |
| `watcher.go` | One poll cycle and update eligibility. |
| `watcher_config.go` | Database-to-runtime configuration mapping. |
| `watcher_deployment.go` | Artifact acquisition and GitHub Deployment reporting. |
| `watcher_failure.go` | Failure classification and rollback-attempt persistence. |
| `deploy.go` | Successful-path deployment orchestration. |
| `deploy_recovery.go` | Automatic and manual rollback compensation. |
| `archive.go` | Safe zip extraction. |
| `release_preparation.go` | Release validation and atomic promotion backup. |
| `release_activation.go` | `current` junction activation and copy fallback. |
| `release_retention.go` | Version listing, retention, and deletion. |
| `managed_config.go` | Validated managed config paths and release-scoped writes. |
| `config_snapshot.go` | Trusted external snapshot capture, validation, and restore. |
| `config_snapshot_migration.go` | Current-version migration and legacy snapshot quarantine. |
| `release_metadata.go` | GitHub repository/ref identity retained for rollback reporting. |
| `service_manager.go` | NSSM status parsing and lifecycle state machine. |
| `service_deployment.go` | NSSM registration and deploy-time service ordering. |
| `iis.go` | IIS application-pool and site registration. |
| `health_check.go` | Post-start HTTP health checks. |
| `github.go`, `github_*.go` | GitHub client types, metadata, assets, releases, inspection, and Deployment API. |
| `state.go` | Watcher state, attempt lineage, logs, poll events, and webhooks. |
| `events.go` | In-process watcher event fan-out. |
| `logger.go` | Structured logging and file-output ownership. |
| `self_update.go` | Watcher executable update and restart. |
| `version.go` | Semantic-version comparison and rollback high-watermark policy. |

## Invariants

- The recorded database version is the authority for rollback selection; a
  first deployment must not infer a previous version from stale directories.
- Artifact validation finishes before service shutdown.
- The failed deployment context must not cancel compensation.
- `current` is changed only after the staged release is ready.
- Required NSSM registration settings fail deployment; optional logging and
  rotation settings warn without invalidating an otherwise runnable service.
- Service lifecycle decisions use NSSM status, not command exit code alone.
- A same-version backup restoration is not reported as a version rollback.

## Focused verification

From the repository root:

```sh
go test -p=1 ./internal/agent -count=1
go vet ./internal/agent
make test-e2e
```

`make test-e2e` starts the real Gin router and background agent against a
temporary SQLite database, fake GitHub release server, real deployment
filesystem, and executable fake NSSM boundary. It covers both a successful
first deployment and a failed first deployment that must not create a rollback
attempt.

The portable tests cover the NSSM state contract and compensation branches. A
real Windows/NSSM integration run remains the release gate for SCM timing and
junction behavior.
