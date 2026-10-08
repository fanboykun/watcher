<script lang="ts">
	import RequestError from '$lib/components/request-error.svelte';
	import { onMount } from 'svelte';
	import {
		api,
		isIISService,
		serviceTypeLabel,
		iisAppKindLabel,
		type ServiceWithWatcher
	} from '$lib/api';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Button from '$lib/components/ui/button';
	import { Server, Play, Square, RefreshCw, Heart, AlertCircle } from '@lucide/svelte';
	import { resolve } from '$app/paths';
	import RequestLoading from '$lib/components/request-loading.svelte';

	let services = $state<ServiceWithWatcher[]>([]);
	let error = $state('');
	let loading = $state(true);
	let actionMsg = $state('');
	let actionError = $state('');
	let pendingServiceID = $state<number | null>(null);
	let serviceConfirmation = $state<{ id: number; name: string; action: 'stop' | 'restart' } | null>(
		null
	);

	onMount(load);

	async function load() {
		loading = true;
		try {
			services = await api.listServices();
			error = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load services';
		} finally {
			loading = false;
		}
	}

	async function serviceAction(id: number, fn: () => Promise<{ message: string }>) {
		if (pendingServiceID !== null) return;
		pendingServiceID = id;
		actionError = '';
		try {
			const res = await fn();
			actionMsg = res.message;
			setTimeout(() => (actionMsg = ''), 3000);
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Action failed';
		} finally {
			pendingServiceID = null;
		}
	}
</script>

<div class="space-y-6">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">Services</h1>
		<p class="text-sm text-muted-foreground">All managed services across NSSM and IIS watchers</p>
	</div>

	<RequestError message={error} onRetry={load} />

	{#if actionMsg}
		<div class="rounded-lg border border-blue-500/30 bg-blue-500/10 p-4 text-sm text-blue-400">
			{actionMsg}
		</div>
	{/if}

	<RequestError message={actionError} />

	{#if loading}
		<RequestLoading label="Loading services…" />
	{:else if services.length > 0}
		<Card.Root class="border-border bg-card">
			<Table.Root>
				<Table.Header>
					<Table.Row class="border-border hover:bg-transparent">
						<Table.Head>Service</Table.Head>
						<Table.Head>Watcher</Table.Head>
						<Table.Head>Mode</Table.Head>
						<Table.Head>Target</Table.Head>
						<Table.Head>Health URL</Table.Head>
						<Table.Head class="text-right">Actions</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each services as svc (svc.id)}
						<Table.Row class="border-border">
							<Table.Cell>
								<a href={resolve(`/services/${svc.id}`)} class="font-medium hover:underline"
									>{svc.windows_service_name}</a
								>
							</Table.Cell>
							<Table.Cell>
								<a
									href={resolve(`/watchers/${svc.watcher_id}`)}
									class="text-sm text-muted-foreground hover:underline"
								>
									{svc.watcher_name}
								</a>
							</Table.Cell>
							<Table.Cell class="font-mono text-xs text-muted-foreground">
								{serviceTypeLabel(svc.service_type)}
							</Table.Cell>
							<Table.Cell class="font-mono text-xs text-muted-foreground">
								{#if isIISService(svc.service_type)}
									{iisAppKindLabel(svc.iis_app_kind || 'static')}
								{:else}
									{svc.binary_name}
								{/if}
							</Table.Cell>
							<Table.Cell class="font-mono text-xs text-muted-foreground"
								>{svc.health_check_url || '—'}</Table.Cell
							>
							<Table.Cell>
								<div class="flex items-center justify-end gap-1">
									{#if !isIISService(svc.service_type)}
										<Button.Root
											variant="ghost"
											size="icon"
											disabled={pendingServiceID !== null}
											class="h-8 w-8 text-emerald-400"
											onclick={() => serviceAction(svc.id, () => api.startService(svc.id))}
											title="Start"
										>
											<Play class="h-4 w-4" />
										</Button.Root>
										<Button.Root
											variant="ghost"
											size="icon"
											disabled={pendingServiceID !== null}
											class="h-8 w-8 text-red-400"
											onclick={() => {
												actionError = '';
												serviceConfirmation = {
													id: svc.id,
													name: svc.windows_service_name,
													action: 'stop'
												};
											}}
											title="Stop"
										>
											<Square class="h-4 w-4" />
										</Button.Root>
										<Button.Root
											variant="ghost"
											size="icon"
											disabled={pendingServiceID !== null}
											class="h-8 w-8 text-amber-400"
											onclick={() => {
												actionError = '';
												serviceConfirmation = {
													id: svc.id,
													name: svc.windows_service_name,
													action: 'restart'
												};
											}}
											title="Restart"
										>
											<RefreshCw class="h-4 w-4" />
										</Button.Root>
									{/if}
									<Button.Root
										variant="ghost"
										size="icon"
										disabled={pendingServiceID !== null}
										class="h-8 w-8 text-blue-400"
										onclick={() =>
											serviceAction(svc.id, () =>
												api
													.serviceHealth(svc.id)
													.then((h) => ({ message: `${svc.windows_service_name}: ${h.status}` }))
											)}
										title="Health check"
									>
										<Heart class="h-4 w-4" />
									</Button.Root>
								</div>
							</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		</Card.Root>
	{:else if !error}
		<Card.Root class="border-dashed border-border bg-card">
			<Card.Content class="flex flex-col items-center justify-center py-16 text-center">
				<Server class="mb-3 h-10 w-10 text-muted-foreground/40" />
				<h3 class="text-sm font-medium text-muted-foreground">No services registered</h3>
				<p class="mt-1 text-xs text-muted-foreground/60">Services are created under watchers</p>
			</Card.Content>
		</Card.Root>
	{/if}
</div>

<Dialog.Root
	open={serviceConfirmation !== null}
	onOpenChange={(open) => {
		if (!open && pendingServiceID === null) serviceConfirmation = null;
	}}
>
	<Dialog.Content showCloseButton={pendingServiceID === null}>
		<Dialog.Header
			><Dialog.Title
				>{serviceConfirmation?.action === 'stop'
					? 'Stop service?'
					: 'Restart service?'}</Dialog.Title
			><Dialog.Description
				>{serviceConfirmation?.name} will stop serving traffic{serviceConfirmation?.action ===
				'restart'
					? ' briefly while it restarts'
					: ''}.</Dialog.Description
			></Dialog.Header
		>
		{#if actionError}<p role="alert" class="text-sm text-red-400">{actionError}</p>{/if}
		<Dialog.Footer
			><Button.Root
				variant="outline"
				disabled={pendingServiceID !== null}
				onclick={() => (serviceConfirmation = null)}>Cancel</Button.Root
			><Button.Root
				variant="destructive"
				loading={pendingServiceID !== null}
				disabled={pendingServiceID !== null}
				onclick={async () => {
					const selected = serviceConfirmation;
					if (!selected) return;
					await serviceAction(selected.id, () =>
						selected.action === 'stop'
							? api.stopService(selected.id)
							: api.restartService(selected.id)
					);
					if (!actionError) serviceConfirmation = null;
				}}
				>{serviceConfirmation?.action === 'stop' ? 'Stop service' : 'Restart service'}</Button.Root
			></Dialog.Footer
		>
	</Dialog.Content>
</Dialog.Root>
