<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Button from '$lib/components/ui/button';
	import { Server, Plus, ExternalLink, Pencil, Trash2 } from '@lucide/svelte';
	import { resolve } from '$app/paths';
	import { isIISService, serviceTypeLabel, iisAppKindLabel, type Watcher, type Service } from '$lib/api';

	let {
		watcher,
		readonly = false,
		manageHref = '',
		onAddService,
		onEditService,
		onDeleteService
	}: {
		watcher: Watcher;
		readonly?: boolean;
		manageHref?: string;
		onAddService?: () => void;
		onEditService?: (svc: Service) => void;
		onDeleteService?: (svcId: number, name: string) => void;
	} = $props();
</script>

<div class="mb-4 flex justify-end">
	{#if readonly && manageHref}
		<a href={manageHref}>
			<Button.Root size="sm" variant="outline">
				<Pencil class="mr-2 h-4 w-4" /> Manage Settings
			</Button.Root>
		</a>
	{:else if onAddService}
		<Button.Root size="sm" onclick={onAddService}>
			<Plus class="mr-2 h-4 w-4" /> Add Service
		</Button.Root>
	{/if}
</div>

{#if watcher.services && watcher.services.length > 0}
	<Card.Root class="border-border bg-card">
		<Table.Root>
			<Table.Header>
				<Table.Row class="border-border hover:bg-transparent">
					<Table.Head>Service Name</Table.Head>
					<Table.Head>Type</Table.Head>
					<Table.Head>Binary / App Pool</Table.Head>
					<Table.Head>Health URL</Table.Head>
					{#if !readonly}
						<Table.Head class="text-right">Actions</Table.Head>
					{/if}
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#each watcher.services as svc (svc.id)}
					<Table.Row class="border-border">
						<Table.Cell>
							<a href={resolve(`/services/${svc.id}`)} class="font-medium hover:underline">
								{svc.windows_service_name}
							</a>
							{#if svc.public_url}
								<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
								<a
									href={svc.public_url}
									target="_blank"
									rel="noopener noreferrer"
									class="ml-1.5 inline-flex items-center text-muted-foreground hover:text-foreground"
									title="Open Public URL"
								>
									<ExternalLink class="h-3 w-3" />
								</a>
							{/if}
						</Table.Cell>
						<Table.Cell>
							<span
								class="inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium {isIISService(svc.service_type)
									? 'border-blue-500/30 bg-blue-500/10 text-blue-400'
									: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-400'}"
							>
								{serviceTypeLabel(svc.service_type)}
							</span>
						</Table.Cell>
						<Table.Cell class="font-mono text-xs text-muted-foreground">
							{#if isIISService(svc.service_type)}
								{iisAppKindLabel(svc.iis_app_kind || 'static')}
							{:else}
								{svc.binary_name}
							{/if}
						</Table.Cell>
						<Table.Cell class="font-mono text-xs text-muted-foreground">
							{svc.health_check_url || '—'}
						</Table.Cell>
						{#if !readonly}
							<Table.Cell class="text-right">
								{#if onEditService}
									<Button.Root
										variant="ghost"
										size="icon"
										class="h-8 w-8"
										onclick={() => onEditService(svc)}
										title="Edit"
									>
										<Pencil class="h-4 w-4" />
									</Button.Root>
								{/if}
								{#if onDeleteService}
									<Button.Root
										variant="ghost"
										size="icon"
										class="h-8 w-8 text-red-400 hover:text-red-300"
										onclick={() => onDeleteService(svc.id, svc.windows_service_name)}
										title="Delete"
									>
										<Trash2 class="h-4 w-4" />
									</Button.Root>
								{/if}
							</Table.Cell>
						{/if}
					</Table.Row>
				{/each}
			</Table.Body>
		</Table.Root>
	</Card.Root>
{:else}
	<Card.Root class="border-dashed border-border bg-card">
		<Card.Content class="flex flex-col items-center justify-center py-12 text-center">
			<Server class="mb-3 h-8 w-8 text-muted-foreground/40" />
			<p class="text-sm text-muted-foreground">No services configured</p>
			<p class="mt-1 text-xs text-muted-foreground/60">
				Click "Add Service" to link a Windows service
			</p>
		</Card.Content>
	</Card.Root>
{/if}
