<script lang="ts">
	import * as Card from '$lib/components/ui/card';

	import { webhookEventDocs, webhookSystemEventDocs } from '$lib/webhooks';

	type FieldContract = {
		field: string;
		type: string;
		meaning: string;
	};
</script>

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
							<p class="text-xs font-medium tracking-wide text-foreground/80 uppercase">
								Field contract
							</p>
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
								<div
									class="mt-3 rounded-md border border-border/70 bg-background/60 p-3 text-sm text-muted-foreground"
								>
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
							<p class="text-xs font-medium tracking-wide text-foreground/80 uppercase">
								Field contract
							</p>
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
								<div
									class="mt-3 rounded-md border border-border/70 bg-background/60 p-3 text-sm text-muted-foreground"
								>
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
