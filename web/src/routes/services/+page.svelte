<script lang="ts">
	import PageHeading from '$lib/components/console/page-heading.svelte';
	import ListToolbar from '$lib/components/console/list-toolbar.svelte';
	import EmptyState from '$lib/components/console/empty-state.svelte';
	import ServiceStatus from '$lib/components/service-status.svelte';
	import RequestError from '$lib/components/request-error.svelte';
	import { queries } from '$lib/query/options';
	import { message } from '$lib/query/client';
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
	import { createServiceList } from '$lib/features/services/list/list-controller.svelte';

	const model = createServiceList();
</script>

{#snippet serviceControls(svc: ServiceWithWatcher, alignEnd = false)}
	<div class={`flex ${alignEnd ? 'justify-end' : ''}`}>
		<DropdownMenu.Root>
			<DropdownMenu.Trigger
				class={Button.buttonVariants({ variant: 'outline', size: 'sm' })}
				disabled={model.pendingServiceID !== null}
				aria-label={`Actions for ${svc.windows_service_name}`}
				aria-busy={model.pendingServiceID === svc.id}
			>
				{#if model.pendingServiceID === svc.id && model.pendingServiceAction}<RefreshCw
						class="size-4 animate-spin"
					/>{model.pendingLabels[model.pendingServiceAction]}{:else}Actions<ChevronDown
						class="size-4"
					/>{/if}
			</DropdownMenu.Trigger>
			<DropdownMenu.Content
				align="end"
				class="w-44"
				onCloseAutoFocus={(event) => {
					if (model.ui.serviceConfirmation) event.preventDefault();
				}}
			>
				{#if !isIISService(svc.service_type)}
					<DropdownMenu.Item
						disabled={model.pendingServiceID !== null}
						onSelect={() => {
							void model.serviceAction(svc.id, 'start', () => api.startService(svc.id));
						}}><Play />Start service</DropdownMenu.Item
					>
					<DropdownMenu.Item
						disabled={model.pendingServiceID !== null}
						onSelect={() => {
							model.ui.actionError = '';
							model.ui.serviceConfirmation = {
								id: svc.id,
								name: svc.windows_service_name,
								action: 'restart'
							};
						}}><RefreshCw />Restart service</DropdownMenu.Item
					>
				{/if}
				<DropdownMenu.Item
					disabled={model.pendingServiceID !== null}
					onSelect={() => {
						void model.serviceAction(svc.id, 'health', () =>
							model.client.fetchQuery(queries.serviceStatus(svc.id)).then((h) => ({
								message: `Runtime: ${h.last_service_status}; health: ${h.last_health_status}`
							}))
						);
					}}><Heart />Check status</DropdownMenu.Item
				>
				{#if !isIISService(svc.service_type)}
					<DropdownMenu.Item
						variant="destructive"
						class="mt-1 border-t border-border pt-2"
						disabled={model.pendingServiceID !== null}
						onSelect={() => {
							model.ui.actionError = '';
							model.ui.serviceConfirmation = {
								id: svc.id,
								name: svc.windows_service_name,
								action: 'stop'
							};
						}}><Square />Stop service</DropdownMenu.Item
					>
				{/if}
			</DropdownMenu.Content>
		</DropdownMenu.Root>
	</div>
{/snippet}

<div class="space-y-6">
	<PageHeading
		eyebrow="Workspace / Runtime"
		title="Services"
		description="The running parts of your infrastructure. Check health, inspect logs, and manage runtime state."
	>
		{#snippet actions()}<Button.Root
				variant="outline"
				size="sm"
				onclick={model.load}
				disabled={model.loading}><RefreshCw class="size-3.5" />Refresh services</Button.Root
			>{/snippet}
	</PageHeading>
	{#if !model.loading && model.services.length}
		<ListToolbar
			bind:value={model.ui.query}
			label="Search services or watchers…"
			count={model.filteredServices.length}
			total={model.services.length}
		>
			<select
				class="filter-select"
				aria-label="Filter services by runtime"
				bind:value={model.ui.runtime}
				><option value="all">All runtimes</option><option value="nssm">NSSM services</option><option
					value="iis">IIS sites</option
				></select
			>
		</ListToolbar>
	{/if}

	<RequestError message={model.error} onRetry={model.load} />

	{#if model.ui.actionMsg}
		<div
			role="status"
			class="rounded-lg border border-blue-500/30 bg-blue-500/10 p-4 text-sm text-blue-400"
		>
			{model.ui.actionMsg}
		</div>
	{/if}

	{#if !model.ui.serviceConfirmation}<RequestError message={model.ui.actionError} />{/if}

	{#if model.loading}
		<RequestLoading label="Loading services…" />
	{:else if model.filteredServices.length > 0}
		<div class="grid gap-4 lg:hidden">
			{#each model.filteredServices as svc (svc.id)}
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
					{#each model.filteredServices as svc (svc.id)}
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
	{:else if !model.error}
		{#if model.services.length}
			<EmptyState
				title="No matching services"
				description="Try another name or select a different runtime."
				icon={Server}
				>{#snippet actions()}<Button.Root
						variant="outline"
						size="sm"
						onclick={() => {
							model.ui.query = '';
							model.ui.runtime = 'all';
						}}>Clear filters</Button.Root
					>{/snippet}</EmptyState
			>
		{:else}
			<EmptyState
				title="A home for every service."
				description="Services belong to a watcher. Open a watcher to register an NSSM service or an IIS site."
				icon={Server}
				>{#snippet actions()}<Button.Root href={resolve('/watchers')} size="sm"
						>Go to watchers</Button.Root
					>{/snippet}</EmptyState
			>
		{/if}
	{/if}
</div>

<Dialog.Root
	open={model.ui.serviceConfirmation !== null}
	onOpenChange={(open) => {
		if (!open && model.pendingServiceID === null) model.ui.serviceConfirmation = null;
	}}
>
	<Dialog.Content showCloseButton={model.pendingServiceID === null}>
		<Dialog.Header
			><Dialog.Title
				>{model.ui.serviceConfirmation?.action === 'stop'
					? 'Stop service?'
					: 'Restart service?'}</Dialog.Title
			><Dialog.Description
				>{model.ui.serviceConfirmation?.name} will stop serving traffic{model.ui.serviceConfirmation
					?.action === 'restart'
					? ' briefly while it restarts'
					: ''}.</Dialog.Description
			></Dialog.Header
		>
		<RequestError message={model.ui.actionError} />
		<Dialog.Footer
			><Button.Root
				variant="outline"
				disabled={model.pendingServiceID !== null}
				onclick={() => (model.ui.serviceConfirmation = null)}>Cancel</Button.Root
			><Button.Root
				variant="destructive"
				loading={model.pendingServiceID !== null}
				disabled={model.pendingServiceID !== null}
				onclick={async () => {
					const selected = model.ui.serviceConfirmation;
					if (!selected) return;
					await model.serviceAction(selected.id, selected.action, () =>
						selected.action === 'stop'
							? api.stopService(selected.id)
							: api.restartService(selected.id)
					);
					if (!model.ui.actionError) model.ui.serviceConfirmation = null;
				}}
				>{model.ui.serviceConfirmation?.action === 'stop'
					? 'Stop service'
					: 'Restart service'}</Button.Root
			></Dialog.Footer
		>
	</Dialog.Content>
</Dialog.Root>
