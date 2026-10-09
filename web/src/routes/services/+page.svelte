<script lang="ts">
	import ServiceStatus from '$lib/components/service-status.svelte';
	import RequestError from '$lib/components/request-error.svelte';
	import { onMount } from 'svelte';
	import {
		api,
		isIISService,
		serviceTypeLabel,
		iisAppKindLabel,
		type ServiceWithWatcher
	} from '$lib/api';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Button from '$lib/components/ui/button';
	import { Server, Play, Square, RefreshCw, Heart, ChevronDown } from '@lucide/svelte';
	import { resolve } from '$app/paths';
	import RequestLoading from '$lib/components/request-loading.svelte';

	let services = $state<ServiceWithWatcher[]>([]);
	let error = $state('');
	let loading = $state(true);
	let actionMsg = $state('');
	let actionError = $state('');
	let pendingServiceID = $state<number | null>(null);
	let pendingServiceAction = $state<'start' | 'stop' | 'restart' | 'health' | null>(null);
	const pendingLabels = {
		start: 'Starting…',
		stop: 'Stopping…',
		restart: 'Restarting…',
		health: 'Checking…'
	};
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

	async function serviceAction(
		id: number,
		action: 'start' | 'stop' | 'restart' | 'health',
		fn: () => Promise<{ message: string }>
	) {
		if (pendingServiceID !== null) return;
		pendingServiceID = id;
		pendingServiceAction = action;
		actionError = '';
		actionMsg = '';
		try {
			const res = await fn();
			const status =
				action === 'health' ? (await api.getService(id)).service : await api.serviceStatus(id);
			services = services.map((svc) => (svc.id === id ? { ...svc, ...status } : svc));
			actionMsg = `${services.find((service) => service.id === id)?.windows_service_name || 'Service'}: ${res.message}`;
			setTimeout(() => (actionMsg = ''), 3000);
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Action failed';
		} finally {
			pendingServiceID = null;
			pendingServiceAction = null;
		}
	}
</script>

