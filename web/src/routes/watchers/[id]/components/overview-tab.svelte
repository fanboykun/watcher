<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import type { Watcher } from '$lib/api';
	import { timeAgo } from '$lib/utils';

	let {
		watcher
	}: {
		watcher: Watcher;
	} = $props();
</script>

<div class="grid gap-4 sm:grid-cols-2">
	<Card.Root class="border-border bg-card">
		<Card.Header class="pb-3">
			<Card.Title class="text-sm font-medium text-muted-foreground">Configuration</Card.Title>
		</Card.Header>
		<Card.Content class="space-y-2 text-sm">
			<div class="flex justify-between">
				<span class="text-muted-foreground">Metadata URL</span>
				<span class="max-w-[220px] truncate font-mono text-xs">{watcher.metadata_url}</span>
			</div>
			<div class="flex justify-between">
				<span class="text-muted-foreground">Check Interval</span>
				<span>{watcher.check_interval_sec}s</span>
			</div>
			<div class="flex justify-between">
				<span class="text-muted-foreground">Install Dir</span>
				<span class="font-mono text-xs">{watcher.install_dir}</span>
			</div>
			<div class="flex justify-between">
				<span class="text-muted-foreground">Download Retries</span>
				<span>{watcher.download_retries}</span>
			</div>
			<div class="flex justify-between">
				<span class="text-muted-foreground">Health Check</span>
				<span>{watcher.hc_enabled ? 'Enabled' : 'Disabled'}</span>
			</div>
			<div class="flex justify-between">
				<span class="text-muted-foreground">Deploy Environment</span>
				<span>{watcher.deployment_environment || 'Global default'}</span>
			</div>
			<div class="flex justify-between">
				<span class="text-muted-foreground">GitHub Token</span>
				<span>{watcher.has_github_token ? (watcher.github_token_masked || 'Configured') : 'Global default'}</span>
			</div>
			<div class="flex justify-between">
				<span class="text-muted-foreground">Webhook</span>
				<span>{watcher.webhook_enabled ? 'Enabled' : 'Disabled'}</span>
			</div>
			<div class="flex justify-between">
				<span class="text-muted-foreground">Webhook URL</span>
				<span class="max-w-[220px] truncate font-mono text-xs">{watcher.webhook_url || 'Global default / unset'}</span>
			</div>
		</Card.Content>
	</Card.Root>

	<Card.Root class="border-border bg-card">
		<Card.Header class="pb-3">
			<Card.Title class="text-sm font-medium text-muted-foreground">Deploy State</Card.Title>
		</Card.Header>
		<Card.Content class="space-y-2 text-sm">
			<div class="flex justify-between">
				<span class="text-muted-foreground">Current Version</span>
				<span class="font-mono">{watcher.current_version || '—'}</span>
			</div>
			<div class="flex justify-between">
				<span class="text-muted-foreground">Last Checked</span>
				<span>{watcher.last_checked ? timeAgo(watcher.last_checked) : 'Never'}</span>
			</div>
			<div class="flex justify-between">
				<span class="text-muted-foreground">Last Deployed</span>
				<span>{watcher.last_deployed ? timeAgo(watcher.last_deployed) : 'Never'}</span>
			</div>
			{#if watcher.last_error}
				<div class="mt-2 rounded border border-red-500/30 bg-red-500/10 p-2 text-xs text-red-400">
					{watcher.last_error}
				</div>
			{/if}
		</Card.Content>
	</Card.Root>
</div>
