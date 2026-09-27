<script lang="ts">
	import { page } from '$app/stores';
	import { api } from '$lib/api';
	import type { Watcher } from '$lib/api';
	import { onMount } from 'svelte';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import { Textarea } from '$lib/components/ui/textarea';
	import { ArrowLeft, Archive, AlertCircle } from '@lucide/svelte';
	import RequestLoading from '$lib/components/request-loading.svelte';
	import { resolve } from '$app/paths';

	let watcherId = Number($page.params.id);
	let version = $page.params.version;

	let watcher = $state<Watcher | null>(null);
	let envs = $state<{ serviceName: string; content: string; error?: string }[]>([]);
	let loading = $state(true);
	let error = $state('');

	onMount(async () => {
		try {
			watcher = await api.getWatcher(watcherId);
			if (watcher && watcher.services.length > 0) {
				const results = await Promise.all(
					watcher.services.map(async (svc) => {
						try {
							const res = await api.getServiceSnapshotEnv(svc.id, version as string);
							return {
								serviceName:
									svc.windows_service_name ||
									svc.iis_site_name ||
									svc.iis_app_pool ||
									`Service ${svc.id}`,
								content: res.env_content
							};
						} catch (err) {
							return {
								serviceName:
									svc.windows_service_name ||
									svc.iis_site_name ||
									svc.iis_app_pool ||
									`Service ${svc.id}`,
								content: '',
								error: 'No environment snapshot found for this service.'
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
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center gap-4">
		<a href={resolve(`/watchers/${watcherId}?tab=versions`)}>
			<Button.Root variant="ghost" size="icon" class="h-8 w-8">
				<ArrowLeft class="h-4 w-4" />
			</Button.Root>
		</a>
		<div class="flex-1">
			<h1 class="text-2xl font-bold tracking-tight">Version Configuration: {version}</h1>
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
						<p class="text-sm text-muted-foreground">
							No configuration snapshot is available for this version.
						</p>
					</Card.Content>
				</Card.Root>
			{/if}

			{#each envs as env (env.serviceName)}
				<Card.Root class="border-border bg-card">
					<Card.Header class="pb-3">
						<Card.Title class="flex items-center gap-2 text-lg">
							<Archive class="h-4 w-4 text-blue-400" />
							{env.serviceName}
						</Card.Title>
					</Card.Header>
					<Card.Content>
						{#if env.error}
							<div
								class="rounded-md border border-dashed border-border bg-muted/20 p-6 text-center text-sm text-muted-foreground"
							>
								{env.error}
							</div>
						{:else}
							<Textarea
								value={env.content}
								readonly
								class="min-h-[300px] cursor-text bg-muted/30 font-mono text-sm text-blue-300 focus-visible:ring-0"
							/>
						{/if}
					</Card.Content>
				</Card.Root>
			{/each}
		</div>
	{/if}
</div>
