<script lang="ts">
	import { Checkbox } from '$lib/components/ui/checkbox';
	import Badge from '$lib/components/ui/badge/badge.svelte';
	import { ExternalLink } from '@lucide/svelte';
	import * as Card from '$lib/components/ui/card';
	import {
		webhookDeliveryNotes,
		webhookEventDocs,
		webhookSystemEventDocs,
		type WebhookSelectionState
	} from '$lib/webhooks';

	let {
		title = 'Webhook Event Reference',
		description = 'What each webhook means, when it fires, and what payload fields receivers should expect.',
		selections = $bindable<WebhookSelectionState | null>(null),
		showSelection = false
	}: {
		title?: string;
		description?: string;
		selections?: WebhookSelectionState | null;
		showSelection?: boolean;
	} = $props();
</script>

<Card.Root class="border-border/70 bg-background/40">
	<Card.Header class="pb-3">
		<Card.Title class="text-base">{title}</Card.Title>
		<Card.Description>{description}</Card.Description>
	</Card.Header>
	<Card.Content class="space-y-5">
		<div class="rounded-md border border-border/70 bg-muted/20 p-4">
			<h4 class="text-sm font-medium">Delivery Behavior</h4>
			<ul class="mt-2 space-y-1 text-sm text-muted-foreground">
				{#each webhookDeliveryNotes as note (note)}
					<li>{note}</li>
				{/each}
			</ul>
		</div>

		<div class="space-y-3">
			<h4 class="text-sm font-medium">Subscribed Business Events</h4>
			{#each webhookEventDocs as event (event.eventType)}
				<div class="rounded-md border border-border/70 bg-card/60 p-4">
					<div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
						<div class="space-y-1">
							<div class="flex items-center gap-2">
								<h5 class="font-medium">{event.name}</h5>
								<Badge variant="secondary" class="font-mono text-[11px]">
									{event.eventType}
								</Badge>
								<a
									href="https://github.com/fanboykun/watcher/blob/main/docs/webhooks.md#{event.anchor}"
									target="_blank"
									rel="noopener noreferrer"
									class="text-muted-foreground hover:text-primary transition-colors"
									title="View in Repository Documentation"
								>
									<ExternalLink class="h-3 w-3" />
								</a>
							</div>
							<p class="text-sm text-muted-foreground">{event.when}</p>
						</div>
						{#if showSelection && selections && event.key}
							<label class="flex items-center gap-2 text-sm">
								<Checkbox
									bind:checked={selections[event.key]}
								/>
								<span>Send this event</span>
							</label>
						{/if}
					</div>
					<div class="mt-4 grid gap-4 lg:grid-cols-2">
						<div>
							<h6 class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
								Behavior
							</h6>
							<ul class="mt-2 space-y-1 text-sm text-muted-foreground">
								{#each event.behavior as item (item)}
									<li>{item}</li>
								{/each}
							</ul>
						</div>
						<div>
							<h6 class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
								Payload Highlights
							</h6>
							<ul class="mt-2 space-y-1 font-mono text-xs text-muted-foreground">
								{#each event.payload as field (field)}
									<li>{field}</li>
								{/each}
							</ul>
						</div>
					</div>
				</div>
			{/each}
		</div>

		<div class="space-y-3">
			<h4 class="text-sm font-medium">System Events</h4>
			<p class="text-sm text-muted-foreground">
				These are produced by Watcher itself and are not controlled by the business-event checkboxes.
			</p>
			{#each webhookSystemEventDocs as event (event.eventType)}
				<div class="rounded-md border border-border/70 bg-card/60 p-4">
					<div class="space-y-1">
						<div class="flex items-center gap-2">
							<h5 class="font-medium">{event.name}</h5>
							<Badge variant="secondary" class="font-mono text-[11px]">
								{event.eventType}
							</Badge>
							<a
								href="https://github.com/fanboykun/watcher/blob/main/docs/webhooks.md#{event.anchor}"
								target="_blank"
								rel="noopener noreferrer"
								class="text-muted-foreground hover:text-primary transition-colors"
								title="View in Repository Documentation"
							>
								<ExternalLink class="h-3 w-3" />
							</a>
						</div>
						<p class="text-sm text-muted-foreground">{event.when}</p>
					</div>
					<div class="mt-4 grid gap-4 lg:grid-cols-2">
						<div>
							<h6 class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
								Behavior
							</h6>
							<ul class="mt-2 space-y-1 text-sm text-muted-foreground">
								{#each event.behavior as item (item)}
									<li>{item}</li>
								{/each}
							</ul>
						</div>
						<div>
							<h6 class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
								Payload Highlights
							</h6>
							<ul class="mt-2 space-y-1 font-mono text-xs text-muted-foreground">
								{#each event.payload as field (field)}
									<li>{field}</li>
								{/each}
							</ul>
						</div>
					</div>
				</div>
			{/each}
		</div>
	</Card.Content>
</Card.Root>
