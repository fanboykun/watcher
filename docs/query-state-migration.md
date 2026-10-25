# Frontend server state architecture

The application uses `@tanstack/svelte-query` v6 with Svelte 5 accessors. One QueryClient lives in
the root layout; authentication gates all private observers. Clear and cancel
private queries on logout. Keep the existing API transport and its error messages.

## Ownership

- `lib/query/options.ts`: typed query options and hierarchical keys for auth,
  system, watchers, services, candidates, releases, snapshots, logs, and self management.
  Include every server-side filter, page, target version, and resource ID in keys.
- `lib/query/client.ts`: request defaults and targeted invalidation. Reads are
  cancellable and bounded by a timeout. Operational mutations never retry automatically.
- Components derive data, loading, refresh, and error state from query observers.
  Use mutations for writes and user-triggered operations. Shared action helpers
  may coordinate confirmations and navigation, but TanStack owns request state.
- Local state remains for navigation, filters, confirmation dialogs, transient
  messages, and unsaved forms. Copy configuration into a separate editable draft;
  query cache updates must never overwrite a dirty draft.

## Implementation

1. Add provider, query keys/options, cancellation, mutation and invalidation helpers.
2. Migrate authentication, dashboard, lists, polling, and creation/edit forms.
3. Migrate watcher/service details, histories, release catalog, candidates,
   snapshots, related attempts, and log views.
4. Replace self-update's custom request/cache state with shared queries and
   mutation-cache state. Preserve update progress across client navigation and
   prevent duplicate accepted updates.
5. SSE events invalidate or update the relevant query cache. Stream lifecycle
   and log chunks remain event-driven; streams must close on disposal.

## Contracts

Watchers render before secondary history resolves. Candidate keys include the
exact selection identity and version; editing and saving candidate files never
write active configuration. Approval retains its captured artifact ID through
saving. Version snapshots cannot inherit data from another version. Mutations
refresh affected lists/details without resetting local drafts. Polling uses query
refetch intervals and stops with observer disposal. Log streams retain bounded
buffers and base-aware navigation.

## Verification

Run focused query-cache tests, Svelte checks, lint/formatting, and a production
build. Run the configuration, release-catalog, redesign, and task-regression
browser suites serially at root and `/watcher`, desktop and mobile. Add cache
coverage for deduplication, cancellation, invalidation, filter/resource changes,
and logout. The redesign browser suite also verifies switching service and watcher
edit routes through client navigation without retaining the previous resource.

`bun run check:structure` enforces 350 physical lines per application source file
with no per-file exceptions. It runs before checks, linting, and production builds.
Source-size tests include the 350/351 boundary, nested files, and CRLF input.
