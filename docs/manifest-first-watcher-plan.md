# Manifest-First Watcher Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make watcher creation prefer `version.json` manifests and generate one targeted service config from manifest deployment hints.

**Architecture:** Extend `ServiceMeta` with optional deployment fields, add manifest-aware inspect responses, and update the Svelte create wizard to preserve manifest URLs and select one deployable target. Runtime deployment remains unchanged because it already deploys the selected manifest service artifact.

**Tech Stack:** Go, Gin, Svelte 5, TypeScript.

---

### Task 1: Backend Manifest Inspect

**Files:**
- Modify: `internal/agent/github.go`
- Modify: `internal/api/handlers.go`
- Test: `internal/agent/github_test.go`

- [ ] Extend `ServiceMeta` with optional deployment hint fields.
- [ ] Add an inspect response that reports `source`, `repo_url`, `metadata_url`, and per-service target details.
- [ ] Add a helper that explicitly fetches `version.json` from a repo release ref before falling back to repo asset discovery.
- [ ] Add tests for manifest inspect and deploy hint JSON decoding.

### Task 2: Frontend Manifest Wizard

**Files:**
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/routes/watchers/+page.svelte`

- [ ] Add frontend types for manifest deploy targets.
- [ ] Send `release_ref` during inspect.
- [ ] Stop stripping `metadata_url` when the user enters a manifest URL.
- [ ] Show manifest targets when available and let the user select one.
- [ ] Generate exactly one service draft from the selected manifest target.

### Task 3: Verification

**Files:**
- Validate: backend and frontend checks.

- [ ] Run focused Go tests for GitHub manifest handling.
- [ ] Run Svelte autofixer on the changed Svelte page.
- [ ] Run the project frontend check command.
