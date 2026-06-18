<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { api, type ServiceWritePayload, type Watcher } from '$lib/api';
	import * as Button from '$lib/components/ui/button';
	import ServiceWizardForm from '$lib/components/service-wizard-form.svelte';
	import { ArrowLeft } from '@lucide/svelte';

	const watcherId = Number(page.params.id);

	let watcher = $state<Watcher | null>(null);
	let saving = $state(false);
	let error = $state('');

	onMount(async () => {
		try {
			watcher = await api.getWatcher(watcherId);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load watcher';
		}
	});

	async function createService(payload: ServiceWritePayload) {
		saving = true;
		error = '';
		try {
			const service = await api.createService(watcherId, payload);
			await goto(resolve(`/services/${service.id}`));
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to create service';
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>Add Service | Watcher</title>
</svelte:head>

<div class="space-y-6">
	<div class="flex items-center gap-4">
		<a href={resolve(`/watchers/${watcherId}/edit#services`)}>
			<Button.Root variant="ghost" size="icon" class="h-8 w-8">
				<ArrowLeft class="h-4 w-4" />
			</Button.Root>
		</a>
		<div>
			<h1 class="text-2xl font-bold tracking-tight">Add Service</h1>
			<p class="text-sm text-muted-foreground">
				{watcher ? `Create a managed service for ${watcher.name}.` : 'Create a managed service for this watcher.'}
			</p>
		</div>
	</div>

	<ServiceWizardForm
		title="Service Setup"
		description="Define how Watcher should start, health-check, and write runtime files for this service."
		submitLabel="Create Service"
		submitting={saving}
		error={error}
		onSubmit={createService}
	/>
</div>
