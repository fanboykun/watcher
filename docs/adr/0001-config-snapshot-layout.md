# Config snapshot layout: private, versioned, and authoritative

Rollback must restore managed configuration from the selected version rather
than reuse the current database values. Snapshots therefore live outside the
release and `current` trees, where an IIS/static application cannot serve their
secrets:

```text
<install_dir>/.watcher/snapshots/<encoded-version>/
  manifest.json
  services/<service-name>/
    env/<env_file>
    app/<file_path>
    release/<file_path>
```

`manifest.json` records the exact version, capture source, and capture time. A
directory without a valid manifest is not a trusted snapshot. The service
namespace is stripped during restoration: `env/` and `app/` write beneath the
install directory, while `release/` writes beneath the activated `current`
directory.

Snapshots are captured before service shutdown for a deployment. Updating
managed env or config through the API atomically replaces the active version's
snapshot. Empty managed files are retained because an empty value is different
from a missing historical value.

## Migration

Legacy `releases/<version>/.watcher-snapshot` directories have no provenance:
some were captured during a real deployment while others were approximated
from newer database values. Watcher does not trust them for rollback.

At startup, Watcher:

1. captures a trusted snapshot only for the currently active version when it
   does not already have one;
2. moves legacy directories to
   `<install_dir>/.watcher/legacy-snapshots/<version>` for operator recovery;
3. does not fabricate snapshots for older retained releases.

Consequently, an old version without a trusted snapshot cannot be selected for
automatic or manual rollback until the operator deliberately provides one or
redeploys that version.

## Failure contract

Snapshot presence and manifest validity are checked before any service is
stopped. Restoration errors fail the rollback and activate the existing
compensation path; Watcher never reports a rollback as successful while
silently retaining another version's configuration.

Release retention and explicit version deletion remove the matching trusted
snapshot and stored GitHub release metadata. The quarantined legacy copies are
retained because they are recovery evidence, not trusted runtime state.

## Considered options

Keeping snapshots inside each release was rejected because `current` is a
junction to that directory and could expose configuration secrets through an
IIS/static web root. Backfilling every historical version from current database
configuration was rejected because it invents history and caused old versions
to restore the newest env.

A single JSON payload for secret contents was also rejected. Mirrored files
remain directly inspectable, while the small JSON manifest contains provenance
only.
