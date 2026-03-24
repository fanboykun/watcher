<script lang="ts">
	import { api, type Watcher } from '$lib/api';
	import * as Button from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import * as Table from '$lib/components/ui/table';
	import { Clock, Pause, Play, RefreshCw, Search, AlertCircle, CheckCircle } from '@lucide/svelte';
	import { timeAgo } from '$lib/utils';
	import { onDestroy, onMount } from 'svelte';
	import { invalidate } from '$app/navigation';

	let { data } = $props();
	let watchers = $derived(data.watchers);
	let searchQuery = $state('');
	let globalError = $state('');
	let globalSuccess = $state('');

	let autoRefreshInterval: ReturnType<typeof setInterval>;

	onMount(() => {
		// Auto refresh every 5 seconds to get latest poll status
		autoRefreshInterval = setInterval(() => {
			invalidate('app:watchers');
		}, 5000);
	});

	onDestroy(() => {
		if (autoRefreshInterval) clearInterval(autoRefreshInterval);
	});

	async function togglePause(w: Watcher) {
		const newPaused = !w.paused;
		try {
			await api.updateWatcher(w.id, { paused: newPaused });
			w.paused = newPaused;
			showSuccess(`Watcher ${w.name} ${newPaused ? 'paused' : 'resumed'}`);
			invalidate('app:watchers');
		} catch (e: any) {
			showError(`Failed to toggle pause: ${e.message}`);
		}
	}

	function showSuccess(msg: string) {
		globalSuccess = msg;
		setTimeout(() => (globalSuccess = ''), 3000);
	}

	function showError(msg: string) {
		globalError = msg;
		setTimeout(() => (globalError = ''), 5000);
	}

	function getNextCheck(w: Watcher) {
		if (w.paused) return 'Paused';
		if (!w.last_checked) return 'Pending...';
		
		const last = new Date(w.last_checked).getTime();
		const next = last + (w.check_interval_sec * 1000);
		const now = Date.now();
		
		if (next <= now) return 'Imminent';
		
		const diffSec = Math.floor((next - now) / 1000);
		if (diffSec < 60) return `in ${diffSec}s`;
		return `in ${Math.floor(diffSec/60)}m ${diffSec%60}s`;
	}

	let filteredWatchers = $derived(
		watchers.filter(
			(w) =>
				w.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
				w.service_name.toLowerCase().includes(searchQuery.toLowerCase())
		)
	);
</script>

<div class="flex flex-col gap-6">
	<!-- Header -->
	<div>
		<h1 class="text-2xl font-bold tracking-tight">Polling Management</h1>
		<p class="text-sm text-muted-foreground">Manage and monitor GitHub repository polling</p>
	</div>

	<div class="flex items-center gap-2">
		<div class="relative flex-1 max-w-sm">
			<Search class="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
			<Input type="search" placeholder="Filter watchers..." class="pl-8" bind:value={searchQuery} />
		</div>
		<Button.Root variant="outline" size="icon" onclick={() => { invalidate('app:watchers'); showSuccess('Refreshed'); }}>
			<RefreshCw class="h-4 w-4" />
		</Button.Root>
	</div>

	{#if globalError}
		<div class="rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400">
			<AlertCircle class="mr-2 inline h-4 w-4" />
			{globalError}
		</div>
	{/if}

	{#if globalSuccess}
		<div class="rounded-lg border border-green-500/30 bg-green-500/10 p-4 text-sm text-green-400">
			<CheckCircle class="mr-2 inline h-4 w-4" />
			{globalSuccess}
		</div>
	{/if}

	<Card.Root>
		<Card.Header>
			<Card.Title>Polled Repositories</Card.Title>
			<Card.Description>Shows the monitoring status of all configured watchers.</Card.Description>
		</Card.Header>
		<Card.Content>
			<Table.Root>
				<Table.Header>
					<Table.Row>
						<Table.Head>Watcher</Table.Head>
						<Table.Head>Target Service</Table.Head>
						<Table.Head>Interval</Table.Head>
						<Table.Head>Last Checked</Table.Head>
						<Table.Head>Next Check</Table.Head>
						<Table.Head>Status</Table.Head>
						<Table.Head class="text-right">Actions</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#if filteredWatchers.length === 0}
						<Table.Row>
							<Table.Cell colspan={7} class="h-24 text-center">
								No watchers found.
							</Table.Cell>
						</Table.Row>
					{/if}
					{#each filteredWatchers as w (w.id)}
						<Table.Row
							class="cursor-pointer hover:bg-muted/50 transition-colors"
							onclick={() => (window.location.href = `/watchers/${w.id}?tab=polling`)}
						>
							<Table.Cell class="font-medium">{w.name}</Table.Cell>
							<Table.Cell>{w.service_name}</Table.Cell>
							<Table.Cell>
								<div class="flex items-center gap-1.5 text-muted-foreground text-sm">
									<Clock class="h-3.5 w-3.5" />
									{w.check_interval_sec}s
								</div>
							</Table.Cell>
							<Table.Cell>
								<span title={w.last_checked || ''}>
									{w.last_checked ? timeAgo(w.last_checked) : 'Never'}
								</span>
							</Table.Cell>
							<Table.Cell>
								<span class={w.paused ? 'text-muted-foreground' : 'font-medium'}>
									{getNextCheck(w)}
								</span>
							</Table.Cell>
							<Table.Cell>
								{#if w.paused}
									<span class="inline-flex items-center rounded-md bg-yellow-400/10 px-2 py-1 text-xs font-medium text-yellow-500 ring-1 ring-inset ring-yellow-400/20">
										Paused
									</span>
								{:else}
									<span class="inline-flex items-center rounded-md bg-green-500/10 px-2 py-1 text-xs font-medium text-green-500 ring-1 ring-inset ring-green-500/20">
										Active
									</span>
								{/if}
							</Table.Cell>
							<Table.Cell class="text-right">
								<div class="flex justify-end gap-2" role="group" aria-label="Actions">
									{#if w.paused}
										<Button.Root size="sm" variant="outline" onclick={() => togglePause(w)}>
											<Play class="mr-1 h-3.5 w-3.5" /> Resume
										</Button.Root>
									{:else}
										<Button.Root size="sm" variant="outline" onclick={() => togglePause(w)}>
											<Pause class="mr-1 h-3.5 w-3.5" /> Pause
										</Button.Root>
									{/if}
								</div>
							</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		</Card.Content>
	</Card.Root>
</div>
