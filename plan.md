# Plan: Per-Watcher GitHub Deployment Environment

## Context

Watcher currently sends GitHub Deployment API requests using a single global `ENVIRONMENT` value from agent config (`.env`).

This is insufficient when:
- one watcher deploys to `staging` while another deploys to `production`
- different repositories enforce different protected environment rules
- teams need explicit per-service/per-repo deployment targeting

We need to make deployment environment selection watcher-specific while preserving backward compatibility.

---

## Goals

1. Add **per-watcher deployment environment** configuration.
2. Keep existing installs working with no immediate breaking changes.
3. Make deployment environment resolution deterministic and transparent in logs.
4. Improve failure diagnostics for GitHub environment-related errors.
5. Expose configuration in API + dashboard watcher forms.

---

## Non-Goals

- No multi-environment promotion workflow orchestration.
- No GitHub environment discovery API integration in this phase.
- No full deploy policy engine (approvals, hold windows, etc.).

---

## Current State Summary

- Global `ENVIRONMENT` is loaded via `internal/config/config.go`.
- GitHub deployment creation uses `r.global.Environment` in `internal/agent/watcher.go`.
- Watcher DB model has no dedicated deployment environment field.
- Watcher create/update DTOs and UI currently have no environment field.

---

## Target State

Deployment environment resolved per watcher using precedence:

1. `watcher.deployment_environment` (if non-empty)
2. global `ENVIRONMENT` from `.env`

Behavioral rules:
- If GitHub Deployment integration is enabled and resolved environment is empty -> skip deployment status integration and log explicit warning.
- All deploy logs include resolved environment value during deployment API calls.
- GitHub API errors include actionable hints tied to environment and ref issues.

---

## Data Model Changes

### `internal/database/models.go`

Add column to `Watcher`:
- `DeploymentEnvironment string `gorm:"not null;default:''" json:"deployment_environment"``

Backward compatibility:
- Default empty string.
- Existing rows continue to work via global fallback.

Migration:
- GORM `AutoMigrate` will add the column automatically.

---

## API Contract Changes

### DTO updates (`internal/api/dto.go`)

- `CreateWatcherRequest` add:
  - `DeploymentEnvironment string `json:"deployment_environment"``
- `UpdateWatcherRequest` add:
  - `DeploymentEnvironment *string `json:"deployment_environment"``

### Handler updates (`internal/api/handlers.go`)

- `CreateWatcher` writes `deployment_environment`.
- `UpdateWatcher` supports partial update for `deployment_environment`.
- `ListWatchers/GetWatcher` already return model JSON, so new field should flow automatically.

Validation (lightweight for phase 1):
- trim spaces before saving.
- allow empty (fallback behavior).

---

## Agent Runtime Changes

### `internal/agent/watcher.go`

Add helper:
- `resolveDeploymentEnvironment(watcherEnv, globalEnv string) string`

Usage:
- On deployment, compute resolved environment once.
- Pass resolved environment into `CreateDeployment`.

Behavior:
- If resolved env is empty and GH deployment integration otherwise enabled:
  - append deploy log warning
  - disable GH deployment status updates for that deployment

Observability:
- append deploy logs:
  - `github_deployment: environment=<resolved>`
  - if empty: reason and remediation hint

---

## Frontend Changes

### API types (`web/src/lib/api.ts`)

- `Watcher` interface add `deployment_environment`.
- Create/update payload paths accept the field.

### Watcher creation/edit UI

Files likely impacted:
- `web/src/routes/watchers/+page.svelte`
- `web/src/routes/watchers/[id]/+page.svelte`

Changes:
- Add input field:
  - label: `Deployment Environment (GitHub)`
  - placeholder: `production`
  - helper text: `Optional. Falls back to global ENVIRONMENT if empty.`
- Include value in create/update requests.

### Optional visibility

- Show resolved or configured environment in watcher detail overview card.

---

## Error Handling & Diagnostics

Enhance logs and surfaced errors to clearly identify environment issues:

