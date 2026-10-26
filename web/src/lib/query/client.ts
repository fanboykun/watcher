import { QueryClient, partialMatchKey, type QueryKey } from '@tanstack/query-core';
import { keys } from './keys';

export function createQueryClient() {
	return new QueryClient({
		defaultOptions: {
			queries: { retry: false, staleTime: 0, refetchOnWindowFocus: false },
			mutations: { retry: false }
		}
	});
}

export function invalidate(client: QueryClient, ...keys: QueryKey[]) {
	return Promise.all(keys.map((queryKey) => client.invalidateQueries({ queryKey })));
}

/** Drops cached data for resources that no longer exist so history navigation cannot show them. */
export function forget(client: QueryClient, ...keys: QueryKey[]) {
	keys.forEach((queryKey) => client.removeQueries({ queryKey }));
}

/** Everything a watcher change can alter: its list row, its own tree, and fleet summaries. */
export function watcherScope(id: number): QueryKey[] {
	return [keys.watchers, keys.watcher(id), keys.services, keys.status];
}

/** A service change also alters its owning watcher's detail and history. */
export function serviceScope(id: number, watcherId: number): QueryKey[] {
	return [keys.service(id), ...watcherScope(watcherId)];
}

/**
 * Placeholder for paged and filtered reads: keeps the previous rows on screen while another
 * page or size loads. A different resource, filter or log source never inherits them.
 */
export function keepWithin(prefix: QueryKey) {
	return <T>(previous: T | undefined, query?: { queryKey: QueryKey }) =>
		query && partialMatchKey(query.queryKey, prefix) ? previous : undefined;
}

export function message(error: unknown): string {
	return error instanceof Error ? error.message : '';
}
