import type { FeedAuthType, Watcher } from '$lib/api';

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

/** GitHub-only features (release catalog, deployment reporting) apply to these watchers. */
export function isGitHubSource(sourceType: string | undefined): boolean {
	return !sourceType || sourceType === 'github';
}

export function sourceTypeLabel(sourceType: string | undefined): string {
	if (sourceType === 'fs') return 'File share feed';
	if (sourceType === 's3') return 'S3 / MinIO feed';
	return isGitHubSource(sourceType) ? 'GitHub' : 'Internal release feed';
}

export function sourceLocationLabel(sourceType: string | undefined): string {
	if (sourceType === 'fs') return 'Feed path';
	if (sourceType === 's3') return 'Bucket URL';
	if (sourceType === 'http') return 'Feed URL';
	return 'GitHub repository or metadata URL';
}

export function sourceLocationPlaceholder(sourceType: string | undefined): string {
	if (sourceType === 'fs') return '\\\\fileserver\\releases\\my-app';
	if (sourceType === 's3') return 'https://minio.internal:9000/releases/my-app';
	if (sourceType === 'http') return 'https://releases.internal/apps/my-app';
	return 'https://github.com/org/repo';
}

/** Feeds read over HTTP(S) can carry a credential; s3 always signs with SigV4. */
export function takesFeedCredentials(sourceType: string | undefined): boolean {
	return sourceType === 'http' || sourceType === 's3';
}

/** The credential type a source sends: s3 always uses sigv4. */
export function feedAuthTypeFor(
	sourceType: string | undefined,
	draftType: FeedAuthType
): FeedAuthType {
	if (sourceType === 's3') return 'sigv4';
	return draftType === 'sigv4' ? '' : draftType;
}
