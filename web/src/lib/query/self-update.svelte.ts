import {
	createQuery,
	createMutation,
	useMutationState,
	useQueryClient
} from '@tanstack/svelte-query';
import { SvelteDate } from 'svelte/reactivity';
import { api } from '$lib/api';
import { queries } from './options';

const updateKey = ['self', 'apply-update'] as const;
const dismissKey = ['preferences', 'dismissed-update'] as const;
const storageKey = 'watcher.self-update.dismissed-version';

export function createSelfUpdate(enabled: () => boolean = () => true) {
	const client = useQueryClient();
	const check = createQuery(() => ({ ...queries.selfUpdate(), enabled: enabled() }));
	const dismissed = createQuery(() => ({
		queryKey: dismissKey,
		queryFn: () => localStorage.getItem(storageKey) ?? '',
		initialData:
			typeof localStorage !== 'undefined' ? (localStorage.getItem(storageKey) ?? '') : '',
		staleTime: Infinity
	}));
	const operations = useMutationState({
		filters: { mutationKey: updateKey },
		select: (mutation) => mutation.state
	});
	const mutation = createMutation(() => ({
		mutationKey: updateKey,
		mutationFn: api.selfUpdate,
		gcTime: Infinity
	}));
	const latest = $derived(operations[operations.length - 1]);
	const busy = $derived(latest?.status === 'pending' || latest?.status === 'success');
	const updateStatus = $derived(
		latest?.status === 'pending'
			? 'updating'
			: latest?.status === 'success'
				? 'applied'
				: latest?.status === 'error'
					? 'failed'
					: 'idle'
	);
	const updateMessage = $derived(
		latest?.status === 'pending'
			? 'Downloading and applying the Watcher update…'
			: latest?.status === 'success'
				? `${(latest.data as { message: string }).message}. Reload the dashboard after Watcher restarts to confirm the running version.`
				: (latest?.error?.message ?? '')
	);
	function dismiss(version: string) {
		localStorage.setItem(storageKey, version.trim());
		client.setQueryData(dismissKey, version.trim());
	}
	function clearDismissal() {
		localStorage.removeItem(storageKey);
		client.setQueryData(dismissKey, '');
	}
	async function perform() {
		// Inspect the shared cache synchronously so two mounted observers cannot
		// issue duplicate updates before Svelte flushes their reactive state.
		const previous = client.getMutationCache().findAll({ mutationKey: updateKey }).at(-1);
		if (previous?.state.status === 'pending' || previous?.state.status === 'success') return;
		try {
			await mutation.mutateAsync();
			clearDismissal();
		} catch {
			/* Shared mutation state owns the error. */
		}
	}
	return {
		check,
		dismiss,
		clearDismissal,
		perform,
		get busy() {
			return busy;
		},
		get shouldNotify() {
			return Boolean(
				check.data?.update_available && check.data.latest_version.trim() !== dismissed.data
			);
		},
		get snapshot() {
			return {
				info: check.data ?? null,
				checkedAt: check.dataUpdatedAt ? new SvelteDate(check.dataUpdatedAt).toISOString() : null,
				updateStatus,
				updateMessage
			};
		}
	};
}
