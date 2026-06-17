<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { api, type Watcher } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { webhookDocsHref } from '$lib/webhooks';
	import { AlertCircle, BookOpenText, ExternalLink, Plus, Webhook as WebhookIcon } from '@lucide/svelte';

	type WebhookGroup = {
		serviceName: string;
		watchers: Watcher[];
	};

	let watchers = $state<Watcher[]>([]);
	let error = $state('');
	let showAddDialog = $state(false);

	onMount(async () => {
		try {
			watchers = await api.listWatchers();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load watchers';
		}
	});

	const webhookGroups = $derived.by<WebhookGroup[]>(() => {
		const groups = new Map<string, Watcher[]>();
		for (const watcher of watchers) {
			const key = watcher.service_name || watcher.name;
			const current = groups.get(key) || [];
			current.push(watcher);
			groups.set(key, current);
		}
		return [...groups.entries()]
			.sort((a, b) => a[0].localeCompare(b[0]))
			.map(([serviceName, groupedWatchers]) => ({
				serviceName,
				watchers: groupedWatchers.sort((a, b) => a.name.localeCompare(b.name))
			}));
	});
</script>

<svelte:head>
	<title>Webhooks | Watcher</title>
</svelte:head>

<div class="space-y-6">
	<div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
		<div>
			<div class="inline-flex items-center gap-2 text-sm text-muted-foreground">
				<WebhookIcon class="h-4 w-4" />
				Webhook operations
			</div>
			<h1 class="mt-2 text-2xl font-bold tracking-tight">Webhooks</h1>
			<p class="mt-2 max-w-3xl text-sm text-muted-foreground">
				Configured watcher webhooks grouped by service. Full event and payload documentation lives in the repo docs.
			</p>
			<div class="mt-3 flex flex-wrap gap-2">
				<a href="/docs/webhooks">
					<Button.Root size="sm" variant="outline">
						<BookOpenText class="mr-2 h-4 w-4" />
						Integration Guide
					</Button.Root>
				</a>
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
				<a href={webhookDocsHref} target="_blank" rel="noopener noreferrer">
					<Button.Root size="sm" variant="outline">
						<ExternalLink class="mr-2 h-4 w-4" />
						Repo Docs
					</Button.Root>
				</a>
			</div>
		</div>
		<Button.Root onclick={() => (showAddDialog = true)}>
				<Plus class="mr-2 h-4 w-4" />
				Add Webhook
		</Button.Root>
	</div>

	{#if error}
		<div class="flex items-center rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400">
			<AlertCircle class="mr-2 h-4 w-4 shrink-0" />
			<span>{error}</span>
		</div>
	{/if}

	{#if webhookGroups.length === 0 && !error}
		<Card.Root class="border-dashed border-border bg-card">
			<Card.Content class="flex flex-col items-center justify-center py-16 text-center">
				<WebhookIcon class="mb-3 h-10 w-10 text-muted-foreground/40" />
				<h3 class="text-sm font-medium text-muted-foreground">No watchers yet</h3>
				<p class="mt-1 text-xs text-muted-foreground/60">Create a watcher, then enable its webhook settings.</p>
			</Card.Content>
		</Card.Root>
	{:else}
		<div class="space-y-4">
			{#each webhookGroups as group (group.serviceName)}
				<Card.Root class="border-border bg-card">
					<Card.Header>
						<Card.Title>{group.serviceName}</Card.Title>
						<Card.Description>{group.watchers.length} watcher{group.watchers.length === 1 ? '' : 's'} in this service group</Card.Description>
					</Card.Header>
					<Card.Content class="space-y-3">
						{#each group.watchers as watcher (watcher.id)}
							<div class="rounded-lg border border-border/70 bg-muted/20 p-4">
								<div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
									<div>
										<p class="font-medium">{watcher.name}</p>
										<p class="mt-1 text-xs text-muted-foreground">
											Services:
											{#if watcher.services.length > 0}
												{watcher.services.map((service) => service.windows_service_name).join(', ')}
											{:else}
												No services configured
											{/if}
										</p>
									</div>
									<div class="flex flex-wrap gap-2">
										<a href={resolve(`/watchers/${watcher.id}?tab=webhooks`)}>
											<Button.Root size="sm" variant="outline">History</Button.Root>
										</a>
										<a href={resolve(`/watchers/${watcher.id}/edit#webhooks`)}>
											<Button.Root size="sm">Configure</Button.Root>
										</a>
									</div>
								</div>
								<dl class="mt-4 grid gap-3 text-sm sm:grid-cols-2 xl:grid-cols-4">
									<div>
										<dt class="text-muted-foreground">Webhook</dt>
										<dd class="mt-1">{watcher.webhook_enabled ? 'Enabled' : 'Disabled'}</dd>
									</div>
									<div>
										<dt class="text-muted-foreground">URL</dt>
										<dd class="mt-1 truncate font-mono text-xs">{watcher.webhook_url || 'Global default / unset'}</dd>
									</div>
									<div>
										<dt class="text-muted-foreground">Paused</dt>
										<dd class="mt-1">{watcher.webhook_paused_at ? 'Yes' : 'No'}</dd>
									</div>
									<div>
										<dt class="text-muted-foreground">Failure Streak</dt>
										<dd class="mt-1">{watcher.webhook_failure_streak}</dd>
									</div>
								</dl>
								{#if watcher.webhook_pause_reason}
									<div class="mt-4 rounded-md border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-500">
										{watcher.webhook_pause_reason}
									</div>
								{/if}
							</div>
						{/each}
					</Card.Content>
				</Card.Root>
			{/each}
		</div>
	{/if}
</div>

<Dialog.Root bind:open={showAddDialog}>
	<Dialog.Content class="sm:max-w-2xl">
		<Dialog.Header>
			<Dialog.Title>Add Webhook</Dialog.Title>
			<Dialog.Description>
				Webhooks belong to watchers. Choose an existing watcher to configure, or create a new watcher if you do not have one yet.
			</Dialog.Description>
		</Dialog.Header>

		<div class="space-y-4">
			<div class="flex items-center justify-between gap-3 rounded-lg border border-border/70 bg-muted/20 p-4">
				<div>
					<p class="font-medium">Create a new watcher</p>
					<p class="text-sm text-muted-foreground">
						Use this when the service you want to integrate is not registered yet.
					</p>
				</div>
				<a href={resolve('/watchers/new')}>
					<Button.Root>New Watcher</Button.Root>
				</a>
			</div>

			<div class="space-y-3">
				<p class="text-sm font-medium">Configure an existing watcher</p>
				{#if watchers.length === 0}
					<div class="rounded-lg border border-dashed border-border/70 bg-muted/20 p-6 text-sm text-muted-foreground">
						No watchers available yet.
					</div>
				{:else}
					<div class="max-h-[50vh] space-y-3 overflow-auto pr-1">
						{#each webhookGroups as group (group.serviceName)}
							<div class="rounded-lg border border-border/70 p-3">
								<p class="font-medium">{group.serviceName}</p>
								<div class="mt-3 space-y-2">
									{#each group.watchers as watcher (watcher.id)}
										<div class="flex items-center justify-between gap-3 rounded-md border border-border/60 bg-muted/20 p-3">
											<div>
												<p class="font-medium">{watcher.name}</p>
												<p class="text-xs text-muted-foreground">
													{watcher.webhook_enabled ? 'Webhook enabled' : 'Webhook not enabled'}
												</p>
											</div>
											<a href={resolve(`/watchers/${watcher.id}/edit#webhooks`)}>
												<Button.Root size="sm">Configure</Button.Root>
											</a>
										</div>
									{/each}
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		</div>
	</Dialog.Content>
</Dialog.Root>
