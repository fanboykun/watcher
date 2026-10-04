<script lang="ts">
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { Watcher } from '$lib/api';
	import { onMount } from 'svelte';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import { Textarea } from '$lib/components/ui/textarea';
	import {
		ArrowLeft,
		Archive,
		AlertCircle,
		Save,
		Check,
		RefreshCw,
		Copy,
		ExternalLink
	} from '@lucide/svelte';
	import RequestLoading from '$lib/components/request-loading.svelte';
	import { resolve } from '$app/paths';

	interface ServiceEnvItem {
		serviceId: number;
		serviceName: string;
		activeEnv: string;
		content: string;
		originalContent: string;
		saving: boolean;
		saved: boolean;
		saveError: string;
		error?: string;
	}

	const watcherId = Number(page.params.id);
	const version = page.params.version;

	let watcher = $state<Watcher | null>(null);
	let envs = $state<ServiceEnvItem[]>([]);
	let loading = $state(true);
	let error = $state('');

	onMount(async () => {
		try {
			watcher = await api.getWatcher(watcherId);
			if (watcher && watcher.services.length > 0) {
				const results = await Promise.all(
					watcher.services.map(async (svc) => {
						const serviceName =
							svc.windows_service_name ||
							svc.iis_site_name ||
							svc.iis_app_pool ||
							`Service ${svc.id}`;
						try {
							const res = await api.getServiceSnapshotEnv(svc.id, version as string);
							return {
								serviceId: svc.id,
								serviceName,
								activeEnv: svc.env_content || '',
								content: res.env_content,
								originalContent: res.env_content,
								saving: false,
								saved: false,
								saveError: ''
							};
						} catch {
							return {
								serviceId: svc.id,
								serviceName,
								activeEnv: svc.env_content || '',
								content: svc.env_content || '',
								originalContent: svc.env_content || '',
								saving: false,
								saved: false,
								saveError: '',
								error: 'No snapshot found on disk yet. You can create one by saving below.'
							};
						}
					})
				);
				envs = results;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load version snapshot';
		} finally {
			loading = false;
		}
	});

	async function saveEnv(item: ServiceEnvItem) {
		item.saving = true;
		item.saveError = '';
		item.saved = false;
		try {
			await api.updateServiceSnapshotEnv(item.serviceId, version as string, item.content);
			item.originalContent = item.content;
			item.error = undefined;
			item.saved = true;
			setTimeout(() => {
				item.saved = false;
			}, 3000);
		} catch (e) {
			item.saveError = e instanceof Error ? e.message : 'Failed to save configuration';
		} finally {
			item.saving = false;
		}
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center gap-4">
		<a href={resolve(`/watchers/[id]?tab=versions`, { id: String(watcherId) })}>
			<Button.Root variant="ghost" size="icon" class="h-8 w-8">
				<ArrowLeft class="h-4 w-4" />
			</Button.Root>
		</a>
		<div class="flex-1">
			<div class="flex items-center gap-2">
				<h1 class="text-2xl font-bold tracking-tight">Version Configuration:</h1>
				<span class="rounded bg-primary/10 px-2 py-0.5 font-mono text-xl font-semibold text-primary"
					>{version}</span
				>
			</div>
			{#if watcher}
				<p class="font-mono text-sm text-muted-foreground">{watcher.name}</p>
			{/if}
		</div>
	</div>

	{#if error}
		<div
			class="flex items-center rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400"
		>
			<AlertCircle class="mr-2 h-4 w-4 shrink-0" />
			<span>{error}</span>
		</div>
	{/if}

	{#if loading}
		<RequestLoading label="Loading configuration snapshot..." />
	{:else if watcher}
		<div class="grid gap-6">
			{#if envs.length === 0}
				<Card.Root class="border-dashed border-border bg-card">
					<Card.Content class="flex flex-col items-center justify-center py-12 text-center">
						<Archive class="mb-3 h-8 w-8 text-muted-foreground/40" />
						<p class="text-sm text-muted-foreground">No services found for this watcher.</p>
					</Card.Content>
				</Card.Root>
			{/if}

			{#each envs as env (env.serviceId)}
				<Card.Root class="border-border bg-card">
					<Card.Header class="pb-3">
						<div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
							<div>
								<Card.Title class="flex items-center gap-2 text-lg">
									<Archive class="h-4 w-4 text-blue-400" />
									{env.serviceName}
								</Card.Title>
								<Card.Description class="text-xs">
									Configuration used when deploying or rolling back to {version}.
								</Card.Description>
							</div>

							<div class="flex flex-wrap items-center gap-2">
								<a href={resolve(`/services/[id]?tab=candidates`, { id: String(env.serviceId) })}>
									<Button.Root variant="ghost" size="sm" class="h-8 text-xs text-muted-foreground">
										<ExternalLink class="mr-1 h-3.5 w-3.5" /> View Service
									</Button.Root>
								</a>
								{#if env.activeEnv && env.content !== env.activeEnv}
									<Button.Root
										variant="ghost"
										size="sm"
										class="h-8 text-xs text-muted-foreground"
										onclick={() => (env.content = env.activeEnv)}
										title="Replace with currently running active environment"
									>
										<Copy class="mr-1 h-3.5 w-3.5" /> Copy Active
									</Button.Root>
								{/if}
								{#if env.content !== env.originalContent}
									<Button.Root
										variant="ghost"
										size="sm"
										class="h-8 text-xs text-muted-foreground"
										onclick={() => (env.content = env.originalContent)}
										title="Reset to snapshot state on disk"
									>
										<RefreshCw class="mr-1 h-3.5 w-3.5" /> Reset
									</Button.Root>
								{/if}
								<Button.Root
									variant="default"
									size="sm"
									class="h-8"
									onclick={() => saveEnv(env)}
									disabled={env.saving}
								>
									{#if env.saving}
										<RefreshCw class="mr-1.5 h-3.5 w-3.5 animate-spin" /> Saving...
									{:else if env.saved}
										<Check class="mr-1.5 h-3.5 w-3.5 text-emerald-400" /> Saved!
									{:else}
										<Save class="mr-1.5 h-3.5 w-3.5" /> Save Changes
									{/if}
								</Button.Root>
							</div>
						</div>
					</Card.Header>
					<Card.Content class="space-y-2">
						{#if env.error}
							<div
								class="rounded-md border border-amber-500/20 bg-amber-500/10 p-2.5 text-xs text-amber-300"
							>
								{env.error}
							</div>
						{/if}
						{#if env.saveError}
							<div
								class="rounded-md border border-red-500/30 bg-red-500/10 p-2.5 text-xs text-red-400"
							>
								{env.saveError}
							</div>
						{/if}
						<Textarea
							bind:value={env.content}
							class="min-h-[260px] font-mono text-sm text-blue-300"
							placeholder="KEY=VALUE"
						/>
					</Card.Content>
				</Card.Root>
			{/each}
		</div>
	{/if}
</div>
