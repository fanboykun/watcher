# Watcher Tasks

Tasks 1–6 are implemented locally. Checked items indicate source completion;
local verification and remaining acceptance work are recorded below. Managed
pool relocation remains a separate planned feature.
Updated: 2026-10-23.

The per-watcher GitHub release catalog has a separate
[continuation and handoff plan](release-catalog-plan.md), including its current
implementation, remaining acceptance work, and uncommitted changes.

## 1. Preserve the public host and base path when opening deployment logs

- [x] Fix the automatic navigation to deployment logs when a user connected to a watcher's SSE stream receives a new deployment event.

**Reported behavior:** Navigation uses `{API_BASE_URL}/{log_pathname}`, which can
send the browser to the wrong host or omit the dashboard's custom base path.
For example, a dashboard accessible at `https://sasad.socfindo.co.id/watcher`
must keep deployment-log navigation under that public URL.

**Fix:** The SSE handler previously refreshed details without opening the new
deployment. Manual deployment actions also followed an API-generated absolute
`log_url`. Both paths now build a dashboard-relative route from the watcher and
deployment IDs using the runtime base path. SSE validates the announced IDs;
manual redeploy and rollback ignore the API host.

**Local verification:** The completed task-regression browser run covered root
and `/watcher` routes at desktop/mobile widths, including SSE navigation to the
announced deployment and manual redeploy with an internal-host `log_url`.

**Acceptance criteria:**

- The deployment-log destination preserves the public dashboard host and configured base path, including `/watcher` in the example above.
- The destination opens the logs for the deployment announced by SSE.
- Navigation works for both root-hosted dashboards and reverse-proxy subpaths, without missing or duplicated prefixes.

## 2. Make the new-version banner perform the update action

- [x] Fix the new-version banner so its update action starts the Watcher self-update flow and opens the relevant settings tab.

**Reported behavior:** Clicking the banner only navigates to the settings page;
it neither performs the update nor selects the correct tab.

**Fix:** The banner now opens Settings → Maintenance and starts the shared
self-update action. One request can run at a time across client navigation.
Progress and retryable failures remain visible, and the action URL is consumed
before starting so refresh/back navigation cannot replay it. An accepted response
shows that the update was applied and restart requested; reloading confirms the
running version. Unsaved settings block the action with an explanation.

**Local verification:** The completed task-regression browser run checked the
Maintenance tab, one update request, progress, failure/retry, accepted-restart
wording, back/forward navigation, refresh without replay, and unsaved settings.
The API was mocked; actual Windows self-update/restart remains unverified.

**Acceptance criteria:**

- The banner's update action initiates the self-update flow instead of stopping at navigation.
- Any navigation to settings selects the tab containing Watcher's version and update controls and preserves the dashboard base path.
- Update progress, completion, and failure are visible; repeated clicks do not start duplicate updates.

## 3. Resolve infinite loading for a newly created watcher's first deployment

- [x] Investigate and fix the watcher detail page remaining in a loading state after fresh watcher creation and during its first deployment.

**Reported behavior:** Opening the detail page for a newly created watcher can
leave the frontend loading indefinitely. The report is specific to fresh
watchers and their first deployment.

**Fix:** Initial rendering previously waited for deployment history and other
secondary requests alongside the watcher. The primary watcher request now renders
the page independently, has a 30-second timeout, and offers a working retry.
History failures remain visible without hiding the page. Empty history/current
version states render immediately, and the first deployment opens its live log
stream and receives completion without a manual refresh.

**Local verification:** The completed task-regression browser run held history
requests while the fresh watcher rendered, checked primary/history errors and
timeout retries, and streamed first-deployment progress through completion.

**Acceptance criteria:**

- Creating a watcher and immediately opening its detail page renders the page without requiring a previous successful deployment.
- Empty deployment history and an unset current version render usable empty or pending states.
- The first deployment's progress and final status appear without a manual refresh, including when the page was opened before deployment started.
- Request failures display an actionable error instead of leaving the page loading indefinitely.

## 4. Support all managed config files in deployment candidates

- [x] Extend candidate configuration beyond the env file to include other managed config files.

**Reported behavior:** Candidate configuration currently manages only the env
file. Users also need to prepare changes to other managed config, including
app-dir config files and release-dir config files.

**Fix:** Candidate revisions now include an optional complete `config_files`
set with relative `file_path`, `target` (`app_dir` or `release_dir`), and `content`.
Omitted/null files preserve an existing override or inherit active files for a
new env-only revision; `[]` explicitly removes the managed set. Exact-version
revisions win before `next`, including when the exact revision inherits files.
The editor supports staging, review, reset, saving, and approval with these files.

