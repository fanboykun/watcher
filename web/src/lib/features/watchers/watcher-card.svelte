<script lang="ts">
	import { ArrowUpRight, GitBranch, Layers, Clock } from '@lucide/svelte';
	import { resolve } from '$app/paths';
	import type { Watcher } from '$lib/api';
	import { statusColor, timeAgo } from '$lib/utils';
	import WatcherPollingStatus from '$lib/components/watcher-polling-status.svelte';
	let { watcher }: { watcher: Watcher } = $props();
</script>

<a class="watcher-card" href={resolve('/watchers/[id]', { id: String(watcher.id) })}>
	<div class="flex items-start justify-between gap-3">
		<div class="flex min-w-0 items-center gap-3">
			<span class="repo-symbol"><GitBranch class="size-5" /></span>
			<div class="min-w-0">
				<h3 class="truncate text-sm font-semibold">{watcher.name}</h3>
				<p class="mt-1 truncate font-mono text-xs text-muted-foreground">{watcher.service_name}</p>
			</div>
		</div>
		<ArrowUpRight class="size-4 shrink-0 text-muted-foreground" />
	</div>
	<div class="my-5 flex flex-wrap items-center justify-between gap-2">
		<span class="font-mono text-lg tracking-tight">{watcher.current_version || 'Undeployed'}</span>
		<span class="rounded-md border px-2 py-1 text-[11px] capitalize {statusColor(watcher.status)}"
			>{watcher.status.replaceAll('_', ' ') || 'Idle'}</span
		>
	</div>
	<div class="border-t border-border pt-4"><WatcherPollingStatus {watcher} /></div>
	{#if watcher.last_error}<p class="mt-3 line-clamp-2 text-xs leading-relaxed text-red-400">
			{watcher.last_error}
		</p>{/if}
	<div
		class="mt-5 flex flex-wrap items-center justify-between gap-2 border-t border-border pt-3 text-[11px] text-muted-foreground"
	>
		<span class="inline-flex items-center gap-1.5"
			><Layers class="size-3" />{watcher.services?.length ?? 0} services</span
		>
		<span class="inline-flex items-center gap-1.5"
			><Clock class="size-3" />{watcher.last_deployed
				? `Deployed ${timeAgo(watcher.last_deployed)}`
				: 'No deployments yet'}</span
		>
	</div>
</a>
