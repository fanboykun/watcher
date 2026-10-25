<script lang="ts">
	import * as Card from '$lib/components/ui/card';

	type FieldContract = {
		field: string;
		type: string;
		meaning: string;
	};
</script>

<Card.Root class="border-border bg-card">
	<Card.Header>
		<Card.Title>Start to End</Card.Title>
		<Card.Description>
			What a developer integrating a webhook receiver usually needs to do.
		</Card.Description>
	</Card.Header>
	<Card.Content class="space-y-4 text-sm text-muted-foreground">
		<div>
			<p class="font-medium text-foreground">1. Build your receiver endpoint</p>
			<p class="mt-1">
				Create an HTTP <code>POST</code> endpoint that accepts JSON, verifies the Standard Webhooks
				signature, and returns a <code>2xx</code> response when the payload is accepted.
			</p>
		</div>
		<div>
			<p class="font-medium text-foreground">2. Make the receiver idempotent</p>
			<p class="mt-1">
				Watcher delivers webhooks with at-least-once semantics. Deduplicate by <code>event_id</code>
				and treat <code>delivery_id</code> as the attempt identifier.
			</p>
		</div>
		<div>
			<p class="font-medium text-foreground">3. Configure the watcher</p>
			<p class="mt-1">
				Open the watcher’s webhook settings, enable delivery, set the URL or inherit the global
				default, decide whether to use a watcher-specific signing secret, and choose which business
				events should be emitted.
			</p>
		</div>
		<div>
			<p class="font-medium text-foreground">4. Know the headers and payload contract</p>
			<p class="mt-1">
				Watcher sends <code>Content-Type: application/json</code>, the Standard Webhooks signature
				headers, and Watcher’s extra observability headers. Payload schemas are documented below, in
				the repo docs, and in the OpenAPI file.
			</p>
		</div>
		<div>
			<p class="font-medium text-foreground">5. Test before real events happen</p>
			<p class="mt-1">
				Use <code>Send Test Webhook</code> from the watcher edit page or the watcher webhook history tab.
				It uses the same outbox and retry pipeline as real events.
			</p>
		</div>
		<div>
			<p class="font-medium text-foreground">6. Watch delivery history</p>
			<p class="mt-1">
				Check the watcher webhook tab or the webhook hub to see whether deliveries are succeeding,
				retrying, or paused.
			</p>
		</div>
		<div>
			<p class="font-medium text-foreground">7. Handle pauses and replay intentionally</p>
			<p class="mt-1">
				If the endpoint keeps failing, Watcher can pause delivery. Resume only continues future
				events. Resume with replay moves suppressed events back to pending in normal FIFO order.
			</p>
		</div>
	</Card.Content>
</Card.Root>
