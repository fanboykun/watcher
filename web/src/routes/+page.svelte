<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { api, type SystemStatus, type Watcher } from '$lib/api';
	import {
		ArrowUpRight,
		ArrowRight,
		Eye,
		Server,
		Rocket,
		CircleAlert,
		Plus,
		RefreshCw,
		CheckCheck,
		Radio,
		Terminal,
		GitBranch
	} from '@lucide/svelte';
	import * as Button from '$lib/components/ui/button';
	import PageHeading from '$lib/components/console/page-heading.svelte';
	import Metric from '$lib/components/console/metric.svelte';
	import EmptyState from '$lib/components/console/empty-state.svelte';
	import WatcherCard from '$lib/features/watchers/watcher-card.svelte';
	import { needsAttention } from '$lib/features/watchers/presentation';
	import RequestLoading from '$lib/components/request-loading.svelte';
	import RequestError from '$lib/components/request-error.svelte';

	let status = $state.raw<SystemStatus | null>(null);
	let watchers = $state.raw<Watcher[]>([]);
	let error = $state('');
	let loading = $state(true);
	let refreshing = $state(false);
	let updatedAt = $state('');
	let mounted = false;
	const attention = $derived(watchers.filter(needsAttention));
	const active = $derived(watchers.filter((w) => !w.paused).length);
	const awaitingReview = $derived(watchers.filter((w) => w.status === 'pending_approval').length);

	async function load() {
		if (refreshing) return;
		refreshing = true;
		try {
			const [nextStatus, nextWatchers] = await Promise.all([api.status(), api.listWatchers()]);
			if (!mounted) return;
			status = nextStatus;
			watchers = nextWatchers;
			error = '';
			updatedAt = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
		} catch (cause) {
			if (mounted) error = cause instanceof Error ? cause.message : 'Could not load your workspace';
		} finally {
			if (mounted) {
				loading = false;
				refreshing = false;
			}
		}
	}
	onMount(() => {
		mounted = true;
		void load();
		const timer = setInterval(() => {
			if (document.visibilityState === 'visible') void load();
		}, 15000);
		return () => {
			mounted = false;
			clearInterval(timer);
		};
	});
</script>

