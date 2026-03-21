<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type Watcher } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Eye, Plus, Trash2, Zap, Clock, AlertCircle } from '@lucide/svelte';
	import { resolve } from '$app/paths';

	let watchers = $state<Watcher[]>([]);
	let error = $state('');
	let triggerMsg = $state('');
	let showCreate = $state(false);
	let creating = $state(false);

	// Create form fields
	let formName = $state('');
	let formServiceName = $state('');
	let formMetadataURL = $state('');
	let formInstallDir = $state('');
	let formInterval = $state(60);
	let formHcEnabled = $state(false);
	let formHcURL = $state('');

	onMount(load);

	async function load() {
		try {
			watchers = await api.listWatchers();
			error = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load watchers';
		}
	}

	async function createWatcher() {
		creating = true;
		try {
			await api.createWatcher({
				name: formName,
				service_name: formServiceName,
				metadata_url: formMetadataURL,
				install_dir: formInstallDir,
				check_interval_sec: formInterval,
				hc_enabled: formHcEnabled,
				hc_url: formHcURL
			});
			showCreate = false;
			resetForm();
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to create watcher';
		} finally {
			creating = false;
		}
	}

	function resetForm() {
		formName = '';
		formServiceName = '';
		formMetadataURL = '';
		formInstallDir = '';
		formInterval = 60;
		formHcEnabled = false;
		formHcURL = '';
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

	async function deleteWatcher(id: number, name: string) {
		if (!confirm(`Delete watcher "${name}" and all its services?`)) return;
		try {
			await api.deleteWatcher(id);
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Delete failed';
		}
	}

	function statusColor(s: string) {
		switch (s) {
			case 'healthy':
				return 'bg-emerald-500/15 text-emerald-400 border-emerald-500/30';
			case 'deploying':
				return 'bg-blue-500/15 text-blue-400 border-blue-500/30';
			case 'failed':
				return 'bg-red-500/15 text-red-400 border-red-500/30';
			case 'rollback':
				return 'bg-amber-500/15 text-amber-400 border-amber-500/30';
			default:
				return 'bg-muted text-muted-foreground border-border';
		}
	}

	function timeAgo(ts: string | null): string {
		if (!ts) return '—';
		const diff = Date.now() - new Date(ts).getTime();
		const mins = Math.floor(diff / 60000);
		if (mins < 1) return 'just now';
		if (mins < 60) return `${mins}m ago`;
		const hrs = Math.floor(mins / 60);
		if (hrs < 24) return `${hrs}h ago`;
		return `${Math.floor(hrs / 24)}d ago`;
	}
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold tracking-tight">Watchers</h1>
			<p class="text-sm text-muted-foreground">Repository poll loops</p>
		</div>
		<Button.Root onclick={() => (showCreate = true)}>
			<Plus class="mr-2 h-4 w-4" /> Add Watcher
		</Button.Root>
	</div>

	{#if error}
		<div class="rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400">
			<AlertCircle class="mr-2 inline h-4 w-4" />
			{error}
		</div>
	{/if}

	{#if triggerMsg}
		<div class="rounded-lg border border-blue-500/30 bg-blue-500/10 p-4 text-sm text-blue-400">
			<Zap class="mr-2 inline h-4 w-4" />
			{triggerMsg}
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
								<a href={resolve(`/watchers/${w.id}`)} class="font-medium hover:underline"
									>{w.name}</a
								>
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
								{w.services.length}
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
										onclick={() => deleteWatcher(w.id, w.name)}
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

<!-- Create Watcher Dialog -->
<Dialog.Root bind:open={showCreate}>
	<Dialog.Content class="sm:max-w-[500px]">
		<Dialog.Header>
			<Dialog.Title>Add Watcher</Dialog.Title>
			<Dialog.Description>Configure a new repository watcher</Dialog.Description>
		</Dialog.Header>
		<form
			class="space-y-4"
			onsubmit={(e) => {
				e.preventDefault();
				createWatcher();
			}}
		>
			<div class="grid gap-4 sm:grid-cols-2">
				<div class="space-y-2">
					<Label for="name">Name</Label>
					<Input id="name" placeholder="my-app" bind:value={formName} required />
				</div>
				<div class="space-y-2">
					<Label for="serviceName">Service Name</Label>
					<Input id="serviceName" placeholder="my-app" bind:value={formServiceName} required />
				</div>
			</div>

			<div class="space-y-2">
				<Label for="metadataURL">Metadata URL</Label>
				<Input
					id="metadataURL"
					placeholder="https://github.com/org/repo/releases/latest/download/version.json"
					bind:value={formMetadataURL}
					required
				/>
			</div>

			<div class="space-y-2">
				<Label for="installDir">Install Directory</Label>
				<Input id="installDir" placeholder="C:\apps\my-app" bind:value={formInstallDir} required />
			</div>

			<div class="grid gap-4 sm:grid-cols-2">
				<div class="space-y-2">
					<Label for="interval">Check Interval (seconds)</Label>
					<Input id="interval" type="number" min="10" bind:value={formInterval} />
				</div>
				<div class="space-y-2">
					<Label for="hcURL">Health Check URL</Label>
					<Input id="hcURL" placeholder="http://localhost:3000/health" bind:value={formHcURL} />
				</div>
			</div>

			<div class="flex items-center gap-2">
				<input
					type="checkbox"
					id="hcEnabled"
					bind:checked={formHcEnabled}
					class="rounded border-border"
				/>
				<Label for="hcEnabled">Enable health checks</Label>
			</div>

			<Dialog.Footer>
				<Button.Root variant="outline" type="button" onclick={() => (showCreate = false)}>
					Cancel
				</Button.Root>
				<Button.Root type="submit" disabled={creating}>
					{creating ? 'Creating...' : 'Create Watcher'}
				</Button.Root>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
