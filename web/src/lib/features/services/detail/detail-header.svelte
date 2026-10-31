<script lang="ts">
	import { page } from '$app/state';
	import { api, isIISService, type Service, type Watcher } from '$lib/api';
	import * as Button from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { ArrowLeft, Play, Square, RefreshCw, ChevronDown, Pencil, Trash2 } from '@lucide/svelte';
	import { resolve } from '$app/paths';
	import type { createServiceDetail } from '$lib/features/services/detail/detail-controller.svelte';
	let { model }: { model: ReturnType<typeof createServiceDetail> } = $props();
</script>

<header class="page-header border-b border-border pb-6">
	<div class="page-identity">
		<Button.Root
			href={resolve('/services')}
			variant="ghost"
			size="icon"
			aria-label="Back to services"><ArrowLeft /></Button.Root
		>
		<div class="min-w-0">
			<p class="eyebrow mb-2">Workspace / service</p>
			<h1 class="text-2xl font-semibold tracking-tight">
				{model.service?.windows_service_name || model.service?.binary_name || 'Service'}
			</h1>
			{#if model.watcher}<a
					href={resolve(`/watchers/${model.watcher.id}`)}
					class="text-sm text-muted-foreground hover:underline">Watcher · {model.watcher.name}</a
				>{/if}
		</div>
	</div>
	{#if model.service}
		<div class="page-actions">
			<Button.Root
				href={resolve(`/services/${model.id}/edit`)}
				variant="outline"
				size="sm"
				disabled={model.actionBusy}><Pencil /> Edit</Button.Root
			>
			<DropdownMenu.Root>
				<DropdownMenu.Trigger
					class={Button.buttonVariants({ variant: 'outline', size: 'sm' })}
					disabled={model.actionBusy}
					aria-busy={Boolean(model.pendingAction)}
				>
					{#if model.pendingAction}<RefreshCw
							class="animate-spin"
						/>Working…{:else}Actions<ChevronDown />{/if}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content
					align="end"
					class="w-44"
					onCloseAutoFocus={(event) => {
						if (model.ui.confirmServiceAction || model.ui.showDeleteDialog) event.preventDefault();
					}}
				>
					{#if !isIISService(model.service.service_type)}
						<DropdownMenu.Item
							onSelect={() => {
								void model.runAction('start', () => api.startService(model.id));
							}}><Play />Start service</DropdownMenu.Item
						>
						<DropdownMenu.Item
							onSelect={() => {
								model.ui.error = '';
								model.ui.confirmServiceAction = 'restart';
							}}><RefreshCw />Restart service</DropdownMenu.Item
						>
						<DropdownMenu.Item
							onSelect={() => {
								model.ui.error = '';
								model.ui.confirmServiceAction = 'stop';
							}}><Square />Stop service</DropdownMenu.Item
						>
					{/if}
					<DropdownMenu.Item
						variant="destructive"
						class="mt-1 border-t border-border pt-2"
						onSelect={() => {
							model.ui.error = '';
							model.ui.showDeleteDialog = true;
						}}><Trash2 />Delete service</DropdownMenu.Item
					>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		</div>
	{/if}
</header>
