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
		<Card.Title>Headers And Auth</Card.Title>
		<Card.Description>
			Watcher adds custom request metadata that your receiver can inspect.
		</Card.Description>
	</Card.Header>
	<Card.Content class="space-y-4 text-sm text-muted-foreground">
		<div class="rounded-md border border-border/70 bg-muted/20 p-4">
			<p class="font-medium text-foreground">Standard headers</p>
			<ul class="mt-2 space-y-1">
				<li><code>Content-Type: application/json</code></li>
				<li>
					<code>webhook-id</code>: stable event identifier used for idempotency across retries
				</li>
				<li>
					<code>webhook-timestamp</code>: Unix timestamp in seconds for the delivery attempt
				</li>
				<li>
					<code>webhook-signature</code>: Standard Webhooks HMAC-SHA256 signature for the exact
					request body
				</li>
				<li>
					<code>X-Watcher-Event</code>: event type name such as
					<code>watcher.deployment_failed</code>
				</li>
				<li>
					<code>X-Watcher-Delivery-ID</code>: delivery attempt identifier for this HTTP request
				</li>
			</ul>
		</div>
		<div class="rounded-md border border-border/70 bg-muted/20 p-4">
			<p class="font-medium text-foreground">Signature behavior</p>
			<ul class="mt-2 space-y-1">
				<li>
					Watcher signs the literal request payload bytes using Standard Webhooks HMAC-SHA256.
				</li>
				<li>The signing secret can come from the global default or a watcher-specific override.</li>
				<li>Use the same signing secret on your receiver when verifying the request.</li>
				<li>
					The signature covers <code>webhook-id</code>, <code>webhook-timestamp</code>, and the raw
					request body.
				</li>
			</ul>
		</div>
	</Card.Content>
</Card.Root>
