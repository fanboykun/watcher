# Watcher

A pull-based deployment agent that polls GitHub releases and deploys versioned artifacts to Windows hosts, with a dashboard for monitoring and control.

## Language

### Deployment

**Watcher**:
A configured deployment target that polls a GitHub repository or `version.json` feed for new releases and orchestrates their deployment.
_Avoid_: job, task, monitor

**Deploy**:
The full pipeline of downloading, extracting, stopping services, swapping the `current` junction, writing managed config, registering services, starting services, and health-checking for a specific version.
_Avoid_: release, publish, push

**Rollback**:
Reverting the `current` junction to a previously deployed version and restoring that version's config snapshot, then re-registering and starting services.
_Avoid_: revert, undo, downgrade

**Release dir**:
The versioned directory on disk where an artifact is extracted: `<install_dir>/releases/<version>/`.
_Avoid_: version dir, artifact dir

**Current junction**:
The `<install_dir>/current/` directory junction (Windows `mklink /J`) that always points to the active release dir.
_Avoid_: symlink, current dir, active version

**Install dir**:
The root directory for a watcher on disk, containing `releases/`, `current/`, `downloads/`, `logs/`, and private `.watcher/` state.
_Avoid_: app dir, base dir, root dir

### Config

**Managed config**:
All files whose content is stored in the DB and written to disk by Watcher on deploy: env files, app-dir config files, and release-dir config files.
_Avoid_: config files, env files, settings

**Env file**:
A per-service file written to `<install_dir>/<env_file>` containing the service's `EnvContent` blob, passed to NSSM as `AppEnvironmentExtra`.
_Avoid_: dotenv, environment file, .env

**App-dir config file**:
A `ServiceConfigFile` with `target = "app_dir"`, written to a stable path under `<install_dir>/` — persists across releases.
_Avoid_: static config, install-dir file

**Release-dir config file**:
A `ServiceConfigFile` with `target = "release_dir"`, written into `current/` after each junction swap — version-scoped.
_Avoid_: versioned config, current config

### Snapshots

**Config snapshot**:
A trusted mirrored copy of all managed config for a specific version, stored under `<install_dir>/.watcher/snapshots/<version>/` with a provenance manifest. Used by rollback to restore config to the state owned by that version.
_Avoid_: config backup, config archive, config dump

**Legacy snapshot**:
An untrusted snapshot from the former in-release layout. Startup migration moves it to `.watcher/legacy-snapshots/` for operator recovery but never uses it automatically for rollback.
_Avoid_: backfilled snapshot, trusted snapshot

**Snapshot restoration**:
The act of reading a config snapshot during rollback and writing each file to its actual target path on disk, before services are re-registered and started.
_Avoid_: config restore, snapshot apply
