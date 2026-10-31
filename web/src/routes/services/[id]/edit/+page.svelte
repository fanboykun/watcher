<script lang="ts">
	import PageHeading from '$lib/components/console/page-heading.svelte';
	import RequestLoading from '$lib/components/request-loading.svelte';
	import ErrorScreen from '$lib/components/errors/error-screen.svelte';
	import { untrack } from 'svelte';
	import { createQuery, createMutation, useQueryClient } from '@tanstack/svelte-query';
	import { queries } from '$lib/query/options';
	import { invalidate, message, serviceScope } from '$lib/query/client';

	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { api, type Service, type ServiceWritePayload, type Watcher } from '$lib/api';
	import * as Button from '$lib/components/ui/button';
	import ServiceWizardForm from '$lib/components/service-wizard-form.svelte';
	import { ArrowLeft } from '@lucide/svelte';

	const serviceId = $derived(Number(page.params.id));

	const client = useQueryClient();
	const detail = createQuery(() => queries.service(serviceId));
	// The form owns its draft and remounts whenever `initial` changes identity. Seed it once,
	// from a response fetched for this view: runtime fields such as last_health_checked_at
	// change on every refresh and must not wipe in-progress edits.
	const ready = $derived(detail.isFetchedAfterMount && detail.data?.service.id === serviceId);
	const service = $derived(ready ? untrack(() => detail.data!.service) : null);
	const watcher = $derived(detail.data?.watcher ?? null);
	const mutation = createMutation(() => ({
		mutationFn: (payload: ServiceWritePayload) =>
			api.updateService(watcher!.id, serviceId, payload),
		onSuccess: (saved) => invalidate(client, ...serviceScope(serviceId, saved.watcher_id))
	}));
	const loading = $derived(detail.isPending || !detail.isFetchedAfterMount);
	const saving = $derived(mutation.isPending);
	const error = $derived(message(detail.error || mutation.error));
	async function load() {
		await detail.refetch();
	}
	async function updateService(payload: ServiceWritePayload) {
		try {
			await mutation.mutateAsync(payload);
			return true;
		} catch {
			return false;
		}
	}
</script>

<svelte:head>
	<title>Edit Service | Watcher</title>
</svelte:head>

<div class="space-y-6">
	<PageHeading
		eyebrow="Workspace / Service configuration"
		title="Edit service"
		description={service
			? `Update how Watcher manages ${service.windows_service_name}.`
			: 'Update service runtime, health checks, and managed files.'}
	>
		{#snippet actions()}<Button.Root
				href={resolve(`/services/${serviceId}`)}
				variant="outline"
				size="sm"><ArrowLeft class="size-3.5" />Back to service</Button.Root
			>{/snippet}
	</PageHeading>

	{#if loading}<RequestLoading label="Loading service configuration…" />
	{:else if !service}<ErrorScreen error={detail.error} onRetry={load} />
	{:else}
		<ServiceWizardForm
			layout="single"
			showHeader={false}
			title="Service Setup"
			description="Edit service runtime details, IIS or NSSM settings, and managed files."
			submitLabel="Save Service Changes"
			initial={service}
			submitting={saving}
			{error}
			onSubmit={updateService}
		/>
	{/if}
</div>
