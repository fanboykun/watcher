# Catalog browser verification

These runners are opt-in for significant UI changes. Do not run them automatically
after every edit or for small changes: Chromium and builds can exhaust WSL
resources. Run one browser suite at a time, and do not overlap frontend builds
with full Go checks. Use focused checks appropriate to the change by default.

`verify-release-catalog.cjs` serves the production build with the same runtime
base rewriting as Watcher's Go handler. It mocks API responses and tests both `/`
and `/watcher` at desktop (1440 px) and mobile (390 px) widths.

From `web/`, with Playwright and a Chromium browser available:

```sh
rtk proxy bun run build
rtk proxy node scripts/verify-release-catalog.cjs
```

If Playwright is installed outside this checkout, set `PLAYWRIGHT_MODULE` to its
package path. Set `CHROMIUM_EXECUTABLE` to use an existing Chromium executable;
otherwise Playwright uses its installed browser. The runner adds no dependencies
to the application. `DEBUG_CATALOG=1` prints mock request paths.

Coverage includes older release pagination, prereleases, drafts, ZIP selection,
release notes, empty/error recovery, host download completion after leaving Releases, configuration
from Releases and Versions, approval with the exact artifact ID, reload/direct
entry, failed deployment review/discard, retained downloads, failed candidate
reload, same-version artifact replacement during save, unsaved edits across
watcher events/navigation, and active deployment controls. Managed-config coverage
includes env-only inheritance, app-dir/release-dir file staging and review,
save/reload, exact/next targets, explicit empty file sets, API path validation,
reset, and saving file edits during approval without calling live service writes. Set `CATALOG_STRESS=1`
to also overlap candidate requests and reject a delayed superseded response; this
adversarial case dispatches a DOM click for the second target while loading.

These checks prove browser behavior with deterministic API fixtures. They do not
perform GitHub downloads, run the Go API, or validate Windows/NSSM/IIS activation.

## Reported task regressions

`verify-task-fixes.cjs` uses the same production-build server, Playwright module,
and Chromium settings described above. Run from `web/`:

```sh
rtk proxy node scripts/verify-task-fixes.cjs
```

It checks root and `/watcher` deployments at desktop and mobile widths: public
host/base-path navigation from SSE and manual actions, a fresh watcher with
stalled history requests, initial errors/timeouts and retries, first-deployment
stream completion, and the update banner opening Maintenance and starting one
update. Update cases cover progress, failure/retry, accepted-restart wording,
back/forward navigation, refresh without a duplicate update, and unsaved-settings
protection. All mutations use deterministic API mocks; no running Watcher is
updated by this runner.
