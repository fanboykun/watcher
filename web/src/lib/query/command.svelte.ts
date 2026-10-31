import { createMutation, useQueryClient, type QueryKey } from '@tanstack/svelte-query';
import { invalidate } from './client';

/** A named operational command; confirmations and navigation stay at the call site. */
export function createCommand<T = void>(affected: () => QueryKey[] = () => []) {
	const client = useQueryClient();
	return createMutation(() => ({
		mutationFn: ({ run }: { name: string; run: () => Promise<T> }) => run(),
		onSuccess: () => invalidate(client, ...affected())
	}));
}
