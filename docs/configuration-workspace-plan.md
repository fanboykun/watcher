# Configuration and deployment UX implementation

1. Share a file workspace across active service files, candidate configuration, and service forms. Keep raw text authoritative; bundle a real editor with line numbers, search, history, syntax highlighting, diagnostics, and an exact text review.
2. Keep the active configuration and pending release context visible. Preserve the backend distinction between inherited managed files and an explicitly empty file set. Changes to a candidate never call active service endpoints.
3. Preserve in-memory drafts across tabs and service actions. Track their baseline separately from refreshed server metadata. Confirm internal navigation, browser unload, reset, file removal, and applying active configuration with a restart.
4. Review candidates in three stages: target release, service/file changes, and deployment confirmation. Bind approval to the exact loaded version and catalog selection, including when saving refreshes the watcher.
5. Keep the setup wizard for new services; show existing service settings in grouped sections with one save action and the same configuration workspace.
6. Replace duplicate deployment tables with a shared attempt timeline. Link parent/root attempts using backend IDs, fetch only missing related attempts, retain pagination, and distinguish failed deploys from successful recovery.

Verification: focused editor/model tests, Svelte checks, scoped lint/formatting, production build, and browser checks at desktop/mobile under root and reverse-proxy paths. Exercise actual editor input, undo, comments/quoting, failed saves, unsaved navigation, active apply/restart, candidate isolation, changed artifact rejection, service edits, and related attempts. Windows/NSSM execution remains a separate integration check.