Draft saves leave active rows and live files untouched. Activation validates
paths/collisions, replaces the selected live managed set, and commits active
configuration only after health success. Recovery restores pre-attempt files;
rollback restores the complete trusted snapshot path inventory and active rows,
including added, removed, and renamed files. Concurrently edited drafts survive
consumption, and app-dir file identities are compared case-insensitively for
Windows.

**Local verification:** API/agent regressions cover inheritance and explicit
removal, save isolation, unsafe/conflicting paths, config-only concurrent edits,
successful activation, start/health failure recovery, same-version failure,
snapshot inventory, rollback, and case-only renames. The expanded managed-config
browser matrix is not fully verified; its latest run was stopped after WSL
resource problems.

**Acceptance criteria:**

- Users can stage and review managed config file contents and target paths alongside the env file for an exact-version or `next` candidate.
- Saving a candidate keeps its changes isolated from active service configuration and live files until deployment activation.
- Deployment applies the candidate's managed config files to their declared app-dir or release-dir targets using the existing path-validation rules.
- Config snapshots include the deployed managed config; failed deployment recovery and rollback restore the appropriate previous files.
- Existing env-only candidates remain supported.

## 5. Prevent unsafe managed watcher directory edits

- [x] Reject installation-directory edits that would detach a pool from its releases and runtime paths.

**Confirmed problem:** Edit settings updated only `watchers.install_dir`, then
restarted the worker. It did not move cached ZIPs, extracted releases, snapshots,
or service/IIS paths. Catalog artifacts retain their original install directory,
so downloads disappear from the new directory's list or fail configuration
validation. A deployment could also be cancelled while still using the old root.

**Fix:** An existing pool's install directory is read-only. API updates containing
a different `install_dir` return HTTP 409 before persisting any settings or
requesting worker sync. This applies even to empty or paused pools, avoiding a
race with startup, queued work, or compensation still completing on the old root.
Clients can omit the field or send its unchanged value when saving other settings.

**Local verification:** Focused API tests passed for empty pools, downloads in
progress/completed, pending approval, deployment, rollback, and paused pools.
Rejected edits preserve pool state, artifact ownership, and existing files, do
not create the destination, and do not partially save other submitted settings.

- [ ] Implement [managed pool relocation](watcher-relocation-plan.md), including
      recovery for pools whose directory was changed by older builds. Blocking
      unsafe edits does not move or repair an already affected pool.

## 6. Consolidate candidate editing and show complete configuration snapshots

- [x] Remove the duplicate env-only service candidate editor.
- [x] Display captured app-dir and release-dir files alongside env in snapshot history.

**Confirmed gap:** Managed-file candidates existed in the watcher editor, while
the service page still used an older env-only editor. The version snapshot page
also requested only each current service's env snapshot. Storage already captured
managed app-dir/release-dir files, but the dashboard did not expose them.

**Fix:** Service candidate navigation opens the single watcher editor and focuses
the selected service. Saved targets and per-service draft deletion remain in that
editor. The new watcher snapshot API and viewer read every historical service
tree and show file targets, paths, and contents, including services removed from
the current configuration. The env-only API remains compatible. Missing history
is never replaced with active files.

**Verification:** Focused API/agent tests cover all captured file categories,
empty/env-only snapshots, historical paths and removed services, encoded tags and
the reverse-proxy prefix, missing snapshots, symlink rejection, read-only
inspection, saved-target ownership, and exact-versus-next draft identity. Static
Svelte compilation and autofixer checks passed for the changed components. No
browser or full application build ran for this follow-up. The repository linter
hit the 256 MB Node heap cap; it was not retried with more memory.

## Verification record and remaining acceptance

During implementation, the full Go suite, focused race checks, Go vet, Windows
amd64 cross-build, frontend check/build, and the tasks 1–3 browser regression
matrix passed. The frontend check reported zero errors and warnings. On the
latest resumed session, Go vet and the frontend check/build passed again. The
latest full Go rerun was interrupted and is not additional passing evidence.

Remaining manual acceptance:

- [ ] Complete the managed-config browser matrix when explicitly requested and
      sufficient resources are available.
- [ ] Validate actual Windows NSSM/IIS activation, health recovery, rollback,
      junction behavior, and self-update/restart.
- [ ] Validate representative public/private GitHub access for the release catalog.

Browser checks are opt-in for significant UI work, not a default step after small
changes. Run one suite at a time and keep frontend builds separate from full Go
checks. See [browser verification guidance](../web/scripts/README.md).
