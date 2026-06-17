<script lang="ts">
	import { onMount } from 'svelte';
	import {
		api,
		type Watcher
	} from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Eye, Plus, Trash2, Zap, AlertCircle } from '@lucide/svelte';
	import { resolve } from '$app/paths';
	import { statusColor, timeAgo } from '$lib/utils';

	let watchers = $state<Watcher[]>([]);
	let error = $state('');
	let triggerMsg = $state('');
	let showDeleteDialog = $state(false);
	let deleting = $state(false);
	let deleteWatcherID = $state<number | null>(null);
	let deleteWatcherName = $state('');

	onMount(load);

	async function load() {
		try {
			watchers = await api.listWatchers();
			error = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load watchers';
		}
	}

	function openDeleteWatcherDialog(id: number, name: string) {
		deleteWatcherID = id;
		deleteWatcherName = name;
		showDeleteDialog = true;
	}

	async function confirmDeleteWatcher() {
		if (!deleteWatcherID) return;
		deleting = true;
		try {
			await api.deleteWatcher(deleteWatcherID);
			await load();
			showDeleteDialog = false;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Delete failed';
		} finally {
			deleting = false;
		}
	}

	async function triggerCheck(id: number) {
		try {
			const res = await api.triggerCheck(id);
			triggerMsg = res.message;
			setTimeout(() => (triggerMsg = ''), 3000);
		} catch (e) {
			triggerMsg = e instanceof Error ? e.message : 'Trigger failed';
		}
	}


</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold tracking-tight">Watchers</h1>
			<p class="text-sm text-muted-foreground">Repository poll loops</p>
		</div>
		<a href={resolve('/watchers/new')}>
			<Button.Root>
				<Plus class="mr-2 h-4 w-4" /> Add Watcher
			</Button.Root>
		</a>
	</div>

	{#if error}
		<div class="rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400 flex items-center">
			<AlertCircle class="mr-2 h-4 w-4 shrink-0" />
			<span>{error}</span>
		</div>
	{/if}

	{#if triggerMsg}
		<div class="rounded-lg border border-blue-500/30 bg-blue-500/10 p-4 text-sm text-blue-400 flex items-center">
			<Zap class="mr-2 h-4 w-4 shrink-0" />
			<span>{triggerMsg}</span>
		</div>
	{/if}

	{#if watchers.length > 0}
		<Card.Root class="border-border bg-card">
			<Table.Root>
				<Table.Header>
					<Table.Row class="border-border hover:bg-transparent">
						<Table.Head>Name</Table.Head>
						<Table.Head>Status</Table.Head>
						<Table.Head>Version</Table.Head>
						<Table.Head>Last Checked</Table.Head>
						<Table.Head>Services</Table.Head>
						<Table.Head class="text-right">Actions</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each watchers as w (w.id)}
						<Table.Row class="border-border">
							<Table.Cell>
								<a href={resolve(`/watchers/${w.id}`)} class="font-medium hover:underline">
									{w.name}
								</a>
								<p class="font-mono text-xs text-muted-foreground">{w.service_name}</p>
							</Table.Cell>
							<Table.Cell>
								<span
									class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium capitalize {statusColor(
										w.status
									)}"
								>
									{w.status}
								</span>
							</Table.Cell>
							<Table.Cell>
								<span class="font-mono text-sm">{w.current_version || '—'}</span>
							</Table.Cell>
							<Table.Cell class="text-muted-foreground">
								{timeAgo(w.last_checked)}
							</Table.Cell>
							<Table.Cell class="text-muted-foreground">
								{w.services ? w.services.length : 0}
							</Table.Cell>
							<Table.Cell class="text-right">
								<div class="flex items-center justify-end gap-1">
									<Button.Root
										variant="ghost"
										size="icon"
										class="h-8 w-8"
										onclick={() => triggerCheck(w.id)}
										title="Trigger check"
									>
										<Zap class="h-4 w-4" />
									</Button.Root>
									<Button.Root
										variant="ghost"
										size="icon"
										class="h-8 w-8 text-red-400 hover:text-red-300"
										onclick={() => openDeleteWatcherDialog(w.id, w.name)}
										title="Delete"
									>
										<Trash2 class="h-4 w-4" />
									</Button.Root>
								</div>
							</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		</Card.Root>
	{:else if !error}
		<Card.Root class="border-dashed border-border bg-card">
			<Card.Content class="flex flex-col items-center justify-center py-16 text-center">
				<Eye class="mb-3 h-10 w-10 text-muted-foreground/40" />
				<h3 class="text-sm font-medium text-muted-foreground">No watchers yet</h3>
				<p class="mt-1 text-xs text-muted-foreground/60">Click "Add Watcher" to get started</p>
			</Card.Content>
		</Card.Root>
	{/if}
</div>

<!-- Delete Confirmation Dialog -->
<Dialog.Root bind:open={showDeleteDialog}>
	<Dialog.Content class="sm:max-w-[420px]">
		<Dialog.Header>
			<Dialog.Title>Delete Watcher</Dialog.Title>
			<Dialog.Description>
				This will delete watcher <span class="font-medium">{deleteWatcherName}</span> and all linked services.
			</Dialog.Description>
		</Dialog.Header>
		<Dialog.Footer>
			<Button.Root variant="outline" type="button" onclick={() => (showDeleteDialog = false)} disabled={deleting}>
				Cancel
			</Button.Root>
			<Button.Root type="button" class="bg-red-600 text-white hover:bg-red-700" onclick={confirmDeleteWatcher} disabled={deleting}>
				{deleting ? 'Deleting...' : 'Delete'}
			</Button.Root>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
