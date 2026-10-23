<script lang="ts">
	import PageHeading from '$lib/components/console/page-heading.svelte';
	import RequestLoading from '$lib/components/request-loading.svelte';
	import RequestError from '$lib/components/request-error.svelte';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { api, type Service, type ServiceWritePayload, type Watcher } from '$lib/api';
	import * as Button from '$lib/components/ui/button';
	import ServiceWizardForm from '$lib/components/service-wizard-form.svelte';
	import { ArrowLeft } from '@lucide/svelte';

	const serviceId = Number(page.params.id);

	let service = $state<Service | null>(null);
	let watcher = $state<Watcher | null>(null);
	let saving = $state(false);
	let loading = $state(true);
	let error = $state('');

	onMount(() => {
		void load();
	});
	async function load() {
		loading = true;
		error = '';
		try {
			const detail = await api.getService(serviceId);
			service = detail.service;
			watcher = detail.watcher;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load service';
		} finally {
			loading = false;
		}
	}

	async function updateService(payload: ServiceWritePayload) {
		if (!service || !watcher) return false;
		saving = true;
		error = '';
		try {
			await api.updateService(watcher.id, service.id, payload);
			return true;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to update service';
			return false;
		} finally {
			saving = false;
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
	{:else if !service}<RequestError message={error} onRetry={load} />
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
