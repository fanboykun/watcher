<script lang="ts">
	import PageHeading from '$lib/components/console/page-heading.svelte';
	import ListToolbar from '$lib/components/console/list-toolbar.svelte';
	import EmptyState from '$lib/components/console/empty-state.svelte';
	import WatcherCard from '$lib/features/watchers/watcher-card.svelte';
	import {
		matchesWatcher,
		needsAttention,
		type WatcherFilter
	} from '$lib/features/watchers/presentation';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import WatcherPollingStatus from '$lib/components/watcher-polling-status.svelte';
	import RequestError from '$lib/components/request-error.svelte';
	import ErrorScreen from '$lib/components/errors/error-screen.svelte';
	import { goto } from '$app/navigation';
	import { createQuery, createMutation, useQueryClient } from '@tanstack/svelte-query';
	import { queries, keys } from '$lib/query/options';
	import { forget, invalidate, message } from '$lib/query/client';
	import { api, type Watcher } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Eye, Plus, Trash2, Zap, ChevronDown, ExternalLink } from '@lucide/svelte';
	import { resolve } from '$app/paths';
	import RequestLoading from '$lib/components/request-loading.svelte';

	const client = useQueryClient();
	const list = createQuery(() => ({ ...queries.watchers(), refetchInterval: 5000 }));
	const watchers = $derived(list.data ?? []);
	const deleteMutation = createMutation(() => ({
		mutationFn: api.deleteWatcher,
		onSuccess: () => invalidate(client, keys.watchers, keys.services, keys.status)
	}));
	const checkMutation = createMutation(() => ({
		mutationFn: api.triggerCheck,
		onSuccess: () => invalidate(client, keys.watchers)
	}));
	let query = $state('');
	let filter = $state<WatcherFilter>('all');
	const checkingID = $derived(checkMutation.isPending ? (checkMutation.variables ?? null) : null);
	const filteredWatchers = $derived(watchers.filter((w) => matchesWatcher(w, query, filter)));
	const attentionCount = $derived(watchers.filter(needsAttention).length);
	// Without any rows a failed read blocks the page; with rows it is only a stale warning.
	const blocking = $derived(list.isError && !list.data ? list.error : null);
	const error = $derived(message((blocking ? null : list.error) || deleteMutation.error));
	const loading = $derived(list.isPending);
	let triggerMsg = $state('');
	let triggerError = $state('');
	let showDeleteDialog = $state(false);
	const deleting = $derived(deleteMutation.isPending);
	let deleteWatcherID = $state<number | null>(null);
	let deleteWatcherName = $state('');
	async function load() {
		await list.refetch();
	}

	function openDeleteWatcherDialog(id: number, name: string) {
		deleteWatcherID = id;
		deleteWatcherName = name;
		showDeleteDialog = true;
	}

	async function confirmDeleteWatcher() {
		if (!deleteWatcherID || deleting) return;
		try {
			await deleteMutation.mutateAsync(deleteWatcherID);
			forget(client, keys.watcher(deleteWatcherID));
			showDeleteDialog = false;
		} catch {
			/* Mutation error remains visible. */
		}
	}

	async function triggerCheck(id: number) {
		if (checkingID !== null) return;
		triggerError = '';
		try {
			const res = await checkMutation.mutateAsync(id);
			triggerMsg = res.message;
			setTimeout(() => (triggerMsg = ''), 3000);
		} catch (e) {
			triggerError = e instanceof Error ? e.message : 'Trigger failed';
		}
	}
</script>

