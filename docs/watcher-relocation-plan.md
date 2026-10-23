# Managed pool relocation — planned, not implemented

Changing a watcher's deployment root is a migration of the managed application
pool. It is separate from moving or updating the Watcher executable.

## Confirmed failure

The old edit handler changed only `watchers.install_dir` and requested worker
sync. Downloaded catalog rows still owned their old root. Release listing and
snapshots looked under the new root; Windows services and IIS could still point
at the old one. Sync cancelled an existing worker and waited for its compensation,
but the database path had already changed before that join completed.

The immediate fix makes `install_dir` immutable through ordinary settings edits.
Rejected changes do not save other settings, request sync, or touch either root.
This closes the edit path without attempting an unsafe filesystem move.

## Required relocation flow

1. Reserve relocation durably for the watcher. All download, deployment,
   approval, rollback, service/config mutation, and deletion entry points must
   honor the same reservation. Pausing scheduled polls alone is insufficient.
2. Quiesce the worker and join any active deployment and compensation. Re-read
   the final committed pool state after the join. Settle queued work explicitly.
3. Validate source/destination ownership, permissions, free space, and overlapping
   pool roots. Reject destinations nested in either pool or another managed
   pool. Preserve the original pool until the operation fully succeeds.
4. Copy to a staging destination and verify releases, cached artifact checksums,
   snapshots, app-dir configs, and other pool files. Rebuild `current` against
   the destination release; copying an old absolute junction is insufficient.
5. Switch runtime paths under a service maintenance window: NSSM executable,
   AppDirectory, stdout/stderr, and applicable IIS paths. Validate service state
   and health. Every switched setting must have a recorded compensation step.
6. Commit the watcher root and all catalog artifact roots together only after
   successful activation. Preserve candidate/version identity and configuration
   history. Release the reservation and resume the worker at the committed root.
7. Persist enough progress to recover a process restart at every step. A failure
   restores the old runtime paths and committed root; incomplete copies must
   never become deployable. Old-root cleanup is a later explicit operation.

Implementation must define how existing open browser sessions and downloads
observe relocation progress and receive actionable conflicts, rather than
writing through a stale root.

## Already affected pools

Repair requires an inventory of the configured root, artifact-owned roots,
retained releases, trusted snapshots, `current` targets, and actual Windows
service/IIS settings. A deployment may have written to either root after the
edit. Do not infer the correct state from `current_version` alone or overwrite a
root based only on a missing download list. This repair flow is not implemented.

## Verification

Use small temporary-directory and API/agent tests for interrupted copies,
cross-volume moves, concurrent download/deploy/rollback admission, queued work,
unchanged pending artifact IDs, DB failure, and restart recovery. Validate real
NSSM/IIS switching and junction behavior separately on Windows. Browser checks
remain optional and serial under the resource policy in `web/scripts/README.md`.
