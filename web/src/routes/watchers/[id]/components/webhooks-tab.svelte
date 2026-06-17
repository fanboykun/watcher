<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Button from '$lib/components/ui/button';
	import * as Select from '$lib/components/ui/select/index.js';
	import type { Watcher, WebhookDelivery } from '$lib/api';
	import { formatDate } from '$lib/utils';
	import { ExternalLink } from '@lucide/svelte';

	let {
		watcher,
		deliveries,
		deliveryPage = $bindable(1),
		deliveryPageSize = $bindable(20),
		deliveryTotal,
		onPageChange,
		onPageSizeChange,
		onSendTest,
		onResume,
		onResumeReplay
	}: {
		watcher: Watcher;
		deliveries: WebhookDelivery[];
		deliveryPage: number;
		deliveryPageSize: number;
		deliveryTotal: number;
		onPageChange: (page: number) => void | Promise<void>;
		onPageSizeChange: (size: number) => void | Promise<void>;
		onSendTest: () => void | Promise<void>;
		onResume: () => void | Promise<void>;
		onResumeReplay: () => void | Promise<void>;
	} = $props();
</script>

<div class="space-y-4">
	<Card.Root class="border-border bg-card">
		<Card.Header class="pb-3 flex flex-row items-center justify-between space-y-0">
			<Card.Title class="text-sm font-medium text-muted-foreground">Webhook Delivery</Card.Title>
			<a
				href="https://github.com/fanboykun/watcher/blob/main/docs/webhooks.md"
				target="_blank"
				rel="noopener noreferrer"
				class="text-xs text-primary hover:underline inline-flex items-center gap-1 font-medium"
			>
				<ExternalLink class="h-3 w-3" />
				Integration Guide
			</a>
		</Card.Header>
		<Card.Content class="space-y-3 text-sm">
			<div class="grid gap-3 sm:grid-cols-2">
				<div class="flex justify-between gap-3">
					<span class="text-muted-foreground">Enabled</span>
					<span>{watcher.webhook_enabled ? 'Yes' : 'No'}</span>
				</div>
				<div class="flex justify-between gap-3">
					<span class="text-muted-foreground">URL</span>
					<span class="max-w-60 truncate font-mono text-xs">{watcher.webhook_url || 'Global default / unset'}</span>
				</div>
				<div class="flex justify-between gap-3">
					<span class="text-muted-foreground">Bearer Token</span>
					<span>{watcher.has_webhook_bearer_token ? (watcher.webhook_bearer_token_masked || 'Configured') : 'Global default'}</span>
				</div>
				<div class="flex justify-between gap-3">
					<span class="text-muted-foreground">Paused</span>
					<span>{watcher.webhook_paused_at ? `Yes (${formatDate(watcher.webhook_paused_at)})` : 'No'}</span>
				</div>
			</div>
			{#if watcher.webhook_pause_reason}
				<div class="rounded border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-500">
					{watcher.webhook_pause_reason}
				</div>
			{/if}
			<div class="flex flex-wrap gap-2">
				<Button.Root size="sm" variant="outline" onclick={onSendTest}>Send Test Webhook</Button.Root>
				<Button.Root size="sm" variant="outline" onclick={onResume}>Resume Only</Button.Root>
				<Button.Root size="sm" variant="outline" onclick={onResumeReplay}>Resume and Replay Suppressed</Button.Root>
			</div>
		</Card.Content>
	</Card.Root>

	<div class="mb-3 flex items-center justify-between gap-2">
		<div class="text-xs text-muted-foreground">
			Showing {deliveries.length === 0 ? 0 : (deliveryPage - 1) * deliveryPageSize + 1} - {Math.min(deliveryPage * deliveryPageSize, deliveryTotal)} of {deliveryTotal}
		</div>
		<div class="flex items-center gap-2">
			<Select.Root
				type="single"
				value={String(deliveryPageSize)}
				onValueChange={(v) => {
					if (v) {
						onPageSizeChange(Number(v));
					}
				}}
			>
				<Select.Trigger class="h-8 w-28 text-xs bg-card">
					{deliveryPageSize} / page
				</Select.Trigger>
				<Select.Content>
					<Select.Item value="20" label="20 / page">20 / page</Select.Item>
					<Select.Item value="50" label="50 / page">50 / page</Select.Item>
					<Select.Item value="100" label="100 / page">100 / page</Select.Item>
				</Select.Content>
			</Select.Root>
			<Button.Root variant="outline" size="sm" disabled={deliveryPage <= 1} onclick={() => onPageChange(deliveryPage - 1)}>Prev</Button.Root>
			<Button.Root variant="outline" size="sm" disabled={deliveryPage * deliveryPageSize >= deliveryTotal} onclick={() => onPageChange(deliveryPage + 1)}>Next</Button.Root>
		</div>
	</div>

	<Card.Root class="border-border bg-card">
		<Table.Root>
			<Table.Header>
				<Table.Row class="border-border hover:bg-transparent">
					<Table.Head>Status</Table.Head>
					<Table.Head>Delivery ID</Table.Head>
					<Table.Head>Attempt</Table.Head>
					<Table.Head>HTTP</Table.Head>
					<Table.Head>When</Table.Head>
					<Table.Head>Error</Table.Head>
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#if deliveries.length === 0}
					<Table.Row class="border-border">
						<Table.Cell colspan={6} class="py-8 text-center text-sm text-muted-foreground">No webhook deliveries yet</Table.Cell>
					</Table.Row>
				{:else}
					{#each deliveries as delivery (delivery.id)}
						<Table.Row class="border-border">
							<Table.Cell class="capitalize">{delivery.status}</Table.Cell>
							<Table.Cell class="font-mono text-xs">{delivery.delivery_id}</Table.Cell>
							<Table.Cell>{delivery.attempt_number}</Table.Cell>
							<Table.Cell>{delivery.response_status_code || '—'}</Table.Cell>
							<Table.Cell class="text-xs text-muted-foreground">{formatDate(delivery.last_attempt_at || delivery.created_at)}</Table.Cell>
							<Table.Cell class="max-w-65 truncate text-xs text-red-400">{delivery.error || '—'}</Table.Cell>
						</Table.Row>
					{/each}
				{/if}
			</Table.Body>
		</Table.Root>
	</Card.Root>
</div>