- On `CreateDeployment` failure (404/422), include:
  - owner/repo
  - ref
  - environment
- Add hint text:
  - create environment in GitHub repo settings
  - ensure token has deployments permission
  - verify ref/tag exists

Deploy log examples:
- `github_deployment: using environment=staging`
- `github_deployment: disabled because deployment environment is empty (watcher + global ENVIRONMENT both empty)`

---

## Backward Compatibility Strategy

- Existing watchers with no `deployment_environment` will continue using global `ENVIRONMENT`.
- No forced migration/backfill required.
- Existing API clients remain valid because new field is additive and optional.

---

## Task Breakdown

### Phase 1: Backend model + API

1. Add `deployment_environment` to `Watcher` model.
2. Extend watcher DTOs.
3. Wire create/update handler logic.
4. Verify JSON responses include field.

Deliverable:
- backend supports storing and returning per-watcher environment.

### Phase 2: Agent deployment behavior

1. Implement environment resolution helper.
2. Replace direct use of global `ENVIRONMENT`.
3. Add logs for resolved environment.
4. Add safe-disable logic when unresolved empty.

Deliverable:
- per-watcher environment controls GH deployment API environment.

### Phase 3: Frontend watcher forms

1. Add field in Add Watcher flow.
2. Add field in Edit Watcher flow.
3. Include in API payloads.
4. Display in watcher detail summary.

Deliverable:
- users can manage environment per watcher from dashboard.

### Phase 4: Tests

1. Unit test environment resolution helper.
2. Extend handler tests (or add minimal tests) for create/update environment field mapping.
3. Smoke check compile + existing targeted tests.

Deliverable:
- confidence on regression and behavior.

### Phase 5: Documentation

1. Update README and AGENTS docs for per-watcher environment precedence.
2. Add migration/usage notes and examples.

Deliverable:
- docs align with behavior.

---

## Suggested Implementation Order (Execution Priority)

1. Model + DTO + handlers.
2. Agent resolution logic + logging.
3. Frontend forms.
4. Tests.
5. Docs.

Rationale:
- Keeps backend contract stable before UI wiring.
- Ensures runtime behavior exists before exposing controls.

---

## Acceptance Criteria

1. New watchers can be created with `deployment_environment`.
2. Existing watchers can be updated with `deployment_environment`.
3. For watcher with env set, GH deployment uses watcher env.
4. For watcher env empty and global env set, GH deployment uses global env.
5. If both empty, GH deployment integration is skipped with explicit log reason.
6. UI supports editing this value in watcher create/edit screens.
7. No regression to deploy flow, rollback flow, or non-GH deployment behavior.

---

## Manual Verification Checklist

1. Set global `ENVIRONMENT=production`.
2. Watcher A set `deployment_environment=staging`.
3. Watcher B leave empty.
4. Trigger deployments for both.
5. Verify in GitHub Deployments:
   - A uses `staging`
   - B uses `production`
6. Clear global env and watcher env; trigger deploy:
   - deployment should proceed
   - GH deployment status integration should be skipped with clear log message

---

## Risks and Mitigations

### Risk: Misconfigured environment names
- Mitigation: explicit log hints and UI helper text.

### Risk: Hidden fallback confusion
- Mitigation: always log resolved environment and source (watcher/global).

### Risk: API/UI drift
- Mitigation: keep field names identical across model/DTO/UI (`deployment_environment`).

### Risk: Token permission failures mistaken for env issues
- Mitigation: preserve and improve 401/403/404/422 hinting in GitHub client errors.

---

## Rollout Notes

- Safe to ship as additive change.
- No downtime migration required.
- Recommend release note entry:
  - "Per-watcher GitHub deployment environment with global fallback"

---

## Optional Follow-Up Enhancements (Not in this phase)

1. Environment picker with known values from repo metadata.
2. Per-watcher toggle: enable/disable GitHub deployment status reporting.
3. Validation endpoint: test GitHub deployment API credentials + environment before save.
4. Audit log entry when deployment environment changes.

