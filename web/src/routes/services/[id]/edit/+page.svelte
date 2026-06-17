<script lang="ts">
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
	let error = $state('');

	onMount(async () => {
		try {
			const detail = await api.getService(serviceId);
			service = detail.service;
			watcher = detail.watcher;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load service';
		}
	});

	async function updateService(payload: ServiceWritePayload) {
		if (!service || !watcher) return;
		saving = true;
		error = '';
		try {
			await api.updateService(watcher.id, service.id, payload);
			await goto(resolve(`/services/${service.id}`));
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to update service';
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>Edit Service | Watcher</title>
</svelte:head>

<div class="space-y-6">
	<div class="flex items-center gap-4">
		<a href={resolve(`/services/${serviceId}`)}>
			<Button.Root variant="ghost" size="icon" class="h-8 w-8">
				<ArrowLeft class="h-4 w-4" />
			</Button.Root>
		</a>
		<div>
			<h1 class="text-2xl font-bold tracking-tight">Edit Service</h1>
			<p class="text-sm text-muted-foreground">
				{service ? `Update how Watcher manages ${service.windows_service_name}.` : 'Update this service configuration.'}
			</p>
		</div>
	</div>

	{#if service}
		<ServiceWizardForm
			title="Service Setup"
			description="Edit service runtime details, IIS or NSSM settings, and managed files."
			submitLabel="Save Service Changes"
			initial={service}
			submitting={saving}
			error={error}
			onSubmit={updateService}
		/>
	{/if}
</div>
