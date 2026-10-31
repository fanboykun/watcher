<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import RequestError from '$lib/components/request-error.svelte';
	import { createGitHubQuota } from './github-quota-controller.svelte';
	import { quotaSourceLabel, quotaUsage, quotaView } from './github-quota';

	const quota = createGitHubQuota();
	const toneClass = {
		ok: 'text-emerald-400',
		warn: 'text-amber-400',
		unknown: 'text-muted-foreground'
	};
</script>

<Card.Root class="mt-4">
	<Card.Header>
		<Card.Title>GitHub API quota</Card.Title>
		<Card.Description>
			Shared by every watcher using the same token. Unchanged polls are revalidated and cost no
			quota.
		</Card.Description>
	</Card.Header>
	<Card.Content>
		<RequestError message={quota.error} />
		{#if quota.loading}
			<p class="text-sm text-muted-foreground">Loading quota…</p>
		{:else}
			<ul class="divide-y divide-border">
				{#each quota.entries as entry (`${entry.source}:${entry.watcher_ids.join(',')}`)}
					{@const view = quotaView(entry)}
					<li class="flex flex-wrap items-start justify-between gap-2 py-3 first:pt-0 last:pb-0">
						<div class="min-w-0">
							<p class="text-sm font-medium">{quotaSourceLabel(entry)}</p>
							<p class="text-xs text-muted-foreground">{quotaUsage(entry)}</p>
						</div>
						<div class="min-w-0 text-right">
							<p class={`text-sm font-medium ${toneClass[view.tone]}`}>{view.label}</p>
							<p class="text-xs text-muted-foreground">{view.detail}</p>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	</Card.Content>
</Card.Root>
