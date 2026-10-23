# Plan: Deployment Candidates & Version-Bound Configuration

## The Problem
Treating pending configurations as a single floating "draft" is insufficient for robust release management. Developers need the ability to prepare configurations specifically tailored to a particular upcoming version (e.g., `v2.0.0-rc1`) long before it even drops. Furthermore, each downloaded version should be treated as an isolated "Deployment Candidate" with its own configuration lifecycle, ensuring that updates are completely deterministic and safe from human error.

## The Solution: Version-Bound Configurations
The **Deployment Candidate Configuration** architecture is implemented locally. Configurations are no longer just global or floating drafts; they can be bound to specific version tags.

---

### Phase 1: Database Schema Modifications
Configuration revisions live in a dedicated table; the old floating `StagedEnvContent` fields are removed. Managed config revisions extend this table without replacing env-only drafts.

1. **`ServiceConfigRevision` Model**:
   A new table to store configurations for specific targets.
   ```go
   type ServiceConfigRevision struct {
       ID            uint      `gorm:"primaryKey" json:"id"`
       ServiceID     uint      `gorm:"not null;uniqueIndex:idx_service_revision_target" json:"service_id"`
       TargetVersion string    `gorm:"not null;uniqueIndex:idx_service_revision_target" json:"target_version"` // e.g., "1.2.0-rc1" or "next"
       EnvContent    string    `gorm:"type:text" json:"env_content"`
       ConfigFiles   *[]RevisionConfigFile `gorm:"serializer:json;type:text" json:"config_files"`
       CreatedAt     time.Time `json:"created_at"`
       UpdatedAt     time.Time `json:"updated_at"`
   }
   ```
   `RevisionConfigFile` contains `file_path`, `target` (`app_dir` or
   `release_dir`), and `content`. `ConfigFiles == nil` inherits active managed
   files. A non-nil empty list explicitly replaces the managed file set with no
   files. Existing env-only rows remain compatible and inherit active files.

2. **`Watcher` Model** (Retained from previous step):
   ```go
   InterceptNextRelease bool   `gorm:"not null;default:false" json:"intercept_next_release"`
   PendingVersion       string `gorm:"not null;default:''" json:"pending_version"`
   ```
3. **Revert**: Remove `StagedEnvContent` and `HasStagedEnv` from the `Service` model.

---

### Phase 2: Configuration Resolution (Agent Deploy Logic)
During orchestration (`internal/agent/watcher_deployment.go`), the agent resolves the configuration for the target version before stopping or starting services.

1. **Resolution Priority**:
   When deploying `targetVersion` (e.g., `v1.2.0`):
   - **Match 1**: Check `ServiceConfigRevision` for `TargetVersion == "v1.2.0"`.
   - **Match 2**: If no exact match, check for `TargetVersion == "next"`.
   - **Match 3**: If neither exists, fall back to current active service configuration.
   - The selected revision supplies env content. Its `config_files: null` inherits
     the active managed files; an explicit array supplies the complete file set,
     including `[]` for removal. An exact env-only revision does not pick files
     from an unrelated `next` revision.
2. **Application & Cleanup**:
   - Resolve selected revisions in memory, validate artifacts and managed paths, and capture the target snapshot before stopping services. Draft preparation does not write live files or update active service rows.
   - Write the selected environment and managed file set to validated service/app-dir
     or release-dir paths during activation. On failure, restore the previous live
     files before recovery; preserve drafts and any historical snapshot for a failed redeploy.
   - After service start and health verification succeed, persist the selected contents and consume their revisions in one transaction. A draft edited during deployment remains staged. Exact-version revisions take precedence over `next`; unused drafts remain available.

---

### Phase 3: The Intercept Safety Net
We maintain the **"Require Approval for Next Release"** workflow.
- If a developer clicks "Intercept Next Release", the Watcher halts orchestration when it detects a new release, going into a `pending_approval` state.
- `POST /api/watchers/:id/approve` requires `{"version":"<pending version>"}`. Approval applies only to that version and survives retries; a changed target requires fresh approval. The one-shot interception flag clears only after successful deployment or explicit discard.
- `auto_deploy=false` requires approval for every new target. The default is automatic deployment.
- This gives the developer an indefinite window to visit the Deployment Candidates UI, ensure the configuration is correct for that specific version, and manually click "Approve & Deploy".

