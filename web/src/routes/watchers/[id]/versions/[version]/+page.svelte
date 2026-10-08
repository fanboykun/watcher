<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { api, type Watcher } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import { Textarea } from '$lib/components/ui/textarea';
	import { ArrowLeft, Archive, AlertCircle } from '@lucide/svelte';
	import RequestLoading from '$lib/components/request-loading.svelte';

	interface ServiceEnvItem {
		serviceId: number;
		serviceName: string;
		content: string;
		error?: string;
	}

	const watcherId = $derived(Number(page.params.id));
	const version = $derived(page.params.version || '');
	let watcher = $state<Watcher | null>(null);
	let envs = $state<ServiceEnvItem[]>([]);
	let loading = $state(true);
	let error = $state('');

	$effect(() => {
		const id = watcherId;
		const target = version;
		let disposed = false;
		async function load() {
			loading = true;
			error = '';
			watcher = null;
			envs = [];
			try {
				const loaded = await api.getWatcher(id);
				const results = await Promise.all(loaded.services.map(async (svc) => {
					const item: ServiceEnvItem = {
						serviceId: svc.id,
						serviceName: svc.windows_service_name || svc.iis_site_name || svc.iis_app_pool || `Service ${svc.id}`,
						content: ''
					};
					try {
						item.content = (await api.getServiceSnapshotEnv(svc.id, target)).env_content;
					} catch (e) {
						item.error = e instanceof Error ? e.message : 'Could not load environment snapshot';
					}
					return item;
				}));
				if (!disposed) { watcher = loaded; envs = results; }
			} catch (e) {
				if (!disposed) error = e instanceof Error ? e.message : 'Failed to load version snapshot';
			} finally {
				if (!disposed) loading = false;
			}
		}
		void load();
		return () => { disposed = true; };
	});
</script>

<div class="space-y-6">
	<div class="flex items-center gap-4">
		<Button.Root href={resolve('/watchers/[id]', { id: String(watcherId) }) + '?tab=versions'} variant="ghost" size="icon" aria-label="Back to versions">
			<ArrowLeft class="h-4 w-4" />
		</Button.Root>
		<div>
			<h1 class="text-2xl font-bold tracking-tight">Configuration snapshot: {version}</h1>
			<p class="text-sm text-muted-foreground">Historical configuration used for rollback. Prepare future changes in Candidates.</p>
		</div>
	</div>
	{#if error}
		<p role="alert" class="flex items-center gap-2 text-sm text-red-400"><AlertCircle class="h-4 w-4" />{error}</p>
	{/if}
	{#if loading}
		<RequestLoading label="Loading configuration snapshot..." />
	{:else if watcher}
		{#if envs.length === 0}
			<p class="text-sm text-muted-foreground">No services found for this watcher.</p>
		{/if}
		{#each envs as env (env.serviceId)}
			<Card.Root>
				<Card.Header>
					<Card.Title class="flex items-center gap-2"><Archive class="h-4 w-4" />{env.serviceName}</Card.Title>
					<Card.Description>Environment captured for {version}.</Card.Description>
				</Card.Header>
				<Card.Content>
					{#if env.error}
						<p role="alert" class="text-sm text-amber-300">{env.error}</p>
					{:else}
						<Textarea value={env.content} readonly aria-label={`${env.serviceName} environment snapshot`} class="min-h-[260px] font-mono text-sm" />
					{/if}
				</Card.Content>
			</Card.Root>
		{/each}
	{/if}
</div>
