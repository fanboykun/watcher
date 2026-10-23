import type { Watcher } from '$lib/api';

export type WatcherFilter = 'all' | 'attention' | 'paused';

export function needsAttention(watcher: Watcher): boolean {
	return (
		watcher.status === 'failed' ||
		watcher.status === 'pending_approval' ||
		watcher.last_poll_status === 'error' ||
		Boolean(watcher.last_error)
	);
}

export function matchesWatcher(watcher: Watcher, query: string, filter: WatcherFilter): boolean {
	const matchesText = [
		watcher.name,
		watcher.service_name,
		watcher.metadata_url,
		watcher.current_version
	]
		.join(' ')
		.toLowerCase()
		.includes(query.trim().toLowerCase());
	return (
		matchesText &&
		(filter === 'all' || (filter === 'paused' ? watcher.paused : needsAttention(watcher)))
	);
}
