# Plan: Deployment Candidates & Version-Bound Configuration

## The Problem
Treating pending configurations as a single floating "draft" is insufficient for robust release management. Developers need the ability to prepare configurations specifically tailored to a particular upcoming version (e.g., `v2.0.0-rc1`) long before it even drops. Furthermore, each downloaded version should be treated as an isolated "Deployment Candidate" with its own configuration lifecycle, ensuring that updates are completely deterministic and safe from human error.

## The Solution: Version-Bound Configurations
We are introducing a **Deployment Candidate Configuration** architecture. Configurations are no longer just global or floating drafts; they can be bound to specific version tags.

---

### Phase 1: Database Schema Modifications
We will revert the single `StagedEnvContent` fields and introduce a dedicated table for configuration revisions.

1. **`ServiceConfigRevision` Model**:
   A new table to store configurations for specific targets.
   ```go
   type ServiceConfigRevision struct {
       ID            uint      `gorm:"primaryKey" json:"id"`
       ServiceID     uint      `gorm:"not null;index" json:"service_id"`
       TargetVersion string    `gorm:"not null;index" json:"target_version"` // e.g., "1.2.0-rc1" or "next"
       EnvContent    string    `gorm:"type:text" json:"env_content"`
       CreatedAt     time.Time `json:"created_at"`
       UpdatedAt     time.Time `json:"updated_at"`
   }
   ```
2. **`Watcher` Model** (Retained from previous step):
   ```go
   InterceptNextRelease bool   `gorm:"not null;default:false" json:"intercept_next_release"`
   PendingVersion       string `gorm:"not null;default:''" json:"pending_version"`
   ```
3. **Revert**: Remove `StagedEnvContent` and `HasStagedEnv` from the `Service` model.

---

### Phase 2: Configuration Resolution (Agent Deploy Logic)
During orchestration (`internal/agent/watcher_deployment.go`), the agent will resolve the correct configuration for the target version before starting the services.

1. **Resolution Priority**:
   When deploying `targetVersion` (e.g., `v1.2.0`):
   - **Match 1**: Check `ServiceConfigRevision` for `TargetVersion == "v1.2.0"`.
   - **Match 2**: If no exact match, check for `TargetVersion == "next"`.
   - **Match 3**: If neither exists, fallback to the current active `Service.EnvContent`.
2. **Application & Cleanup**:
   - If a revision is selected, update the live `Service.EnvContent` with the revision's content and write the `.env` file to disk immediately prior to taking the active configuration snapshot.
   - If the `next` wildcard revision was consumed, it can be deleted or renamed to the locked version to prevent it from accidentally applying to subsequent releases.

---

### Phase 3: The Intercept Safety Net
We maintain the **"Require Approval for Next Release"** workflow.
- If a developer clicks "Intercept Next Release", the Watcher halts orchestration when it downloads a new release, going into a `pending_approval` state.
- This gives the developer an indefinite window to visit the Deployment Candidates UI, ensure the configuration is correct for that specific version, and manually click "Approve & Deploy".

---

### Phase 4: API Endpoints
We need endpoints to manage the new revisions.

1. **Revisions API**:
   - `GET /api/services/:id/revisions` - List all pending config revisions for a service.
   - `PUT /api/services/:id/revisions/:target` - Create or update a config revision (e.g. `:target` = `v1.5.0` or `next`).
   - `DELETE /api/services/:id/revisions/:target` - Remove a planned revision.
2. **Intercept API** (Already implemented):
   - `POST /api/watchers/:id/intercept`
   - `POST /api/watchers/:id/approve`

---

### Phase 5: SvelteKit Dashboard UI (Deployment Candidates)
Provide a dedicated view for managing releases and configurations safely.

1. **Deployment Candidates Page**:
   - A new tab/section in the Watcher details showing downloaded but unreleased versions (intercepted versions) alongside a button to "Pre-configure Future Version".
   - Users can type `v2.0.0-rc1` and prepare its environment variables in advance.
2. **"Next" Wildcard Configurator**:
   - Users can define environment changes for the "Next deployment, whatever version it ends up being."
3. **Approval Flow**:
   - If the Watcher is in `pending_approval`, show a prominent banner with the intercepted version, a button to jump to its specific Candidate Configuration, and an "Approve & Deploy" button.
