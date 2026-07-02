# Config snapshot layout: per-service namespaced, mirrored files

Rollback needs to restore managed config to the state it was in at deploy time, not the current DB state. We capture a config snapshot into `releases/<version>/.watcher-snapshot/` immediately after `writeReleaseConfigFiles()` and before services start. The snapshot mirrors all three managed config categories using a per-service namespace subdirectory:

```
.watcher-snapshot/
  env/<windows_service_name>/<env_file>
  app/<windows_service_name>/<file_path>
  release/<windows_service_name>/<file_path>
```

During snapshot restoration, the service namespace is stripped and each file is written to its actual target path: `env/` → `<install_dir>/<env_file>`, `app/` → `<install_dir>/<file_path>`, `release/` → `<current_dir>/<file_path>`.

## Considered options

**Single JSON file** (`config.json`) was rejected in favour of mirrored files because the snapshot is a persistent, user-editable artefact. Operators should be able to inspect and modify individual config files directly without parsing JSON, and the mirrored layout makes the mapping to on-disk paths self-evident.

**Flat layout** (no per-service subdirectory) was rejected for `env/` and `app/` because multiple services under one watcher can write to the same relative path, causing silent last-writer-wins collisions. Per-service namespacing makes ownership auditable. The `release/` category uses the same namespacing for consistency, even though `writeReleaseConfigFiles()` writes flat to `current/` — the namespace is stripped on restore.

**Separate `<install_dir>/config-snapshots/<version>/`** was rejected in favour of co-location inside the release dir. `CleanOldReleases()` already governs release dir retention, so co-location means snapshot lifecycle is automatically tied to version retention with no additional cleanup logic. Rollback already requires the release dir to exist, so there is no scenario where a snapshot would need to outlive its release dir.

## Consequences

- Snapshots written before this feature existed are backfilled at agent startup using current DB config. Backfilled snapshots are an approximation and are logged as such.
- Snapshots are plain files with no immutability enforcement. Operators may edit or delete them freely; rollback reads whatever is present at restoration time.
- If no snapshot exists for the rollback target (deleted by operator, or backfill failed), rollback proceeds without config restoration and logs a warning — same behaviour as before this feature.
