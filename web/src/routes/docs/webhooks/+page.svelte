<script lang="ts">
	import EventPayloads from '$lib/features/webhooks/reference/event-payloads.svelte';
	import DeliveryFlow from '$lib/features/webhooks/reference/delivery-flow.svelte';
	import FieldContracts from '$lib/features/webhooks/reference/field-contracts.svelte';
	import SigningHeaders from '$lib/features/webhooks/reference/signing-headers.svelte';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import { webhookDocsHref, webhookOpenAPISpecHref } from '$lib/webhooks';
	import { BookOpenText, ExternalLink } from '@lucide/svelte';

	type FieldContract = {
		field: string;
		type: string;
		meaning: string;
	};
</script>

<svelte:head>
	<title>Webhook Docs | Watcher</title>
</svelte:head>

<div class="space-y-6">
	<div>
		<div class="inline-flex items-center gap-2 text-sm text-muted-foreground">
			<BookOpenText class="h-4 w-4" />
			Integration guide
		</div>
		<h1 class="mt-2 text-2xl font-bold tracking-tight">Webhook Integration</h1>
		<p class="mt-2 max-w-3xl text-sm text-muted-foreground">
			This page explains how to integrate your webhook receiver with Watcher from setup through
			testing. The repo markdown remains the canonical reference.
		</p>
		<div class="mt-3 flex flex-wrap gap-2">
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
			<a href={webhookDocsHref} target="_blank" rel="noopener noreferrer">
				<Button.Root size="sm" variant="outline">
					<ExternalLink class="mr-2 h-4 w-4" />
					Repo Docs
				</Button.Root>
			</a>
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
			<a href={webhookOpenAPISpecHref} target="_blank" rel="noopener noreferrer">
				<Button.Root size="sm" variant="outline">OpenAPI Spec</Button.Root>
			</a>
		</div>
	</div>

	<Card.Root class="border-border bg-card">
		<Card.Header>
			<Card.Title>Receiver Requirements</Card.Title>
			<Card.Description>
				What your webhook service should support before you point a watcher at it.
			</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-3 text-sm text-muted-foreground">
			<p>
				<span class="font-medium text-foreground">HTTP endpoint:</span> expose a <code>POST</code> endpoint
				reachable by the Watcher host.
			</p>
			<p>
				<span class="font-medium text-foreground">JSON body:</span> accept
				<code>application/json</code> request bodies and parse nested typed objects.
			</p>
			<p>
				<span class="font-medium text-foreground">Success response:</span> return any
				<code>2xx</code> response after accepting the event.
			</p>
			<p>
				<span class="font-medium text-foreground">Idempotency:</span> deduplicate using
				<code>event_id</code> because delivery is at-least-once.
			</p>
			<p>
				<span class="font-medium text-foreground">Auth handling:</span> verify the Standard Webhooks HMAC
				signature using the shared signing secret you configured in Watcher.
			</p>
			<p>
				<span class="font-medium text-foreground">Operational visibility:</span> log or store
				<code>type</code>, <code>event_id</code>, <code>webhook-id</code>, and
				<code>X-Watcher-Delivery-ID</code> so you can trace retries and incidents.
			</p>
		</Card.Content>
	</Card.Root>

	<SigningHeaders />

	<Card.Root class="border-border bg-card">
		<Card.Header>
			<Card.Title>Signing Secret Format</Card.Title>
			<Card.Description>
				What the signing secret looks like, where to get it, and how both sides should use it.
			</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-3 text-sm text-muted-foreground">
			<p>
				<span class="font-medium text-foreground">Format:</span> use high-entropy base64 secret
				material. The conventional <code>whsec_</code> prefix is allowed but not required.
			</p>
			<p>
				<span class="font-medium text-foreground">Generate it:</span> create a new unpredictable secret,
				store it in your receiver first, then copy the exact same value into Watcher.
			</p>
			<p>
				<span class="font-medium text-foreground">Watcher side:</span> Watcher decodes the secret,
				signs the raw request body, and sends <code>webhook-id</code>,
				<code>webhook-timestamp</code>, and <code>webhook-signature</code>.
			</p>
			<p>
				<span class="font-medium text-foreground">Receiver side:</span> verify the raw body with the
				exact same secret before trusting the payload. If the secret or body differs, verification
				fails with <code>invalid signature</code>.
			</p>
		</Card.Content>
	</Card.Root>

	<FieldContracts />

	<DeliveryFlow />

	<Card.Root class="border-border bg-card">
		<Card.Header>
			<Card.Title>Secret Setup And Rotation</Card.Title>
			<Card.Description>
				How to manage the webhook signing secret safely from first setup through rotation.
			</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-3 text-sm text-muted-foreground">
			<p>
				<span class="font-medium text-foreground">Generate once, store twice:</span> create the secret
				in your receiver secret store first, then copy the same value into Watcher.
			</p>
			<p>
				<span class="font-medium text-foreground">Scope secrets narrowly:</span> prefer one secret per
				environment or endpoint instead of one shared secret everywhere.
			</p>
			<p>
				<span class="font-medium text-foreground">Keep secrets out of logs:</span> Watcher only exposes
				masked secret presence. Your receiver should do the same.
			</p>
			<p>
				<span class="font-medium text-foreground">Rotate carefully:</span> add the new secret to the
				receiver, update Watcher, send <code>watcher.webhook_test</code>, verify success, then
				remove the old secret.
			</p>
			<p>
				<span class="font-medium text-foreground">Current limitation:</span> Watcher currently signs with
				one active HMAC secret at a time and does not yet emit multiple signatures for zero-downtime rotation.
			</p>
		</Card.Content>
	</Card.Root>

	<Card.Root class="border-border bg-card">
		<Card.Header>
			<Card.Title>Migration Notes</Card.Title>
			<Card.Description>What existing Watcher webhook consumers need to change.</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-3 text-sm text-muted-foreground">
			<p>
				<span class="font-medium text-foreground">Replace bearer auth assumptions:</span> Watcher no
				longer expects receivers to verify <code>Authorization: Bearer ...</code> for outbound webhooks.
			</p>
			<p>
				<span class="font-medium text-foreground">Add standard verification:</span> verify
				<code>webhook-id</code>, <code>webhook-timestamp</code>, and <code>webhook-signature</code> with
				the shared signing secret.
			</p>
			<p>
				<span class="font-medium text-foreground">Prefer standard fields for new code:</span> use
				<code>type</code>, <code>timestamp</code>, and <code>data</code> first.
			</p>
			<p>
				<span class="font-medium text-foreground">Compatibility remains:</span> Watcher still
				includes the legacy convenience fields like <code>event_id</code>, <code>event_type</code>,
				and <code>occurred_at</code> during the transition.
			</p>
		</Card.Content>
	</Card.Root>

	<Card.Root class="border-border bg-card">
		<Card.Header>
			<Card.Title>Production Checklist</Card.Title>
			<Card.Description>
				Minimum standards before you consider the integration shipped.
			</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-2 text-sm text-muted-foreground">
			<p>1. Verify signatures against the raw request body, not re-serialized JSON.</p>
			<p>2. Deduplicate on <code>event_id</code>, not <code>X-Watcher-Delivery-ID</code>.</p>
			<p>3. Return <code>2xx</code> only after your receiver has durably accepted the event.</p>
			<p>
				4. Log <code>event_id</code>, <code>type</code>, and <code>X-Watcher-Delivery-ID</code> for incident
				tracing.
			</p>
			<p>
				5. Rehearse pause, resume, and replay handling before relying on the integration
				operationally.
			</p>
			<p>6. Rehearse secret rotation in a non-production environment.</p>
		</Card.Content>
	</Card.Root>

	<EventPayloads />

	<Card.Root class="border-border bg-card">
		<Card.Header>
			<Card.Title>When things go wrong</Card.Title>
			<Card.Description>Typical integration failure modes.</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-3 text-sm text-muted-foreground">
			<p>
				<span class="font-medium text-foreground">401 or 403:</span> signing secret mismatch, stale timestamp
				rejection, or wrong endpoint authorization expectations.
			</p>
			<p>
				<span class="font-medium text-foreground">400:</span> your receiver rejected the payload shape.
				Check the repo docs and the OpenAPI schema.
			</p>
			<p>
				<span class="font-medium text-foreground">429 or 5xx:</span> Watcher retries automatically using
				the configured retry schedule.
			</p>
			<p>
				<span class="font-medium text-foreground">Paused delivery:</span> the watcher hit its consecutive
				failure threshold. Resume from the watcher webhook controls after fixing the endpoint.
			</p>
		</Card.Content>
	</Card.Root>

	<Card.Root class="border-border bg-card">
		<Card.Header>
			<Card.Title>Canonical Reference</Card.Title>
			<Card.Description>
				Use the repo markdown when you need the exact contract wording and detailed examples.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
			<a href={webhookDocsHref} target="_blank" rel="noopener noreferrer">
				<Button.Root>
					<ExternalLink class="mr-2 h-4 w-4" />
					Open docs/webhooks.md
				</Button.Root>
			</a>
		</Card.Content>
	</Card.Root>
</div>
