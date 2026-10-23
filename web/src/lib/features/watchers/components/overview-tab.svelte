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

<div class="space-y-4">
	<Card.Root>
		<Card.Header><Card.Title class="text-base">Deployment</Card.Title></Card.Header>
		<Card.Content>
			<dl class="grid grid-cols-2 gap-4 sm:grid-cols-3">
				<div>
					<dt class="text-xs text-muted-foreground">Current version</dt>
					<dd class="mt-1 font-mono text-lg font-semibold break-all">
						{watcher.current_version || 'No deployment yet'}
					</dd>
				</div>
				<div>
					<dt class="text-xs text-muted-foreground">Last deployed</dt>
					<dd class="mt-1 text-sm">
						{watcher.last_deployed ? timeAgo(watcher.last_deployed) : 'Never'}
					</dd>
				</div>
				<div>
					<dt class="text-xs text-muted-foreground">Last checked</dt>
					<dd class="mt-1 text-sm">
						{watcher.last_checked ? timeAgo(watcher.last_checked) : 'Never'}
					</dd>
				</div>
			</dl>
			{#if watcher.last_error}<p
					role="alert"
					class="mt-4 rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm break-words text-red-400"
				>
					{watcher.last_error}
				</p>{/if}
		</Card.Content>
	</Card.Root>
	<Card.Root>
		<Card.Header><Card.Title class="text-base">Configuration</Card.Title></Card.Header>
		<Card.Content class="space-y-4">
			<dl class="grid grid-cols-2 gap-4">
				<div>
					<dt class="text-xs text-muted-foreground">Deployment policy</dt>
					<dd class="mt-1 text-sm">
						{watcher.auto_deploy
							? 'Automatic'
							: 'Manual approval'}{#if watcher.intercept_next_release}
							· Next release held{/if}
					</dd>
				</div>
				<div>
					<dt class="text-xs text-muted-foreground">Polling</dt>
					<dd class="mt-1 text-sm">
						{watcher.paused ? 'Paused' : `Every ${watcher.check_interval_sec}s`}
					</dd>
				</div>
				<div>
					<dt class="text-xs text-muted-foreground">Health checks</dt>
					<dd class="mt-1 text-sm">{watcher.hc_enabled ? 'Enabled' : 'Disabled'}</dd>
				</div>
				<div>
					<dt class="text-xs text-muted-foreground">Install directory</dt>
					<dd class="mt-1 font-mono text-xs break-all">{watcher.install_dir}</dd>
				</div>
			</dl>
			<details class="border-t border-border pt-4 text-sm">
				<summary class="cursor-pointer text-muted-foreground"
					>Source and integration details</summary
				>
				<dl class="mt-4 grid gap-4 sm:grid-cols-2">
					<div class="sm:col-span-2">
						<dt class="text-xs text-muted-foreground">Metadata URL</dt>
						<dd class="mt-1 font-mono text-xs break-all">{watcher.metadata_url}</dd>
					</div>
					<div>
						<dt class="text-xs text-muted-foreground">Download retries</dt>
						<dd class="mt-1">{watcher.download_retries}</dd>
					</div>
					<div>
						<dt class="text-xs text-muted-foreground">Deployment environment</dt>
						<dd class="mt-1 break-words">{watcher.deployment_environment || 'Global default'}</dd>
					</div>
					<div>
						<dt class="text-xs text-muted-foreground">GitHub token</dt>
						<dd class="mt-1 break-all">
							{watcher.has_github_token
								? watcher.github_token_masked || 'Watcher override'
								: 'Global default'}
						</dd>
					</div>
					<div>
						<dt class="text-xs text-muted-foreground">Webhooks</dt>
						<dd class="mt-1">{watcher.webhook_enabled ? 'Enabled' : 'Disabled'}</dd>
					</div>
					{#if watcher.webhook_enabled}<div class="sm:col-span-2">
							<dt class="text-xs text-muted-foreground">Webhook URL</dt>
							<dd class="mt-1 font-mono text-xs break-all">
								{watcher.webhook_url || 'Global default'}
							</dd>
						</div>{/if}
				</dl>
			</details>
		</Card.Content>
	</Card.Root>
</div>
