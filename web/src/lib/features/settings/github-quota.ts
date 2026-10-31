import type { GitHubQuota } from '$lib/api';

export type QuotaTone = 'ok' | 'warn' | 'unknown';

export interface QuotaView {
	tone: QuotaTone;
	label: string;
	detail: string;
}

function clock(ts: string): string {
	return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

export function quotaSourceLabel(quota: GitHubQuota): string {
	switch (quota.source) {
		case 'global':
			return 'Global token';
		case 'anonymous':
			return 'No token (shared per IP)';
		default:
			return 'Watcher token override';
	}
}

export function quotaUsage(quota: GitHubQuota): string {
	const count = quota.watcher_ids.length;
	if (count === 0) return 'Used by self-update and repository inspection';
	return `Used by ${count} watcher${count === 1 ? '' : 's'}`;
}

export function quotaView(quota: GitHubQuota): QuotaView {
	if (quota.blocked_until) {
		return {
			tone: 'warn',
			label: 'Rate limited',
			detail: `GitHub requests resume at ${clock(quota.blocked_until)}.`
		};
	}
	if (quota.limit === 0) {
		return {
			tone: 'unknown',
			label: 'Not reported yet',
			detail: 'Shown after the first GitHub API request with this token.'
		};
	}
	const left = `${quota.remaining.toLocaleString()} of ${quota.limit.toLocaleString()} left`;
	const reset = quota.reset_at ? `resets at ${clock(quota.reset_at)}` : 'full quota available';
	if (quota.remaining <= quota.reserve) {
		return {
			tone: 'warn',
			label: left,
			detail: `Polling waits for the reset so deployments keep the rest; ${reset}.`
		};
	}
	return { tone: 'ok', label: left, detail: reset.charAt(0).toUpperCase() + reset.slice(1) + '.' };
}
