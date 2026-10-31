<script lang="ts">
	import { resolve } from '$app/paths';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import { webhookDocsHref } from '$lib/webhooks';
	import { ExternalLink, Send } from '@lucide/svelte';
	import type { createDelivery } from '$lib/features/webhooks/delivery/delivery-controller.svelte';
	let { model }: { model: ReturnType<typeof createDelivery> } = $props();
</script>

<Card.Root>
	<Card.Header class="section-toolbar">
		<div>
			<Card.Title class="text-base">Webhook delivery</Card.Title><Card.Description
				>{!model.watcher.webhook_enabled
					? 'Disabled for this watcher'
					: model.watcher.webhook_paused_at
						? 'Paused after delivery failures'
						: 'Enabled'}</Card.Description
			>
		</div>
		<div class="page-actions">
			<Button.Root
				variant="outline"
				size="sm"
				disabled={model.busy || !model.watcher.webhook_enabled}
				onclick={model.onSendTest}><Send /> Send test</Button.Root
			>
			<Button.Root href={resolve('/webhooks')} variant="ghost" size="sm">Webhook hub</Button.Root>
			<Button.Root
				href={webhookDocsHref}
				target="_blank"
				rel="noopener noreferrer"
				variant="ghost"
				size="sm">Guide <ExternalLink /></Button.Root
			>
		</div>
	</Card.Header>
	<Card.Content class="space-y-4">
		{#if model.watcher.webhook_enabled}
			<div class="grid grid-cols-2 gap-4 sm:grid-cols-4">
				<div>
					<p class="text-xs text-muted-foreground">Failure streak</p>
					<p class="mt-1 text-xl font-semibold">{model.watcher.webhook_failure_streak}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">Succeeded · this page</p>
					<p class="mt-1 text-xl font-semibold">{model.deliverySummary.successCount}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">Pending · this page</p>
					<p class="mt-1 text-xl font-semibold">{model.deliverySummary.pendingCount}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">Suppressed · this page</p>
					<p class="mt-1 text-xl font-semibold">{model.deliverySummary.suppressedCount}</p>
				</div>
			</div>
		{/if}
		{#if model.watcher.webhook_paused_at || model.deliverySummary.suppressedCount > 0}
			<div class="space-y-3 rounded-lg border border-amber-500/30 bg-amber-500/5 p-3">
				<p class="text-sm break-words">
					{model.watcher.webhook_pause_reason || 'Delivery needs attention.'}
				</p>
				<div class="page-actions">
					<Button.Root variant="outline" size="sm" disabled={model.busy} onclick={model.onResume}
						>Resume future deliveries</Button.Root
					><Button.Root
						variant="outline"
						size="sm"
						disabled={model.busy}
						onclick={() => (model.ui.showReplay = true)}>Replay suppressed deliveries</Button.Root
					>
				</div>
			</div>
		{/if}
		<details class="text-sm">
			<summary class="cursor-pointer text-muted-foreground">Routing and credentials</summary>
			<dl class="mt-3 grid gap-3 sm:grid-cols-2">
				<div>
					<dt class="text-xs text-muted-foreground">
						Endpoint · {model.routeSource(model.watcher.webhook_url)}
					</dt>
					<dd class="mt-1 font-mono text-xs break-all">
						{model.watcher.webhook_url || 'Inherited from Settings'}
					</dd>
				</div>
				<div>
					<dt class="text-xs text-muted-foreground">Signing secret</dt>
					<dd class="mt-1">{model.authSource(model.watcher)}</dd>
				</div>
			</dl>
		</details>
	</Card.Content>
</Card.Root>
