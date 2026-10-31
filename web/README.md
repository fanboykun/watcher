# Watcher dashboard

Svelte 5 + SvelteKit static SPA, embedded in the Go agent.

```sh
bun install
bun run dev
bun run check
bun run check:structure
bun test src scripts/check-structure.test.ts
bun run build
```

The development server proxies `/api` to the Go agent at `localhost:8080`.
The production build is emitted to `build/` for Go embedding. Fonts are bundled
locally so Windows installations do not need access to a font service.

See [frontend structure and design](../docs/frontend-design.md) for component
boundaries, design tokens, and verification guidance. Import the backend client
from `$lib/api`; it re-exports response types and authentication helpers for
existing consumers. Domain UI lives under `src/lib/features`, shared console
components under `src/lib/components/console`, and navigation under
`src/lib/components/shell`.

TanStack Query owns server-state fetching, polling, mutation progress, and cache
invalidation. Feature controllers keep editable drafts separate from query data.
All application source files have an enforced maximum of 350 physical lines;
`check`, `build`, and `lint` reject oversized files.
