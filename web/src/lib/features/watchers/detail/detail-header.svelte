<script lang="ts">
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { page } from '$app/state';
	import { type Watcher } from '$lib/api';
	import * as Button from '$lib/components/ui/button';
	import { ArrowLeft, Zap, Play, Pause, RefreshCw, RotateCcw, Pencil } from '@lucide/svelte';
	import { resolve } from '$app/paths';
	import { statusColor } from '$lib/utils';
	import type { createWatcherDetail } from '$lib/features/watchers/detail/detail-controller.svelte';
	let { model }: { model: ReturnType<typeof createWatcherDetail> } = $props();
</script>

<header class="page-header border-b border-border pb-6">
	<div class="page-identity">
		<Button.Root
			href={resolve('/watchers')}
			variant="ghost"
			size="icon"
			aria-label="Back to watchers"><ArrowLeft /></Button.Root
		>
		<div class="min-w-0">
			<p class="eyebrow mb-2">Workspace / watcher</p>
			<h1 class="text-2xl font-semibold tracking-tight">{model.watcher?.name || 'Watcher'}</h1>
			{#if model.watcher && model.watcher.service_name !== model.watcher.name}<p
					class="text-sm text-muted-foreground"
				>
					{model.watcher.service_name}
				</p>{/if}
		</div>
		{#if model.watcher}<span
				class="shrink-0 rounded-full border px-2.5 py-1 text-xs font-medium capitalize {statusColor(
					model.watcher.status
				)}">Deployment: {model.watcher.status.replaceAll('_', ' ')}</span
			>{/if}
	</div>
	{#if model.watcher}
		<div class="page-actions">
			<Button.Root
				variant="outline"
				size="sm"
				disabled={model.actionBusy}
				loading={model.pendingAction === 'pause'}
				onclick={() => model.runPending('pause', model.togglePause)}
				>{#if model.watcher.paused}<Play /> Resume{:else}<Pause /> Pause{/if}</Button.Root
			>
			<Button.Root
				variant="outline"
				size="sm"
				disabled={model.actionBusy || model.watcher.paused}
				loading={model.pendingAction === 'poll'}
				onclick={() => model.runPending('poll', model.triggerCheck)}
				><RefreshCw /> Poll now</Button.Root
			>
			<Button.Root
				href={resolve(`/watchers/${model.id}/edit`)}
				variant="outline"
				size="sm"
				disabled={model.actionBusy}><Pencil /> Edit</Button.Root
			>
			<DropdownMenu.Root>
				<DropdownMenu.Trigger
					class={Button.buttonVariants({ variant: 'outline', size: 'sm' })}
					disabled={model.actionBusy}
					>{#if model.pendingAction === 'intercept'}<RefreshCw
							class="animate-spin"
						/>Updating…{:else}More actions{/if}</DropdownMenu.Trigger
				>
				<DropdownMenu.Content align="end" class="w-56">
					<DropdownMenu.Item
						disabled={model.actionBusy}
						onSelect={() => {
							void model.runPending('intercept', model.toggleIntercept);
						}}
						><Zap />Intercept next: {model.watcher.intercept_next_release
							? 'On'
							: 'Off'}</DropdownMenu.Item
					>
					<DropdownMenu.Item
						class="text-amber-400"
						disabled={model.actionBusy ||
							!!model.watcher.pending_catalog_id ||
							model.watcher.status === 'pending_approval'}
						onSelect={model.triggerRedeploy}><RotateCcw />Redeploy</DropdownMenu.Item
					>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		</div>
	{/if}
</header>