<div class="space-y-7">
	<PageHeading
		eyebrow="Mission control / 01"
		title="Your fleet, at a glance."
		description="Every repository, release, and running service. One clear view."
	>
		{#snippet actions()}
			<Button.Root
				variant="outline"
				size="sm"
				onclick={load}
				disabled={refreshing}
				aria-label="Refresh overview"
				><RefreshCw class={['size-3.5', refreshing && 'animate-spin']} />Refresh</Button.Root
			>
			<Button.Root size="sm" href={resolve('/watchers/new')}
				><Plus class="size-4" /> Add watcher</Button.Root
			>
		{/snippet}
	</PageHeading>
	<RequestError message={error} onRetry={load} />
	{#if loading}
		<RequestLoading label="Connecting to your deployment workspace…" />
	{:else if status}
		<div class="console-metrics">
			<Metric
				label="Watchers"
				value={status.watcher_count}
				detail={`${active} polling enabled`}
				icon={Eye}
			/>
			<Metric
				label="Managed services"
				value={status.service_count}
				detail="Across NSSM and IIS"
				icon={Server}
			/>
			<Metric
				label="Deployments / 24h"
				value={status.deploys_24h}
				detail="In the last 24 hours"
				icon={Rocket}
			/>
			<Metric
				label="Awaiting review"
				value={awaitingReview}
				detail="Release approvals pending"
				icon={CircleAlert}
				accent={awaitingReview > 0}
			/>
		</div>
		<div class="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_300px]">
			<section class="console-panel" aria-labelledby="attention-title">
				<div class="panel-heading">
					<div>
						<h2 id="attention-title">
							Needs attention <span
								class="ml-2 rounded bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground"
								>{attention.length.toString().padStart(2, '0')}</span
							>
						</h2>
						<p>Release approvals and operational issues.</p>
					</div>
					<CircleAlert class="size-4 text-primary" />
				</div>
				{#each attention as watcher (watcher.id)}
					<a
						class="attention-row"
						href={resolve(
							`/watchers/[id]?tab=${watcher.status === 'pending_approval' ? 'candidates' : 'overview'}`,
							{ id: String(watcher.id) }
						)}
					>
						<span class="repo-symbol"><GitBranch class="size-4" /></span>
						<div class="min-w-0 flex-1">
							<p class="text-xs font-semibold">{watcher.name}</p>
							<p class="mt-1 truncate text-xs text-muted-foreground">
								{watcher.status === 'pending_approval'
									? `Release ${watcher.pending_version} is ready for review`
									: watcher.last_error ||
										watcher.last_poll_error ||
										'Deployment failed. Open watcher to investigate.'}
							</p>
						</div>
						<span class="hidden text-[11px] text-primary sm:inline"
							>{watcher.status === 'pending_approval' ? 'Review' : 'Investigate'}</span
						><ArrowUpRight class="size-3.5 shrink-0 text-muted-foreground" />
					</a>
				{:else}
					<div class="flex items-start gap-3 px-6 py-7">
						<CheckCheck class="mt-0.5 size-5 text-emerald-400" />
						<div>
							<p class="text-sm font-medium">
								{watchers.length
									? 'Nothing needs your attention.'
									: 'A clean slate. Ready when you are.'}
							</p>
							<p class="mt-1 text-xs leading-relaxed text-muted-foreground">
								{watchers.length
									? 'No pending approvals or reported watcher errors.'
									: 'Connect your first repository to start watching for releases.'}
							</p>
						</div>
					</div>
				{/each}
			</section>
			<section class="console-panel" aria-labelledby="agent-title">
				<div class="panel-heading">
					<h2 id="agent-title">Agent heartbeat</h2>
					<span class="font-mono text-[10px] text-muted-foreground uppercase">{status.status}</span>
				</div>
				<dl class="space-y-3 px-5 py-5 text-xs">
					<div class="flex justify-between gap-2">
						<dt class="text-muted-foreground">Version</dt>
						<dd class="font-mono">{status.version}</dd>
					</div>
					<div class="flex justify-between gap-2">
						<dt class="text-muted-foreground">Uptime</dt>
						<dd class="font-mono">{status.uptime_human}</dd>
					</div>
					<div class="flex justify-between gap-2">
						<dt class="text-muted-foreground">Last refreshed</dt>
						<dd class="font-mono">{updatedAt}</dd>
					</div>
				</dl>
				<a class="quick-link" href={resolve('/logs')}
					><Terminal class="size-3.5 text-muted-foreground" /> Inspect agent logs <ArrowRight
						class="ml-auto size-3.5"
					/></a
				>
			</section>
		</div>
		<section aria-labelledby="fleet-title">
			<div class="mb-4 flex flex-wrap items-center justify-between gap-3">
				<div class="flex items-center gap-3">
					<h2 id="fleet-title" class="text-sm font-semibold">Deployment fleet</h2>
					<span class="font-mono text-[10px] text-muted-foreground"
						>{watchers.length.toString().padStart(2, '0')} REPOSITORIES</span
					>
				</div>
				<a
					class="inline-flex items-center gap-1.5 text-xs text-muted-foreground hover:text-primary"
					href={resolve('/watchers')}>View all watchers <ArrowUpRight class="size-3.5" /></a
				>
			</div>
			{#if watchers.length}
				<div class="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">
					{#each watchers as watcher (watcher.id)}<WatcherCard {watcher} />{/each}
				</div>
			{:else}
				<EmptyState
					title="Your first release starts here."
					description="Connect a GitHub repository, choose your services, and let Watcher handle the delivery."
					icon={Radio}
					>{#snippet actions()}<Button.Root href={resolve('/watchers/new')} size="sm"
							><Plus class="size-4" />Connect a repository</Button.Root
						>{/snippet}</EmptyState
				>
			{/if}
		</section>
	{/if}
</div>
