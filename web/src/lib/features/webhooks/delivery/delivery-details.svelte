<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Button from '$lib/components/ui/button';
	import * as Select from '$lib/components/ui/select/index.js';
	import { formatDate } from '$lib/utils';
	import type { createDelivery } from '$lib/features/webhooks/delivery/delivery-controller.svelte';
	let { model }: { model: ReturnType<typeof createDelivery> } = $props();
</script>

<Dialog.Root bind:open={model.ui.showDeliveryDetails}>
	<Dialog.Content class="max-h-[90vh] w-[min(1100px,95vw)] overflow-hidden sm:max-w-[1100px]">
		<Dialog.Header>
			<Dialog.Title>Webhook Delivery Details</Dialog.Title>
			<Dialog.Description>
				Inspect the exact payload, signed request metadata, and receiver response for this delivery
				attempt.
			</Dialog.Description>
		</Dialog.Header>

		<div class="max-h-[72vh] space-y-4 overflow-y-auto pr-1">
			{#if model.selectedDeliveryLoading}
				<div
					class="rounded-xl border border-border/60 bg-muted/20 p-4 text-sm text-muted-foreground"
				>
					Loading delivery details...
				</div>
			{:else if model.selectedDeliveryError}
				<div class="rounded-xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-200">
					{model.selectedDeliveryError}
				</div>
			{:else if model.selectedDeliveryDetails}
				<div class="grid gap-4 lg:grid-cols-2">
					<div class="space-y-4">
						<div class="rounded-xl border border-border/60 bg-background/40 p-4">
							<p class="text-xs tracking-[0.14em] text-muted-foreground uppercase">Delivery</p>
							<div class="mt-3 grid gap-2 text-sm">
								<p>
									<span class="text-muted-foreground">Event:</span>
									{model.selectedDeliveryDetails.delivery.event_type}
								</p>
								<p>
									<span class="text-muted-foreground">Delivery ID:</span>
									{model.selectedDeliveryDetails.delivery.delivery_id}
								</p>
								<p>
									<span class="text-muted-foreground">Status:</span>
									{model.selectedDeliveryDetails.delivery.status}
								</p>
								<p>
									<span class="text-muted-foreground">Attempt:</span> #{model
										.selectedDeliveryDetails.delivery.attempt_number}
								</p>
								<p>
									<span class="text-muted-foreground">HTTP:</span>
									{model.headerValue(model.selectedDeliveryDetails.delivery.response_status_code)}
								</p>
								<p>
									<span class="text-muted-foreground">Created:</span>
									{formatDate(model.selectedDeliveryDetails.delivery.created_at)}
								</p>
								<p>
									<span class="text-muted-foreground">Last attempt:</span>
									{model.headerValue(model.selectedDeliveryDetails.delivery.last_attempt_at)}
								</p>
								<p>
									<span class="text-muted-foreground">Completed:</span>
									{model.headerValue(model.selectedDeliveryDetails.delivery.completed_at)}
								</p>
							</div>
						</div>

						<div class="rounded-xl border border-border/60 bg-background/40 p-4">
							<p class="text-xs tracking-[0.14em] text-muted-foreground uppercase">Request</p>
							<div class="mt-3 space-y-3 text-sm">
								<p class="font-mono text-xs break-all text-muted-foreground">
									{model.selectedDeliveryDetails.request.url}
								</p>
								<p>
									<span class="text-muted-foreground">Auth:</span>
									{model.selectedDeliveryDetails.request.auth_type}
								</p>
								<p>
									<span class="text-muted-foreground">Secret source:</span>
									{model.selectedDeliveryDetails.request.secret_source}
								</p>
								<div class="space-y-1">
									<p class="text-xs tracking-[0.14em] text-muted-foreground uppercase">Headers</p>
									<pre
										class="overflow-x-auto rounded-lg border border-border/60 bg-muted/30 p-3 text-xs leading-5 text-muted-foreground">{model.prettyJson(
											model.selectedDeliveryDetails.request.headers
										)}</pre>
								</div>
							</div>
						</div>
					</div>

					<div class="space-y-4">
						<div class="rounded-xl border border-border/60 bg-background/40 p-4">
							<p class="text-xs tracking-[0.14em] text-muted-foreground uppercase">Event payload</p>
							<p class="mt-2 text-sm text-muted-foreground">
								This is the exact business event body we signed and sent to the receiver.
							</p>
							<pre
								class="mt-3 max-h-80 overflow-auto rounded-lg border border-border/60 bg-muted/30 p-3 text-xs leading-5 text-muted-foreground">{model.prettyJson(
									model.selectedDeliveryDetails.event.payload
								)}</pre>
						</div>

						<div class="rounded-xl border border-border/60 bg-background/40 p-4">
							<p class="text-xs tracking-[0.14em] text-muted-foreground uppercase">
								Receiver response
							</p>
							<div class="mt-3 space-y-2 text-sm">
								<p>
									<span class="text-muted-foreground">Response code:</span>
									{model.headerValue(model.selectedDeliveryDetails.delivery.response_status_code)}
								</p>
								<p>
									<span class="text-muted-foreground">Outcome:</span>
									{model.selectedDeliveryDetails.delivery.error ? 'Failed' : 'Accepted or pending'}
								</p>
							</div>
							{#if model.selectedDeliveryDetails.delivery.error}
								<pre
									class="mt-3 max-h-52 overflow-auto rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-xs leading-5 text-red-100">{model
										.selectedDeliveryDetails.delivery.error}</pre>
							{:else if model.selectedDeliveryDetails.delivery.response_body}
								<pre
									class="mt-3 max-h-52 overflow-auto rounded-lg border border-border/60 bg-muted/30 p-3 text-xs leading-5 text-muted-foreground">{model
										.selectedDeliveryDetails.delivery.response_body}</pre>
							{:else}
								<p class="mt-3 text-sm text-muted-foreground">No response body recorded.</p>
							{/if}
						</div>

						<div class="rounded-xl border border-amber-500/20 bg-amber-500/5 p-4">
							<p class="text-xs tracking-[0.14em] text-amber-300 uppercase">Redelivery</p>
							<p class="mt-2 text-sm text-muted-foreground">
								A per-delivery redeliver action is not wired up yet. For now, use the watcher-level
								replay controls or resend the test event after fixing the receiver.
							</p>
						</div>
					</div>
				</div>
			{:else}
				<div
					class="rounded-xl border border-border/60 bg-muted/20 p-4 text-sm text-muted-foreground"
				>
					Select a delivery to inspect its payload and response history.
				</div>
			{/if}
		</div>

		<Dialog.Footer>
			<Button.Root type="button" variant="outline" onclick={model.closeDeliveryDetails}
				>Close</Button.Root
			>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
