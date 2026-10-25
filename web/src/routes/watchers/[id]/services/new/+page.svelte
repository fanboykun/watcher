<script lang="ts">
	import PageHeading from '$lib/components/console/page-heading.svelte';
	import RequestLoading from '$lib/components/request-loading.svelte';
	import RequestError from '$lib/components/request-error.svelte';
	import { createQuery, createMutation, useQueryClient } from '@tanstack/svelte-query';
	import { queries, keys } from '$lib/query/options';
	import { invalidate, message } from '$lib/query/client';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { api, type ServiceWritePayload, type Watcher } from '$lib/api';
	import * as Button from '$lib/components/ui/button';
	import ServiceWizardForm from '$lib/components/service-wizard-form.svelte';
	import { ArrowLeft } from '@lucide/svelte';

	const watcherId = $derived(Number(page.params.id));

	const client = useQueryClient();
	const detail = createQuery(() => queries.watcher(watcherId));
	const watcher = $derived(detail.data ?? null);
	const mutation = createMutation(() => ({
		mutationFn: (payload: ServiceWritePayload) => api.createService(watcherId, payload),
		onSuccess: () => invalidate(client, keys.services, keys.watchers, keys.watcher(watcherId))
	}));
	let createdServiceID = 0;
	const loading = $derived(detail.isPending);
	const saving = $derived(mutation.isPending);
	const error = $derived(message(detail.error || mutation.error));
	async function load() {
		await detail.refetch();
	}
	async function createService(payload: ServiceWritePayload) {
		try {
			const result = await mutation.mutateAsync(payload);
			createdServiceID = result.id;
			return true;
		} catch {
			return false;
		}
	}
</script>

<svelte:head>
	<title>Add Service | Watcher</title>
</svelte:head>

<div class="space-y-6">
	<PageHeading
		eyebrow="Workspace / New service"
		title="Add service"
		description={watcher
			? `Create a managed service for ${watcher.name}.`
			: 'Define how Watcher runs and monitors your service.'}
	>
		{#snippet actions()}<Button.Root
				href={resolve(`/watchers/${watcherId}/edit#services`)}
				variant="outline"
				size="sm"><ArrowLeft class="size-3.5" />Back to watcher</Button.Root
			>{/snippet}
	</PageHeading>

	{#if loading}<RequestLoading label="Loading watcher configuration…" />
	{:else if !watcher}<RequestError message={error} onRetry={load} />
	{:else}
		<ServiceWizardForm
			showHeader={false}
			title="Service Setup"
			description="Define how Watcher should start, health-check, and write runtime files for this service."
			submitLabel="Create Service"
			submitting={saving}
			{error}
			onSubmit={createService}
			onSaved={() => goto(resolve(`/services/${createdServiceID}`))}
		/>
	{/if}
</div>
