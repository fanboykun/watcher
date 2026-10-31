<script lang="ts">
	import * as Card from '$lib/components/ui/card';

	type FieldContract = {
		field: string;
		type: string;
		meaning: string;
	};

	const commonEnvelopeFields: FieldContract[] = [
		{
			field: 'type',
			type: 'string',
			meaning:
				'Standard Webhooks event type. Matches webhook payload semantics and the legacy event_type field.'
		},
		{
			field: 'timestamp',
			type: 'RFC3339 date-time string',
			meaning: 'When the business event occurred. Matches the legacy occurred_at field.'
		},
		{
			field: 'data',
			type: 'object',
			meaning:
				'Standard Webhooks event data envelope. Includes watcher, summary, event_id, and event-specific nested objects.'
		},
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
			meaning:
				'Operator-facing explanation for why deployment will not proceed when will_deploy is false.'
		}
	];
</script>

<Card.Root class="border-border bg-card">
	<Card.Header>
		<Card.Title>Field Meaning And Types</Card.Title>
		<Card.Description>
			Field-name lists are only a summary. Use these contracts when you need to know the actual type
			and intended meaning.
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
				This is a discovery event. It means Watcher saw a newer remote version than the current one.
				It does not, by itself, guarantee that deployment started.
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
				<p>
					<span class="font-medium text-foreground">Interpretation:</span> `will_deploy=true` means Watcher
					can continue toward deployment after discovery. `will_deploy=false` means the discovery is real,
					but rollout is blocked and `block_reason` explains why.
				</p>
			</div>
		</div>
	</Card.Content>
</Card.Root>
