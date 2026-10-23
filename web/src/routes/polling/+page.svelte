<script lang="ts">
	import { onMount } from 'svelte';
	import { invalidate } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { api, type Watcher } from '$lib/api';
	import * as Button from '$lib/components/ui/button';
	import * as Table from '$lib/components/ui/table';
	import PageHeading from '$lib/components/console/page-heading.svelte';
	import ListToolbar from '$lib/components/console/list-toolbar.svelte';
	import EmptyState from '$lib/components/console/empty-state.svelte';
	import WatcherPollingStatus from '$lib/components/watcher-polling-status.svelte';
	import RequestError from '$lib/components/request-error.svelte';
	import { Clock, Pause, Play, RefreshCw, Radio } from '@lucide/svelte';
	let { data } = $props();
	const watchers = $derived(data.watchers);
	let query = $state('');
	let filter = $state('all');
	let error = $state('');
	let success = $state('');
	let pendingID = $state<number | null>(null);
	let refreshing = $state(false);
	let successTimer: ReturnType<typeof setTimeout> | undefined;
	const filtered = $derived(
		watchers.filter(
			(w) =>
				[w.name, w.service_name].join(' ').toLowerCase().includes(query.trim().toLowerCase()) &&
				(filter === 'all' || (filter === 'paused' ? w.paused : !w.paused))
		)
	);
	async function refresh() {
		if (refreshing) return;
		refreshing = true;
		try {
			await invalidate('app:watchers');
		} finally {
			refreshing = false;
		}
	}
	onMount(() => {
		const timer = setInterval(() => {
			if (document.visibilityState === 'visible') void refresh();
		}, 5000);
		return () => {
			clearInterval(timer);
			clearTimeout(successTimer);
		};
	});
	async function togglePause(watcher: Watcher) {
		if (pendingID !== null) return;
		pendingID = watcher.id;
		error = '';
		success = '';
		try {
			await api.updateWatcher(watcher.id, { paused: !watcher.paused });
			await refresh();
			success = `${watcher.name}: polling ${watcher.paused ? 'resumed' : 'paused'}.`;
			clearTimeout(successTimer);
			successTimer = setTimeout(() => (success = ''), 4000);
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not change polling state';
		} finally {
			pendingID = null;
		}
	}
</script>

<div class="space-y-6">
	<PageHeading
		eyebrow="Observability / Repositories"
		title="Polling"
		description="Keep an eye on what's next. Control when repositories are checked for new releases."
	>
		{#snippet actions()}<Button.Root
				variant="outline"
				size="sm"
				onclick={refresh}
				disabled={refreshing}
				><RefreshCw class={['size-3.5', refreshing && 'animate-spin']} />Refresh</Button.Root
			>{/snippet}
	</PageHeading>
	<div class="flex flex-wrap gap-5 text-xs text-muted-foreground">
		<span
			><strong class="font-mono text-foreground">{watchers.filter((w) => !w.paused).length}</strong> polling
			enabled</span
		><span
			><strong class="font-mono text-primary">{watchers.filter((w) => w.paused).length}</strong> paused</span
		><span class="ml-auto font-mono text-[10px]">REFRESHES EVERY 5S</span>
	</div>
	<RequestError message={data.loadError} onRetry={refresh} />
	<RequestError message={error} />
	{#if success}<p
			role="status"
			class="rounded-md border border-emerald-500/30 bg-emerald-500/10 p-3 text-sm text-emerald-400"
		>
			{success}
		</p>{/if}
	{#if watchers.length}
		<ListToolbar
			bind:value={query}
			label="Search polling repositories…"
			count={filtered.length}
			total={watchers.length}
			><select class="filter-select" aria-label="Filter polling state" bind:value={filter}
				><option value="all">All states</option><option value="active">Polling enabled</option
				><option value="paused">Paused</option></select
			></ListToolbar
		>
	{/if}
	{#if filtered.length}
		<div class="console-panel">
			<Table.Root
				><Table.Header
					><Table.Row
						><Table.Head>Repository</Table.Head><Table.Head>Interval</Table.Head><Table.Head
							>Polling</Table.Head
						><Table.Head>Latest poll</Table.Head><Table.Head class="text-right">Control</Table.Head
						></Table.Row
					></Table.Header
				><Table.Body>
					{#each filtered as watcher (watcher.id)}
						<Table.Row
							><Table.Cell
								><a
									class="font-semibold hover:text-primary"
									href={resolve('/watchers/[id]?tab=polling', { id: String(watcher.id) })}
									>{watcher.name}</a
								>
								<p class="mt-1 font-mono text-[11px] text-muted-foreground">
									{watcher.service_name}
								</p></Table.Cell
							><Table.Cell
								><span
									class="inline-flex items-center gap-1.5 font-mono text-xs whitespace-nowrap text-muted-foreground"
									><Clock class="size-3.5" />{watcher.check_interval_sec}s</span
								></Table.Cell
							><Table.Cell><WatcherPollingStatus {watcher} kind="activity" /></Table.Cell
							><Table.Cell><WatcherPollingStatus {watcher} kind="result" /></Table.Cell><Table.Cell
								class="text-right"
								><Button.Root
									variant="outline"
									size="sm"
									disabled={pendingID !== null}
									loading={pendingID === watcher.id}
									onclick={() => togglePause(watcher)}
									>{#if watcher.paused}<Play class="size-3.5" />Resume{:else}<Pause
											class="size-3.5"
										/>Pause{/if}</Button.Root
								></Table.Cell
							></Table.Row
						>
					{/each}
				</Table.Body></Table.Root
			>
		</div>
	{:else if !data.loadError}
		<EmptyState
			title={watchers.length ? 'No matching repositories' : 'Nothing on watch. Yet.'}
			description={watchers.length
				? 'Try a different search or clear your filters.'
				: 'Add a watcher to start checking GitHub for new releases.'}
			icon={Radio}
			>{#snippet actions()}{#if watchers.length}<Button.Root
						variant="outline"
						size="sm"
						onclick={() => {
							query = '';
							filter = 'all';
						}}>Clear filters</Button.Root
					>{:else}<Button.Root size="sm" href={resolve('/watchers/new')}>Add watcher</Button.Root
					>{/if}{/snippet}</EmptyState
		>
	{/if}
</div>