{#snippet serviceControls(svc: ServiceWithWatcher, alignEnd = false)}
	<div class={`flex ${alignEnd ? 'justify-end' : ''}`}>
		<DropdownMenu.Root>
			<DropdownMenu.Trigger
				class={Button.buttonVariants({ variant: 'outline', size: 'sm' })}
				disabled={pendingServiceID !== null}
				aria-label={`Actions for ${svc.windows_service_name}`}
				aria-busy={pendingServiceID === svc.id}
			>
				{#if pendingServiceID === svc.id && pendingServiceAction}<RefreshCw
						class="size-4 animate-spin"
					/>{pendingLabels[pendingServiceAction]}{:else}Actions<ChevronDown class="size-4" />{/if}
			</DropdownMenu.Trigger>
			<DropdownMenu.Content
				align="end"
				class="w-44"
				onCloseAutoFocus={(event) => {
					if (serviceConfirmation) event.preventDefault();
				}}
			>
				{#if !isIISService(svc.service_type)}
					<DropdownMenu.Item
						disabled={pendingServiceID !== null}
						onSelect={() => {
							void serviceAction(svc.id, 'start', () => api.startService(svc.id));
						}}><Play />Start service</DropdownMenu.Item
					>
					<DropdownMenu.Item
						disabled={pendingServiceID !== null}
						onSelect={() => {
							actionError = '';
							serviceConfirmation = {
								id: svc.id,
								name: svc.windows_service_name,
								action: 'restart'
							};
						}}><RefreshCw />Restart service</DropdownMenu.Item
					>
				{/if}
				<DropdownMenu.Item
					disabled={pendingServiceID !== null}
					onSelect={() => {
						void serviceAction(svc.id, 'health', () =>
							api
								.serviceStatus(svc.id)
								.then((h) => ({
									message: `Runtime: ${h.last_service_status}; health: ${h.last_health_status}`
								}))
						);
					}}><Heart />Check status</DropdownMenu.Item
				>
				{#if !isIISService(svc.service_type)}
					<DropdownMenu.Item
						variant="destructive"
						class="mt-1 border-t border-border pt-2"
						disabled={pendingServiceID !== null}
						onSelect={() => {
							actionError = '';
							serviceConfirmation = { id: svc.id, name: svc.windows_service_name, action: 'stop' };
						}}><Square />Stop service</DropdownMenu.Item
					>
				{/if}
			</DropdownMenu.Content>
		</DropdownMenu.Root>
	</div>
{/snippet}

<div class="space-y-6">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">Services</h1>
		<p class="text-sm text-muted-foreground">All managed services across NSSM and IIS watchers</p>
	</div>

	<RequestError message={error} onRetry={load} />

	{#if actionMsg}
		<div
			role="status"
			class="rounded-lg border border-blue-500/30 bg-blue-500/10 p-4 text-sm text-blue-400"
		>
			{actionMsg}
		</div>
	{/if}

	{#if !serviceConfirmation}<RequestError message={actionError} />{/if}

	{#if loading}
		<RequestLoading label="Loading services…" />
	{:else if services.length > 0}
		<div class="grid gap-4 lg:hidden">
			{#each services as svc (svc.id)}
				<Card.Root
					><Card.Content class="space-y-4">
						<div class="section-toolbar">
							<div class="min-w-0">
								<a
									href={resolve(`/services/${svc.id}`)}
									class="font-medium break-all hover:underline">{svc.windows_service_name}</a
								>
								<p class="mt-1 text-xs text-muted-foreground">
									{serviceTypeLabel(svc.service_type)}
								</p>
							</div>
							<a
								href={resolve(`/watchers/${svc.watcher_id}`)}
								class="text-sm text-muted-foreground hover:underline">{svc.watcher_name}</a
							>
						</div>
						<div class="space-y-1 text-xs text-muted-foreground">
							<p class="font-mono break-all">
								{isIISService(svc.service_type)
									? iisAppKindLabel(svc.iis_app_kind || 'static')
									: svc.binary_name}
							</p>
							{#if svc.health_check_url}<p class="break-all">Health: {svc.health_check_url}</p>{/if}
						</div>
						<ServiceStatus service={svc} />
						{@render serviceControls(svc)}
					</Card.Content></Card.Root
				>
			{/each}
		</div>
		<Card.Root class="hidden border-border bg-card lg:block">
			<Table.Root>
				<Table.Header
					><Table.Row
						><Table.Head>Service</Table.Head><Table.Head>Watcher</Table.Head><Table.Head
							>Runtime</Table.Head
						><Table.Head>Last known status</Table.Head><Table.Head class="text-right"
							>Actions</Table.Head
						></Table.Row
					></Table.Header
				>
				<Table.Body>
					{#each services as svc (svc.id)}
						<Table.Row>
							<Table.Cell
								><a href={resolve(`/services/${svc.id}`)} class="font-medium hover:underline"
									>{svc.windows_service_name}</a
								>
								<p class="mt-1 text-xs text-muted-foreground">
									{serviceTypeLabel(svc.service_type)}
								</p></Table.Cell
							>
							<Table.Cell
								><a
									href={resolve(`/watchers/${svc.watcher_id}`)}
									class="text-sm text-muted-foreground hover:underline">{svc.watcher_name}</a
								></Table.Cell
							>
							<Table.Cell class="max-w-64 whitespace-normal"
								><p class="font-mono text-xs break-all text-muted-foreground">
									{isIISService(svc.service_type)
										? iisAppKindLabel(svc.iis_app_kind || 'static')
										: svc.binary_name}
								</p>
								{#if svc.health_check_url}<p class="mt-1 text-xs break-all text-muted-foreground">
										Health: {svc.health_check_url}
									</p>{/if}</Table.Cell
							>
							<Table.Cell><ServiceStatus service={svc} /></Table.Cell>
							<Table.Cell
								><div class="ml-auto max-w-80">
									{@render serviceControls(svc, true)}
								</div></Table.Cell
							>
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
		<RequestError message={actionError} />
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
					await serviceAction(selected.id, selected.action, () =>
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
