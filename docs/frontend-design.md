# Watcher frontend

The interface is a deployment control room: graphite surfaces, amber controls,
Manrope typography, and IBM Plex Mono for operational data. Status colors retain
their meaning: green for healthy, blue for activity, amber for review, red for failure.
Fonts are bundled for offline installations.

## Structure

- `src/lib/api/`: transport, authentication, event streams, and backend types.
- `src/lib/api.ts`: compatible public client and endpoint methods.
- `src/lib/components/shell/`: responsive navigation and application chrome.
- `src/lib/components/console/`: shared page headers, metrics, filters, and empty states.
- `src/lib/features/`: watcher/service tabs, form editors, and presentation rules.
- `src/routes/`: route composition, data loading, and operational actions.
- `src/lib/styles/`: design tokens and shared console presentation.
- `src/routes/layout.css`: Tailwind integration and base element constraints.
- `src/lib/components/ui/`: accessible low-level primitives.

Keep API response state separate from search/filter state. Use derived values for
filtered collections. Operational actions retain pending states, server errors,
and confirmation dialogs. A filtered list has a different empty state from a new
installation. Navigation must use SvelteKit's base-aware path helpers.

The dashboard prioritizes pending releases and errors, followed by the deployment
fleet. It displays observed state without inventing monitoring data. Update notices
belong to the application shell so they appear once on every route.

## Configuration and deployment workflows

`features/configuration/` owns raw configuration validation and the shared
CodeMirror file workspace. Active service files, candidate drafts, service
settings, and watcher setup use this workspace. Text remains authoritative:
comments, quotes, duplicate environment keys, and existing line endings are
preserved. Formatting JSON is an explicit action. Syntax diagnostics are advisory;
unsafe or duplicate file destinations block saving.

Keep the saved draft baseline separate from active configuration. Candidate files
distinguish inheritance from an explicitly empty set. Saving candidates only uses
candidate endpoints. Deployment review captures the loaded version and catalog ID
and retains them through saving and approval. Active application asks for explicit
confirmation; app-directory files apply immediately while release-directory files
wait for deployment. Configuration drafts survive tabs and metadata refreshes,
and navigation guards prevent accidental loss.

New services retain guided setup. Existing service settings show identity, runtime,
and configuration together and stay open after saving. `features/deployments/`
owns the shared attempt timeline for watcher and service detail views. Parent and
root IDs link recovery to the original failure, including attempts outside the
current page; successful recovery does not hide the failed deployment.

## Verification

Run `bun run check`, `bun run build`, and targeted ESLint/Prettier checks from `web`.
Check populated, empty, failed-request, and filtered views in a browser at desktop
and mobile widths. Test menus, confirmations, keyboard navigation, route refresh,
and authentication. Browser API fixtures validate frontend behavior; live Windows
deployment and NSSM checks remain separate integration gates.

## Server state and file boundaries

TanStack Query v6 owns server data, request progress, errors, polling, and mutations.
The root layout provides one QueryClient behind the authentication gate. Logout
cancels queries and clears the cache. `lib/query/` contains typed query options,
keys, client defaults, operational commands, and shared self-update state. See
[the migration architecture](query-state-migration.md) for cache contracts.

Feature controllers compose queries and mutations. Their `ui` objects contain
only local form, filter, confirmation, and notification state. Views use the
controller's reactive getters and named sections; they do not maintain duplicate
server-data caches. Draft models deliberately copy server configuration and are
protected from background refetches. API endpoint clients and response types are
split by domain; `lib/api.ts` remains the public compatibility facade.

Every application source file under `web/src` has a hard **350 physical line**
limit, including routes, TypeScript, components, and CSS. `check`, `build`, and
`lint` run `check:structure` first. There is no per-file allowlist. Split growing
files by responsibility and keep normal formatting; do not relocate or compress
oversized files to evade the check. This rule is also in `web/AGENTS.md`.
