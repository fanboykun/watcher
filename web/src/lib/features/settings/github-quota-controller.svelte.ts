import { createQuery } from '@tanstack/svelte-query';
import { queries } from '$lib/query/options';

// Read-only view of the agent's shared GitHub quota; refreshed by the query's interval.
export function createGitHubQuota() {
	const query = createQuery(() => queries.githubQuota());
	return {
		get entries() {
			return query.data ?? [];
		},
		get loading() {
			return query.isPending;
		},
		get error() {
			return query.isError ? query.error.message : '';
		}
	};
}
