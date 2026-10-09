<script lang="ts">
	import type { Watcher } from '$lib/api';
	import { timeAgo } from '$lib/utils';
	let { watcher, kind = 'both' }: { watcher: Watcher; kind?: 'activity' | 'result' | 'both' } =
		$props();
	const activity = $derived(watcher.polling_activity || (watcher.paused ? 'paused' : 'unknown'));
	const activityLabel = $derived(
		{
			active: 'Active',
			checking: 'Checking now',
			paused: 'Paused',
			stopped: 'Stopped',
			unknown: 'Unknown'
		}[activity]
	);
	const labels: Record<string, string> = {
		error: 'Failed',
		no_update: 'Up to date',
		new_release: 'New release',
		intercepted: 'Awaiting approval',
		skipped: 'Release skipped',
		deploy_suspended: 'Deployment suspended',
		completed: 'Completed'
	};
	const result = $derived(
		watcher.last_poll_status
			? labels[watcher.last_poll_status] || watcher.last_poll_status.replaceAll('_', ' ')
			: 'No completed polls'
	);
</script>

<div class={kind === 'both' ? 'grid gap-4 sm:grid-cols-2' : 'min-w-0'}>
	{#if kind !== 'result'}
		<div>
			{#if kind === 'both'}<p class="mb-1.5 text-xs text-muted-foreground">Polling</p>{/if}
			<p class="inline-flex items-center gap-2 text-sm font-medium">
				<span
					class={`size-2 rounded-full ${activity === 'checking' ? 'bg-blue-400 motion-safe:animate-pulse' : activity === 'active' ? 'bg-emerald-400' : activity === 'paused' ? 'bg-amber-400' : activity === 'stopped' ? 'bg-red-400' : 'bg-muted-foreground'}`}
				></span>
				{activityLabel}
			</p>
			<p class="mt-1 text-xs text-muted-foreground">
				{activity === 'checking'
					? 'Poll in progress'
					: activity === 'active'
						? `Every ${watcher.check_interval_sec}s`
						: activity === 'paused'
							? 'Automatic polling paused'
							: activity === 'stopped'
								? 'Polling loop is not running'
								: 'Runtime status unavailable'}
			</p>
		</div>
	{/if}
	{#if kind !== 'activity'}
		<div class="min-w-0">
			{#if kind === 'both'}<p class="mb-1.5 text-xs text-muted-foreground">Latest poll</p>{/if}
			<p
				class={`text-sm font-medium ${watcher.last_poll_status === 'error' ? 'text-red-400' : watcher.last_poll_status === 'intercepted' || watcher.last_poll_status === 'deploy_suspended' ? 'text-amber-400' : ''}`}
			>
				{result}
			</p>
			{#if watcher.last_poll_at}<p
					class="mt-1 text-xs text-muted-foreground"
					title={watcher.last_poll_at}
				>
					{timeAgo(watcher.last_poll_at)}
				</p>{/if}
			{#if watcher.last_poll_error}<p
					class={`${kind === 'both' ? 'mt-1 text-xs break-words' : 'mt-1 max-w-72 truncate text-xs'} ${watcher.last_poll_status === 'error' ? 'text-red-400' : 'text-muted-foreground'}`}
					title={watcher.last_poll_error}
				>
					{watcher.last_poll_error}
				</p>{/if}
		</div>
	{/if}
</div>
