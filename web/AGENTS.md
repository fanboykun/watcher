## Project Configuration

- **Language**: TypeScript
- **Package Manager**: bun
- **Add-ons**: prettier, eslint, tailwindcss, sveltekit-adapter, mcp

---

## Mandatory source boundaries

- No application source file under `web/src` may exceed **350 physical lines**.
  This includes routes, components, Svelte modules, TypeScript, and CSS. There is
  no per-file allowlist and no lint suppression for this limit.
- Run `bun run check:structure`. `check`, `build`, and `lint` also enforce this
  limit. Split a growing file before adding behavior; do not compress formatting
  or move the entire file to another directory to bypass the rule.
- Keep each file responsible for one concern. Routes compose feature views;
  feature controllers own query/mutation composition; form models and presentation
  helpers stay pure. Extract named sections with typed props, not arbitrary
  numbered chunks or generic catch-all utility files.
- API endpoint clients and response types are grouped by domain under `lib/api`.
  `lib/api.ts` is a compatibility facade. Shared server-state options live under
  `lib/query`; unsaved form state belongs to its feature, never the query cache.
- Use TanStack Query for server reads, polling, request errors, and mutation
  progress. Include resource IDs, filters, pagination, target versions, and exact
  candidate identity in keys. Mutations must invalidate affected caches and must
  never retry deployment/restart/update operations automatically.
- Keep SSE lifecycle cleanup explicit and write events into the query cache or
  invalidate the matching keys. Never copy query results into a second mutable
  server-data store. Editable drafts are intentional separate copies and must
  survive background refreshes.

---

You are able to use the Svelte MCP server, where you have access to comprehensive Svelte 5 and SvelteKit documentation. Here's how to use the available tools effectively:

## Available MCP Tools:

### 1. list-sections

Use this FIRST to discover all available documentation sections. Returns a structured list with titles, use_cases, and paths.
When asked about Svelte or SvelteKit topics, ALWAYS use this tool at the start of the chat to find relevant sections.

### 2. get-documentation

Retrieves full documentation content for specific sections. Accepts single or multiple sections.
After calling the list-sections tool, you MUST analyze the returned documentation sections (especially the use_cases field) and then use the get-documentation tool to fetch ALL documentation sections that are relevant for the user's task.

### 3. svelte-autofixer

Analyzes Svelte code and returns issues and suggestions.
You MUST use this tool whenever writing Svelte code before sending it to the user. Keep calling it until no issues or suggestions are returned.

### 4. playground-link

Generates a Svelte Playground link with the provided code.
After completing the code, ask the user if they want a playground link. Only call this tool after user confirmation and NEVER if code was written to files in their project.
