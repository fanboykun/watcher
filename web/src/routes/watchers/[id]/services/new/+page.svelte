<script lang="ts">
	import PageHeading from '$lib/components/console/page-heading.svelte';
	import RequestLoading from '$lib/components/request-loading.svelte';
	import RequestError from '$lib/components/request-error.svelte';
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
	let createdServiceID = 0;
	let loading = $state(true);
	let error = $state('');

	onMount(() => {
		void load();
	});
	async function load() {
		loading = true;
		error = '';
		try {
			watcher = await api.getWatcher(watcherId);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load watcher';
		} finally {
			loading = false;
		}
	}

	async function createService(payload: ServiceWritePayload) {
		saving = true;
		error = '';
		try {
			const service = await api.createService(watcherId, payload);
			createdServiceID = service.id;
			return true;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to create service';
			return false;
		} finally {
			saving = false;
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