{#snippet watcherActions(w: Watcher)}
	<DropdownMenu.Root>
		<DropdownMenu.Trigger
			class={Button.buttonVariants({ variant: 'outline', size: 'sm' })}
			aria-label={`Actions for ${w.name}`}
			disabled={checkingID !== null}>Actions<ChevronDown class="size-3.5" /></DropdownMenu.Trigger
		>
		<DropdownMenu.Content
			align="end"
			onCloseAutoFocus={(event) => {
				if (showDeleteDialog) event.preventDefault();
			}}
		>
			<DropdownMenu.Item
				onSelect={() => {
					void goto(resolve('/watchers/[id]', { id: String(w.id) }));
				}}><ExternalLink />Open watcher</DropdownMenu.Item
			>
			<DropdownMenu.Item
				disabled={checkingID !== null || w.paused}
				onSelect={() => {
					void triggerCheck(w.id);
				}}><Zap />Poll now</DropdownMenu.Item
			>
			<DropdownMenu.Item
				variant="destructive"
				class="mt-1 border-t border-border pt-2"
				onSelect={() => openDeleteWatcherDialog(w.id, w.name)}
				><Trash2 />Delete watcher</DropdownMenu.Item
			>
		</DropdownMenu.Content>
	</DropdownMenu.Root>
{/snippet}

<div class="space-y-6">
	<PageHeading
		eyebrow="Workspace / Repositories"
		title="Watchers"
		description="Your repositories on watch. Manage polling, review releases, and control delivery."
	>
		{#snippet actions()}<Button.Root href={resolve('/watchers/new')} size="sm"
				><Plus class="size-4" />Add watcher</Button.Root
			>{/snippet}
	</PageHeading>
	{#if !loading && watchers.length}
		<div class="flex flex-wrap gap-x-6 gap-y-2 text-xs text-muted-foreground">
			<span><strong class="font-mono text-foreground">{watchers.length}</strong> repositories</span
			><span><strong class="font-mono text-primary">{attentionCount}</strong> need attention</span
			><span
				><strong class="font-mono text-foreground">{watchers.filter((w) => w.paused).length}</strong
				> paused</span
			><span class="ml-auto font-mono text-[10px]">REFRESHES EVERY 5S</span>
		</div>
		<ListToolbar
			bind:value={query}
			label="Search repositories…"
			count={filteredWatchers.length}
			total={watchers.length}
		>
			<select class="filter-select" aria-label="Filter watchers by state" bind:value={filter}
				><option value="all">All watchers</option><option value="attention">Needs attention</option
				><option value="paused">Paused</option></select
			>
		</ListToolbar>
	{/if}

	<RequestError message={error} onRetry={() => load()} />

	{#if triggerMsg}
		<div
			role="status"
			class="flex items-center rounded-lg border border-blue-500/30 bg-blue-500/10 p-4 text-sm text-blue-400"
		>
			<Zap class="mr-2 h-4 w-4 shrink-0" />
			<span>{triggerMsg}</span>
		</div>
	{/if}

	<RequestError message={triggerError} />

	{#if loading}
		<RequestLoading label="Loading watchers…" />
	{:else if blocking}
		<ErrorScreen error={blocking} onRetry={load} />
	{:else if filteredWatchers.length > 0}
		<div class="grid gap-4 lg:hidden">
			{#each filteredWatchers as watcher (watcher.id)}<div>
					<WatcherCard {watcher} />
					<div class="mt-2 flex justify-end">{@render watcherActions(watcher)}</div>
				</div>{/each}
		</div>
		<Card.Root class="hidden border-border bg-card lg:block">
			<Table.Root>
				<Table.Header>
					<Table.Row class="border-border hover:bg-transparent">
						<Table.Head>Name</Table.Head>
						<Table.Head>Polling</Table.Head>
						<Table.Head>Version</Table.Head>
						<Table.Head>Latest poll</Table.Head>
						<Table.Head>Services</Table.Head>
						<Table.Head class="text-right">Actions</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each filteredWatchers as w (w.id)}
						<Table.Row class="border-border">
							<Table.Cell>
								<a href={resolve(`/watchers/${w.id}`)} class="font-medium hover:underline">
									{w.name}
								</a>
								<p class="font-mono text-xs text-muted-foreground">{w.service_name}</p>
							</Table.Cell>
							<Table.Cell>
								<WatcherPollingStatus watcher={w} kind="activity" />
							</Table.Cell>
							<Table.Cell>
								<span class="font-mono text-sm">{w.current_version || '—'}</span>
							</Table.Cell>
							<Table.Cell class="text-muted-foreground">
								<WatcherPollingStatus watcher={w} kind="result" />
							</Table.Cell>
							<Table.Cell class="text-muted-foreground">
								{w.services ? w.services.length : 0}
							</Table.Cell>
							<Table.Cell class="text-right">
								<div class="flex justify-end">{@render watcherActions(w)}</div>
							</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		</Card.Root>
	{:else if !error}
		{#if watchers.length}
			<EmptyState
				title="No matching watchers"
				description="Try another repository name or clear your filters."
				icon={Eye}
				>{#snippet actions()}<Button.Root
						variant="outline"
						size="sm"
						onclick={() => {
							query = '';
							filter = 'all';
						}}>Clear filters</Button.Root
					>{/snippet}</EmptyState
			>
		{:else}
			<EmptyState
				title="Put your first repository on watch."
				description="Connect a GitHub repository to poll for new releases and deploy your services."
				icon={Eye}
				>{#snippet actions()}<Button.Root href={resolve('/watchers/new')} size="sm"
						><Plus class="size-4" />Add watcher</Button.Root
					>{/snippet}</EmptyState
			>
		{/if}
	{/if}
</div>

<!-- Delete Confirmation Dialog -->
<Dialog.Root
	bind:open={showDeleteDialog}
	onOpenChange={(open) => {
		if (deleting && !open) showDeleteDialog = true;
	}}
>
	<Dialog.Content class="sm:max-w-[420px]" showCloseButton={!deleting}>
		<Dialog.Header>
			<Dialog.Title>Delete Watcher</Dialog.Title>
			<Dialog.Description>
				This will delete watcher <span class="font-medium">{deleteWatcherName}</span> and all linked services.
			</Dialog.Description>
		</Dialog.Header>
		{#if error}<p role="alert" class="text-sm text-red-400">{error}</p>{/if}
		<Dialog.Footer>
			<Button.Root
				variant="outline"
				type="button"
				onclick={() => (showDeleteDialog = false)}
				disabled={deleting}
			>
				Cancel
			</Button.Root>
			<Button.Root
				type="button"
				class="bg-red-600 text-white hover:bg-red-700"
				loading={deleting}
				onclick={confirmDeleteWatcher}
				disabled={deleting}
			>
				{deleting ? 'Deleting...' : 'Delete'}
			</Button.Root>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
