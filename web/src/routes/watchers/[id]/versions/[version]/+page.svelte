<script lang="ts">
	import { page } from '$app/state';

	import { resolve } from '$app/paths';
	import { createQuery } from '@tanstack/svelte-query';
	import { queries } from '$lib/query/options';
	import { message } from '$lib/query/client';
	import { type ConfigSnapshotFile } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import { Textarea } from '$lib/components/ui/textarea';
	import { ArrowLeft, Archive } from '@lucide/svelte';
	import RequestLoading from '$lib/components/request-loading.svelte';
	import RequestError from '$lib/components/request-error.svelte';

	const watcherId = $derived(Number(page.params.id));
	const version = $derived(page.params.version || '');
	const snapshotQuery = createQuery(() => queries.snapshot(watcherId, version));
	const snapshot = $derived(snapshotQuery.data ?? null);
	const loading = $derived(snapshotQuery.isPending);
	const error = $derived(message(snapshotQuery.error));

	function targetLabel(target: ConfigSnapshotFile['target']) {
		return target === 'env'
			? 'Environment · app dir'
			: target === 'app_dir'
				? 'App dir'
				: 'Release dir';
	}

	function sourceLabel(source: string) {
		if (source === 'deployment') return 'Deployment';
		if (source === 'active_config_update') return 'Active configuration change';
		if (source === 'current_version_migration') return 'Current version migration';
		return 'Historical snapshot';
	}

	async function loadSnapshot(_force = false) {
		await snapshotQuery.refetch();
	}
</script>

<div class="space-y-6">
	<div class="flex items-center gap-4">
		<Button.Root
			href={resolve('/watchers/[id]', { id: String(watcherId) }) + '?tab=versions'}
			variant="ghost"
			size="icon"
			aria-label="Back to versions"
		>
			<ArrowLeft class="h-4 w-4" />
		</Button.Root>
		<div>
			<h1 class="text-2xl font-bold tracking-tight">Configuration snapshot: {version}</h1>
			<p class="text-sm text-muted-foreground">
				Captured environment and managed files used for rollback. Prepare future changes in
				Candidates.
			</p>
		</div>
	</div>
	<RequestError message={error} onRetry={() => loadSnapshot(true)} />
	{#if loading}
		<RequestLoading label="Loading configuration snapshot..." />
	{:else if snapshot}
		<p class="text-xs text-muted-foreground">
			Captured {new Date(snapshot.captured_at).toLocaleString()} · {sourceLabel(snapshot.source)}
		</p>
		{#if snapshot.services.length === 0}
			<p class="text-sm text-muted-foreground">No managed files were captured for this version.</p>
		{/if}
		{#each snapshot.services as service (service.service_name)}
			<Card.Root>
				<Card.Header class="flex flex-col items-stretch">
					<Card.Title class="flex items-center gap-2"
						><Archive class="h-4 w-4" />{service.service_name}</Card.Title
					>
					<Card.Description
						>{service.files.length} captured file{service.files.length === 1 ? '' : 's'} · {version}</Card.Description
					>
				</Card.Header>
				<Card.Content class="space-y-4">
					{#each service.files as file (`${file.target}:${file.file_path}`)}
						<section
							class="min-w-0 space-y-2"
							aria-label={`${service.service_name} ${file.target} ${file.file_path}`}
						>
							<div class="flex flex-wrap items-center gap-2">
								<span class="rounded bg-muted px-2 py-1 text-xs">{targetLabel(file.target)}</span>
								<span class="font-mono text-sm break-all">{file.file_path}</span>
							</div>
							<Textarea
								value={file.content}
								readonly
								aria-label={`${service.service_name} ${file.target}/${file.file_path} snapshot`}
								class="min-h-[180px] font-mono text-sm"
							/>
						</section>
					{:else}
						<p class="text-sm text-muted-foreground">
							No managed files were captured for this service.
						</p>
					{/each}
				</Card.Content>
			</Card.Root>
		{/each}
	{/if}
</div>
