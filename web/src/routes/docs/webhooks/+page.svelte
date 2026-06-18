<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import { webhookDocsHref, webhookEventDocs, webhookSystemEventDocs } from '$lib/webhooks';
	import { BookOpenText, ExternalLink } from '@lucide/svelte';

	type FieldContract = {
		field: string;
		type: string;
		meaning: string;
	};

	const commonEnvelopeFields: FieldContract[] = [
		{
			field: 'schema_version',
			type: 'string',
			meaning: 'Webhook payload contract version. The current public contract is v1.'
		},
		{
			field: 'event_id',
			type: 'string',
			meaning: 'Stable business-event identifier. Use this for idempotency and deduplication.'
		},
		{
			field: 'event_type',
			type: 'string',
			meaning: 'Event name such as watcher.version_found. Matches the X-Watcher-Event header.'
		},
		{
			field: 'occurred_at',
			type: 'RFC3339 date-time string',
			meaning: 'When Watcher created the event, not when your endpoint received it.'
		},
		{
			field: 'watcher.id',
			type: 'integer',
			meaning: 'Internal ID of the originating watcher inside this Watcher installation.'
		},
		{
			field: 'watcher.name',
			type: 'string',
			meaning: 'Human-facing watcher name configured in Watcher.'
		},
		{
			field: 'summary',
			type: 'string',
			meaning: 'Human-readable summary for logs and UI. Do not automate from this text.'
		}
	];

	const versionFoundFields: FieldContract[] = [
		{
			field: 'version.discovered_version',
			type: 'string',
			meaning: 'The newer remote version string that Watcher found during polling.'
		},
		{
			field: 'version.current_version',
			type: 'string',
			meaning: 'The watcher current active version when the event was emitted.'
		},
		{
			field: 'version.will_deploy',
			type: 'boolean',
			meaning: 'Whether Watcher intends to continue toward deployment after discovery.'
		},
		{
			field: 'version.block_reason',
			type: 'string',
			meaning: 'Operator-facing explanation for why deployment will not proceed when will_deploy is false.'
		}
	];
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
			This page explains how to integrate your webhook receiver with Watcher from setup through testing. The repo markdown remains the canonical reference.
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
			<a href="/webhooks.openapi.yaml" target="_blank" rel="noopener noreferrer">
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
			<p><span class="font-medium text-foreground">HTTP endpoint:</span> expose a <code>POST</code> endpoint reachable by the Watcher host.</p>
			<p><span class="font-medium text-foreground">JSON body:</span> accept <code>application/json</code> request bodies and parse nested typed objects.</p>
			<p><span class="font-medium text-foreground">Success response:</span> return any <code>2xx</code> response after accepting the event.</p>
			<p><span class="font-medium text-foreground">Idempotency:</span> deduplicate using <code>event_id</code> because delivery is at-least-once.</p>
			<p><span class="font-medium text-foreground">Auth handling:</span> optionally validate a bearer token if you configure one in Watcher.</p>
			<p><span class="font-medium text-foreground">Operational visibility:</span> log or store <code>event_type</code>, <code>event_id</code>, and <code>delivery_id</code> so you can trace retries and incidents.</p>
		</Card.Content>
	</Card.Root>

	<Card.Root class="border-border bg-card">
		<Card.Header>
			<Card.Title>Headers And Auth</Card.Title>
			<Card.Description>
				Watcher adds custom request metadata that your receiver can inspect.
			</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-4 text-sm text-muted-foreground">
			<div class="rounded-md border border-border/70 bg-muted/20 p-4">
				<p class="font-medium text-foreground">Custom headers</p>
				<ul class="mt-2 space-y-1">
					<li><code>Content-Type: application/json</code></li>
					<li><code>X-Watcher-Event</code>: event type name such as <code>watcher.deployment_failed</code></li>
					<li><code>X-Watcher-Delivery-ID</code>: delivery attempt identifier for this HTTP request</li>
				</ul>
			</div>
			<div class="rounded-md border border-border/70 bg-muted/20 p-4">
				<p class="font-medium text-foreground">Authorization behavior</p>
				<ul class="mt-2 space-y-1">
					<li>If no bearer token is configured, Watcher sends no <code>Authorization</code> header.</li>
					<li>If a token is configured, Watcher sends <code>Authorization: Bearer &lt;token&gt;</code>.</li>
					<li>The token can come from the global default or a watcher-specific override.</li>
					<li>If your endpoint requires auth, configure the same token on the watcher or in global settings.</li>
				</ul>
			</div>
		</Card.Content>
	</Card.Root>

	<Card.Root class="border-border bg-card">
		<Card.Header>
			<Card.Title>Field Meaning And Types</Card.Title>
			<Card.Description>
				Field-name lists are only a summary. Use these contracts when you need to know the actual type and intended meaning.
			</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-6">
			<div class="space-y-3">
				<p class="font-medium text-foreground">Shared envelope</p>
				<div class="overflow-x-auto rounded-md border border-border/70">
					<table class="w-full text-sm">
						<thead class="bg-muted/30 text-left text-muted-foreground">
							<tr>
								<th class="px-3 py-2 font-medium">Field</th>
								<th class="px-3 py-2 font-medium">Type</th>
								<th class="px-3 py-2 font-medium">Meaning</th>
							</tr>
						</thead>
						<tbody>
							{#each commonEnvelopeFields as field (field.field)}
								<tr class="border-t border-border/60 align-top">
									<td class="px-3 py-2 font-mono text-xs text-foreground">{field.field}</td>
									<td class="px-3 py-2 text-muted-foreground">{field.type}</td>
									<td class="px-3 py-2 text-muted-foreground">{field.meaning}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>

			<div class="space-y-3">
				<p class="font-medium text-foreground">`watcher.version_found` contract</p>
				<p class="text-sm text-muted-foreground">
					This is a discovery event. It means Watcher saw a newer remote version than the current one. It does not, by itself, guarantee that deployment started.
				</p>
				<div class="overflow-x-auto rounded-md border border-border/70">
					<table class="w-full text-sm">
						<thead class="bg-muted/30 text-left text-muted-foreground">
							<tr>
								<th class="px-3 py-2 font-medium">Field</th>
								<th class="px-3 py-2 font-medium">Type</th>
								<th class="px-3 py-2 font-medium">Meaning</th>
							</tr>
						</thead>
						<tbody>
							{#each versionFoundFields as field (field.field)}
								<tr class="border-t border-border/60 align-top">
									<td class="px-3 py-2 font-mono text-xs text-foreground">{field.field}</td>
									<td class="px-3 py-2 text-muted-foreground">{field.type}</td>
									<td class="px-3 py-2 text-muted-foreground">{field.meaning}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
				<div class="rounded-md border border-border/70 bg-muted/20 p-4 text-sm text-muted-foreground">
					<p><span class="font-medium text-foreground">Interpretation:</span> `will_deploy=true` means Watcher can continue toward deployment after discovery. `will_deploy=false` means the discovery is real, but rollout is blocked and `block_reason` explains why.</p>
				</div>
			</div>
		</Card.Content>
	</Card.Root>

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
					Create an HTTP <code>POST</code> endpoint that accepts JSON, optional bearer authentication, and returns a <code>2xx</code> response when the payload is accepted.
				</p>
			</div>
			<div>
				<p class="font-medium text-foreground">2. Make the receiver idempotent</p>
				<p class="mt-1">
					Watcher delivers webhooks with at-least-once semantics. Deduplicate by <code>event_id</code> and treat <code>delivery_id</code> as the attempt identifier.
				</p>
			</div>
			<div>
				<p class="font-medium text-foreground">3. Configure the watcher</p>
				<p class="mt-1">
					Open the watcher’s webhook settings, enable delivery, set the URL or inherit the global default, decide whether to use a watcher-specific bearer token, and choose which business events should be emitted.
				</p>
			</div>
			<div>
				<p class="font-medium text-foreground">4. Know the headers and payload contract</p>
				<p class="mt-1">
					Watcher sends <code>Content-Type: application/json</code>, <code>X-Watcher-Event</code>, <code>X-Watcher-Delivery-ID</code>, and optionally <code>Authorization: Bearer ...</code>. Payload schemas are documented below, in the repo docs, and in the OpenAPI file.
				</p>
			</div>
			<div>
				<p class="font-medium text-foreground">5. Test before real events happen</p>
				<p class="mt-1">
					Use <code>Send Test Webhook</code> from the watcher edit page or the watcher webhook history tab. It uses the same outbox and retry pipeline as real events.
				</p>
			</div>
			<div>
				<p class="font-medium text-foreground">6. Watch delivery history</p>
				<p class="mt-1">
					Check the watcher webhook tab or the webhook hub to see whether deliveries are succeeding, retrying, or paused.
				</p>
			</div>
			<div>
				<p class="font-medium text-foreground">7. Handle pauses and replay intentionally</p>
				<p class="mt-1">
					If the endpoint keeps failing, Watcher can pause delivery. Resume only continues future events. Resume with replay moves suppressed events back to pending in normal FIFO order.
				</p>
			</div>
		</Card.Content>
	</Card.Root>

	<Card.Root class="border-border bg-card">
		<Card.Header>
			<Card.Title>Event Payload Shapes</Card.Title>
			<Card.Description>
				Every event shares a common envelope, then adds event-specific nested objects.
			</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-4">
			<div class="rounded-md border border-border/70 bg-muted/20 p-4 text-sm text-muted-foreground">
				<p class="font-medium text-foreground">Common envelope</p>
				<p class="mt-2">Every webhook payload includes these top-level fields:</p>
				<ul class="mt-2 space-y-1 font-mono text-xs">
					<li>schema_version</li>
					<li>event_id</li>
					<li>event_type</li>
					<li>occurred_at</li>
					<li>watcher.id</li>
					<li>watcher.name</li>
					<li>summary</li>
				</ul>
			</div>

			<div class="space-y-2">
				<p class="font-medium text-foreground">Business events</p>
				<div class="grid gap-2">
					{#each webhookEventDocs as event (event.eventType)}
						<div class="rounded-md border border-border/70 bg-muted/20 p-3">
							<p class="font-medium">{event.name}</p>
							<p class="mt-1 font-mono text-xs text-muted-foreground">{event.eventType}</p>
							<p class="mt-2 text-sm text-muted-foreground">{event.when}</p>
							<div class="mt-3">
								<p class="text-xs font-medium uppercase tracking-wide text-foreground/80">Field contract</p>
								<div class="mt-2 overflow-x-auto rounded-md border border-border/70">
									<table class="w-full text-sm">
										<thead class="bg-muted/30 text-left text-muted-foreground">
											<tr>
												<th class="px-3 py-2 font-medium">Field</th>
												<th class="px-3 py-2 font-medium">Type</th>
												<th class="px-3 py-2 font-medium">Meaning</th>
											</tr>
										</thead>
										<tbody>
											{#each event.fields as field (field.field)}
												<tr class="border-t border-border/60 align-top">
													<td class="px-3 py-2 font-mono text-xs text-foreground">{field.field}</td>
													<td class="px-3 py-2 text-xs text-muted-foreground">{field.type}</td>
													<td class="px-3 py-2 text-xs text-muted-foreground">{field.meaning}</td>
												</tr>
											{/each}
										</tbody>
									</table>
								</div>
								{#if event.interpretation}
									<div class="mt-3 rounded-md border border-border/70 bg-background/60 p-3 text-sm text-muted-foreground">
										<p class="font-medium text-foreground">Interpretation</p>
										<ul class="mt-2 space-y-1">
											{#each event.interpretation as item (item)}
												<li>{item}</li>
											{/each}
										</ul>
									</div>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			</div>
			<div class="space-y-2">
				<p class="font-medium text-foreground">System events</p>
				<div class="grid gap-2">
					{#each webhookSystemEventDocs as event (event.eventType)}
						<div class="rounded-md border border-border/70 bg-muted/20 p-3">
							<p class="font-medium">{event.name}</p>
							<p class="mt-1 font-mono text-xs text-muted-foreground">{event.eventType}</p>
							<p class="mt-2 text-sm text-muted-foreground">{event.when}</p>
							<div class="mt-3">
								<p class="text-xs font-medium uppercase tracking-wide text-foreground/80">Field contract</p>
								<div class="mt-2 overflow-x-auto rounded-md border border-border/70">
									<table class="w-full text-sm">
										<thead class="bg-muted/30 text-left text-muted-foreground">
											<tr>
												<th class="px-3 py-2 font-medium">Field</th>
												<th class="px-3 py-2 font-medium">Type</th>
												<th class="px-3 py-2 font-medium">Meaning</th>
											</tr>
										</thead>
										<tbody>
											{#each event.fields as field (field.field)}
												<tr class="border-t border-border/60 align-top">
													<td class="px-3 py-2 font-mono text-xs text-foreground">{field.field}</td>
													<td class="px-3 py-2 text-xs text-muted-foreground">{field.type}</td>
													<td class="px-3 py-2 text-xs text-muted-foreground">{field.meaning}</td>
												</tr>
											{/each}
										</tbody>
									</table>
								</div>
								{#if event.interpretation}
									<div class="mt-3 rounded-md border border-border/70 bg-background/60 p-3 text-sm text-muted-foreground">
										<p class="font-medium text-foreground">Interpretation</p>
										<ul class="mt-2 space-y-1">
											{#each event.interpretation as item (item)}
												<li>{item}</li>
											{/each}
										</ul>
									</div>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			</div>
		</Card.Content>
	</Card.Root>

	<Card.Root class="border-border bg-card">
		<Card.Header>
			<Card.Title>When things go wrong</Card.Title>
			<Card.Description>
				Typical integration failure modes.
			</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-3 text-sm text-muted-foreground">
			<p><span class="font-medium text-foreground">401 or 403:</span> bearer token mismatch or wrong endpoint authorization expectations.</p>
			<p><span class="font-medium text-foreground">400:</span> your receiver rejected the payload shape. Check the repo docs and the OpenAPI schema.</p>
			<p><span class="font-medium text-foreground">429 or 5xx:</span> Watcher retries automatically using the configured retry schedule.</p>
			<p><span class="font-medium text-foreground">Paused delivery:</span> the watcher hit its consecutive failure threshold. Resume from the watcher webhook controls after fixing the endpoint.</p>
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
