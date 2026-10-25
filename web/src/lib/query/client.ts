import { QueryClient, type QueryKey } from '@tanstack/query-core';

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

export function message(error: unknown): string {
	return error instanceof Error ? error.message : '';
}