---

### Phase 4: API Endpoints
The revision and watcher candidate APIs share the same managed-file contract.

1. **Revisions API**:
   - `GET /api/services/:id/revisions` - List all pending config revisions for a service.
   - `PUT /api/services/:id/revisions/:target` - Create or update a config revision (e.g. `:target` = `v1.5.0` or `next`).
   - `DELETE /api/services/:id/revisions/:target` - Remove a planned revision.
2. **Watcher Candidate API**:
   - `GET /api/watchers/:id/candidate?target=<version-or-next>` returns per-service
     `active_env`, `candidate_env`, `has_candidate_env`, `active_config_files`,
     `candidate_config_files`, `has_candidate_config_files`, and `is_modified`.
     File lists contain `{file_path, target, content}` objects; candidate files
     show the resolved set, while `has_candidate_config_files` identifies a saved
     explicit override.
   - `PUT /api/watchers/:id/candidate` accepts `target_version` and `services` with
     `service_id`, `env_content`, and optional `config_files`.
   - On either candidate or revision writes, omitted or `null` `config_files`
     preserves an existing override, or leaves a new/env-only revision inheriting
     active files. `[]` explicitly replaces the set with no files. Revision GET
     returns `null` for inheritance and an array for an override.
   - Each service's paths, targets, duplicates, env collisions, and overlapping
     managed targets are validated before saving. Deployment preflight also checks
     conflicts across services before stopping them. Draft saves never modify
     active service rows or live files.
3. **Intercept API** (Already implemented):
   - `POST /api/watchers/:id/intercept`
   - `POST /api/watchers/:id/approve`

---

### Phase 5: SvelteKit Dashboard UI (Deployment Candidates)
Provide a dedicated view for managing releases and configurations safely.

1. **Deployment Candidates Page**:
   - A new tab/section in the Watcher details showing downloaded but unreleased versions (intercepted versions) alongside a button to "Pre-configure Future Version".
   - Users can type `v2.0.0-rc1` and prepare env content and managed config files in advance.
2. **"Next" Wildcard Configurator**:
   - Users can define environment and managed-file changes for the "Next deployment, whatever version it ends up being."
3. **Managed File Editor**:
   - Add, edit, and remove relative file paths, app-dir/release-dir targets, and contents.
   - Review active contents and removed files beside the candidate set. Inheritance
     is explicit; env-only saves omit the file override. Editing a file or choosing
     **Stage active files** prepares an explicit set.
   - Unsaved-state tracking, Reset, Save candidate, target request guards, and
     automatic saving during approval include files. Removing all files persists
     `[]`; reloading preserves that choice.
4. **Approval Flow**:
   - If the Watcher is in `pending_approval`, show a prominent banner with the intercepted version, a button to jump to its specific Candidate Configuration, and an "Approve & Deploy" button.


### Historical snapshots

`GET /api/services/:id/snapshots/:version/env` inspects a trusted historical snapshot using the same path and service naming rules as rollback. Historical inspection is read-only: draft saves never alter rollback snapshots. A missing snapshot is reported as missing, rather than replaced with current environment content.

Snapshots retain the complete deployed managed path set and file contents,
including an explicit empty set. Rollback replaces the running managed set with
that trusted version's set, removes files introduced by the version being left,
and updates active configuration after successful activation. Failed deployment
recovery restores the pre-attempt files, including paths removed or moved by the
candidate. Rollback validates snapshot provenance before service shutdown and uses the
existing managed-path validation when restoring files. Existing snapshot tree
layouts remain readable.

Actual Windows NSSM/IIS activation, junction behavior, and live health rollback
remain environment acceptance gates; local tests do not establish that evidence.

### Reverse proxy subpaths

Build the SPA at the root; the Go server injects `WEB_ASSETS_PATH` (or its `WEB_BASE_PATH` alias) into the bootstrap HTML at runtime. A valid `X-Forwarded-Prefix` is used when neither setting supplies a prefix. Proxies may preserve the prefix or strip it before forwarding. Prefixes use slash-separated letters, digits, dots, underscores, hyphens, or tildes; empty and dot traversal segments are rejected. API and SSE URLs and client links use the same runtime prefix. Existing browser tabs should reload after changing the prefix.
