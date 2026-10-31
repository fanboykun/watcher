<script lang="ts">
	import DeliveryStatus from '$lib/features/webhooks/delivery/delivery-status.svelte';
	import DeliveryDetails from '$lib/features/webhooks/delivery/delivery-details.svelte';
	import { message } from '$lib/query/client';
	import RequestError from '$lib/components/request-error.svelte';

	import * as Card from '$lib/components/ui/card';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Table from '$lib/components/ui/table';
	import * as Button from '$lib/components/ui/button';
	import * as Select from '$lib/components/ui/select/index.js';
	import { type Watcher, type WebhookDelivery } from '$lib/api';
	import { formatDate, timeAgo } from '$lib/utils';

	import { Activity } from '@lucide/svelte';
	import { createDelivery } from '$lib/features/webhooks/delivery/delivery-controller.svelte';
	let {
		watcher,
		busy = false,
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
		busy?: boolean;
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
	const model = createDelivery(() => ({
		watcher,
		busy,
		deliveries,
		deliveryPage,
		deliveryPageSize,
		deliveryTotal,
		onPageChange,
		onPageSizeChange,
		onSendTest,
		onResume,
		onResumeReplay
	}));
</script>

<div class="space-y-5">
	<DeliveryStatus {model} />

	<div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
		<div>
			<h3 class="text-base font-semibold">Recent deliveries</h3>
			<p class="mt-1 text-sm text-muted-foreground">Delivery attempts and receiver responses.</p>
		</div>
		<div class="flex flex-wrap items-center gap-2">
			<div class="text-xs text-muted-foreground">
				Showing {model.safeDeliveries.length === 0
					? 0
					: (model.deliveryPage - 1) * model.deliveryPageSize + 1} -
				{Math.min(model.deliveryPage * model.deliveryPageSize, model.deliveryTotal)} of {model.deliveryTotal}
			</div>
			<Select.Root
				type="single"
				value={String(model.deliveryPageSize)}
				onValueChange={(v) => {
					if (v) {
						model.onPageSizeChange(Number(v));
					}
				}}
			>
				<Select.Trigger class="h-8 w-28 bg-card text-xs">
					{model.deliveryPageSize} / page
				</Select.Trigger>
				<Select.Content>
					<Select.Item value="20" label="20 / page">20 / page</Select.Item>
					<Select.Item value="50" label="50 / page">50 / page</Select.Item>
					<Select.Item value="100" label="100 / page">100 / page</Select.Item>
				</Select.Content>
			</Select.Root>
			<Button.Root
				variant="outline"
				size="sm"
				disabled={model.deliveryPage <= 1}
				onclick={() => model.onPageChange(model.deliveryPage - 1)}
			>
				Prev
			</Button.Root>
			<Button.Root
				variant="outline"
				size="sm"
				disabled={model.deliveryPage * model.deliveryPageSize >= model.deliveryTotal}
				onclick={() => model.onPageChange(model.deliveryPage + 1)}
			>
				Next
			</Button.Root>
		</div>
	</div>

	{#if model.safeDeliveries.length === 0}
		<Card.Root
			><Card.Content class="py-8 text-center"
				><p class="text-sm text-muted-foreground">No webhook deliveries yet.</p></Card.Content
			></Card.Root
		>
	{:else}
		<Card.Root class="border-border bg-card">
			<Table.Root>
				<Table.Header>
					<Table.Row class="border-border hover:bg-transparent">
						<Table.Head>Event</Table.Head>
						<Table.Head>Outcome</Table.Head>
						<Table.Head>Route</Table.Head>
						<Table.Head>Activity</Table.Head>
						<Table.Head>Operator Note</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#if model.safeDeliveries.length === 0}
						<Table.Row class="border-border">
							<Table.Cell colspan={5} class="py-10 text-center">
								<div class="mx-auto max-w-md space-y-2">
									<p class="font-medium">No webhook deliveries yet</p>
									<p class="text-sm text-muted-foreground">
										Once this watcher emits webhook traffic, delivery attempts, retry state, and
										receiver responses will show up here.
									</p>
								</div>
							</Table.Cell>
						</Table.Row>
					{:else}
						{#each model.safeDeliveries as delivery (delivery.id)}
							<Table.Row class="border-border align-top">
								<Table.Cell class="min-w-60">
									<div class="space-y-1">
										<p class="font-mono text-xs text-blue-200">{delivery.event_type || '—'}</p>
										<p class="text-sm break-words whitespace-normal">
											{delivery.summary || 'No summary recorded'}
										</p>
										<p class="text-xs text-muted-foreground">Delivery ID {delivery.delivery_id}</p>
									</div>
								</Table.Cell>
								<Table.Cell class="min-w-42">
									<div class="space-y-2">
										<span
											class={`inline-flex rounded-full border px-2.5 py-1 text-xs font-medium capitalize ${model.statusTone(delivery.status)}`}
										>
											{model.statusLabel(delivery.status)}
										</span>
										<div class="text-xs text-muted-foreground">
											<p>Attempt #{delivery.attempt_number}</p>
											<p>HTTP {delivery.response_status_code || '—'}</p>
										</div>
									</div>
								</Table.Cell>
								<Table.Cell class="min-w-52">
									<div class="space-y-1">
										<p class="font-mono text-xs break-all whitespace-normal">
											{delivery.resolved_url || '—'}
										</p>
										<p class="text-xs break-all whitespace-normal text-muted-foreground">
											Auth {delivery.auth_type || '—'} via {delivery.secret_source || '—'}
										</p>
									</div>
								</Table.Cell>
								<Table.Cell class="min-w-48">
									<div class="space-y-1 text-xs text-muted-foreground">
										<p>
											Last attempt {formatDate(delivery.last_attempt_at || delivery.created_at)}
										</p>
										{#if delivery.completed_at}
											<p>Completed {timeAgo(delivery.completed_at)}</p>
										{/if}
										{#if delivery.next_retry_at}
											<p class="text-blue-300">Next retry {formatDate(delivery.next_retry_at)}</p>
										{/if}
										{#if delivery.replayed_at}
											<p class="text-amber-300">Replayed {formatDate(delivery.replayed_at)}</p>
										{/if}
									</div>
								</Table.Cell>
								<Table.Cell class="max-w-80">
									<div class="space-y-1 text-xs">
										{#if delivery.error}
											<p class="break-words whitespace-normal text-red-300">
												{model.preview(delivery.error, 160)}
											</p>
										{:else if delivery.response_body}
											<p class="break-words whitespace-normal text-muted-foreground">
												{model.preview(delivery.response_body, 160)}
											</p>
										{:else if delivery.next_retry_at}
											<p class="text-blue-300">Waiting for retry window.</p>
										{:else if model.isSuppressed(delivery.status)}
											<p class="text-amber-300">Suppressed until an operator resumes delivery.</p>
										{:else if model.isSucceeded(delivery.status)}
											<p class="text-emerald-300">Receiver accepted this delivery.</p>
										{:else}
											<p class="text-muted-foreground">No receiver note recorded.</p>
										{/if}
										<div class="pt-1">
											<Button.Root
												variant="outline"
												size="sm"
												class="h-8 text-xs"
												onclick={() => model.openDeliveryDetails(delivery)}
											>
												View details
											</Button.Root>
										</div>
									</div>
								</Table.Cell>
							</Table.Row>
						{/each}
					{/if}
				</Table.Body>
			</Table.Root>
		</Card.Root>
	{/if}

	<DeliveryDetails {model} />
</div>

<Dialog.Root
	bind:open={model.ui.showReplay}
	onOpenChange={(open) => {
		if (model.replaying && !open) model.ui.showReplay = true;
	}}
>
	<Dialog.Content showCloseButton={!model.replaying}>
		<RequestError message={model.ui.replayError} />
		<Dialog.Header
			><Dialog.Title>Replay suppressed deliveries?</Dialog.Title><Dialog.Description
				>Previously suppressed events will be sent again in order. Your receiver may receive delayed
				or duplicate events.</Dialog.Description
			></Dialog.Header
		>
		<Dialog.Footer
			><Button.Root
				variant="outline"
				disabled={model.replaying}
				onclick={() => (model.ui.showReplay = false)}>Cancel</Button.Root
			><Button.Root
				variant="destructive"
				disabled={model.replaying || model.busy}
				loading={model.replaying}
				onclick={async () => {
					model.ui.replayError = '';
					try {
						await model.replayMutation.mutateAsync({
							name: 'replay',
							run: async () => {
								await model.onResumeReplay();
							}
						});
						model.ui.showReplay = false;
					} catch (e) {
						model.ui.replayError = e instanceof Error ? e.message : 'Replay could not be requested';
					}
				}}>Replay deliveries</Button.Root
			></Dialog.Footer
		>
	</Dialog.Content>
</Dialog.Root>
