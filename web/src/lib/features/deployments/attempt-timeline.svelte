<script lang="ts">
	import { createQueries } from '@tanstack/svelte-query';
	import { queries } from '$lib/query/options';
	import { type DeployLog } from '$lib/api';
	import { resolve } from '$app/paths';
	import { formatDate, formatDuration, statusColor } from '$lib/utils';
	import * as Button from '$lib/components/ui/button';
	import { Rocket, RotateCcw, Clock, ArrowUpRight, Link, ChevronDown } from '@lucide/svelte';
	let {
		attempts,
		watcherId,
		total,
		page,
		pageSize,
		paging = false,
		onpage,
		onsize
	}: {
		attempts: DeployLog[];
		watcherId: number;
		total: number;
		page: number;
		pageSize: number;
		/** True while the shown rows are the previous page's placeholder. */
		paging?: boolean;
		onpage: (page: number) => void | Promise<void>;
		onsize: (size: number) => void | Promise<void>;
	} = $props();
	let expanded = $state<number[]>([]);
	const relatedIDs = $derived(
		[
			...new Set(attempts.filter((attempt) => expanded.includes(attempt.id)).flatMap(relationIDs))
		].filter((id) => !attempts.some((attempt) => attempt.id === id))
	);
	const lookups = createQueries(() => ({
		queries: relatedIDs.map((id) => ({ ...queries.attempt(watcherId, id), staleTime: 30000 }))
	}));
	const related = $derived(
		Object.fromEntries(
			lookups.flatMap((query) =>
				query.data?.watcher_id === watcherId ? [[query.data.id, query.data]] : []
			)
		)
	);
	const loading = $derived(
		attempts
			.filter((attempt) =>
				relationIDs(attempt).some((id) => lookups[relatedIDs.indexOf(id)]?.isPending)
			)
			.map((attempt) => attempt.id)
	);
	const errors = $derived(
		Object.fromEntries(
			attempts.map((attempt) => [
				attempt.id,
				relationIDs(attempt)
					.map((id) => lookups[relatedIDs.indexOf(id)]?.error?.message)
					.find(Boolean) ?? ''
			])
		)
	);

	const ordered = $derived(
		[...attempts].sort(
			(a, b) => Date.parse(b.started_at || '') - Date.parse(a.started_at || '') || b.id - a.id
		)
	);
	function relationIDs(attempt: DeployLog) {
		return [
			...new Set(
				[attempt.parent_attempt_id, attempt.root_attempt_id].filter((id): id is number =>
					Boolean(id && id !== attempt.id)
				)
			)
		];
	}
	function find(id: number) {
		return attempts.find((attempt) => attempt.id === id) || related[id];
	}
	async function reveal(attempt: DeployLog) {
		if (expanded.includes(attempt.id)) {
			expanded = expanded.filter((id) => id !== attempt.id);
			return;
		}
		expanded = [...expanded, attempt.id];
	}
	async function loadRelated(attempt: DeployLog) {
		await Promise.all(relationIDs(attempt).map((id) => lookups[relatedIDs.indexOf(id)]?.refetch()));
	}
	async function paginate(next: number) {
		await onpage(next);
		expanded = [];
	}

	function reason(attempt: DeployLog) {
		return (
			attempt.reason?.replaceAll('_', ' ') ||
			(attempt.kind === 'rollback' ? 'Rollback requested' : 'Release deployment')
		);
	}
</script>

<div class="space-y-4">
	<div class="section-toolbar">
		<div>
			<h3 class="text-sm font-semibold">Deployment timeline</h3>
			<p class="mt-1 text-xs text-muted-foreground">
				{attempts.length ? (page - 1) * pageSize + 1 : 0}–{Math.min(
					(page - 1) * pageSize + attempts.length,
					total
				)} of {total} attempts · deployment and recovery are recorded separately
			</p>
		</div>
		<div class="flex flex-wrap items-center gap-2">
			<select
				class="filter-select h-8 text-xs"
				aria-label="Deployment attempts per page"
				value={pageSize}
				disabled={paging}
				onchange={async (event) => {
					await onsize(Number(event.currentTarget.value));
					expanded = [];
				}}
				><option value="10">10 / page</option><option value="25">25 / page</option><option
					value="50">50 / page</option
				></select
			><Button.Root
				variant="outline"
				size="sm"
				disabled={paging || page <= 1}
				onclick={() => paginate(page - 1)}>Prev</Button.Root
			><Button.Root
				variant="outline"
				size="sm"
				disabled={paging || page * pageSize >= total}
				onclick={() => paginate(page + 1)}>Next</Button.Root
			>
		</div>
	</div>
	{#if ordered.length}
		<ol
			class={['attempt-timeline transition-opacity', paging && 'opacity-60']}
			aria-label="Deployment attempts"
			aria-busy={paging}
		>
			{#each ordered as attempt (attempt.id)}
				<li class="attempt" id={`attempt-${attempt.id}`}>
					<span
						class={[
							'attempt-icon',
							attempt.status === 'failed'
								? 'text-red-400'
								: attempt.kind === 'rollback'
									? 'text-amber-300'
									: 'text-primary'
						]}
						>{#if attempt.kind === 'rollback'}<RotateCcw class="size-4" />{:else}<Rocket
								class="size-4"
							/>{/if}</span
					>
					<article class="attempt-card" aria-label={`Attempt ${attempt.id}`}>
						<div class="section-toolbar">
							<div>
								<p class="eyebrow mb-1">
									{attempt.kind === 'rollback' ? 'Rollback' : 'Deployment'} / #{attempt.id}
								</p>
								<h4 class="font-mono text-sm font-medium">
									{attempt.from_version || 'First deployment'}
									<span class="mx-1 text-muted-foreground">→</span>
									{attempt.version}
								</h4>
							</div>
							<span
								class="rounded border px-2 py-1 text-[11px] capitalize {statusColor(
									attempt.status
								)}">{attempt.status.replaceAll('_', ' ')}</span
							>
						</div>
						<div class="mt-3 flex flex-wrap gap-x-5 gap-y-2 text-[11px] text-muted-foreground">
							<span class="inline-flex items-center gap-1.5"
								><Clock class="size-3" />{formatDate(attempt.started_at)}</span
							><span>{formatDuration(attempt.duration_ms)}</span><span class="capitalize"
								>{attempt.triggered_by || 'agent'} · {reason(attempt)}</span
							>{#if attempt.github_deployment_id > 0}<span
									>GitHub deployment #{attempt.github_deployment_id}</span
								>{/if}
						</div>
						{#if attempt.error}<div
								class="mt-3 rounded border border-red-500/20 bg-red-500/5 px-3 py-2"
							>
								<p class="text-xs break-words text-red-300">{attempt.error}</p>
								{#if attempt.failure_phase}<p
										class="mt-1 font-mono text-[10px] text-muted-foreground"
									>
										Phase: {attempt.failure_phase}
									</p>{/if}
							</div>{/if}
						{#if attempt.failed_target_version}<p class="mt-3 text-xs text-muted-foreground">
								Recovery after failed target <span class="font-mono text-foreground"
									>{attempt.failed_target_version}</span
								>. This rollback has its own result above.
							</p>{/if}
						<div class="mt-4 flex flex-wrap items-center gap-2">
							<Button.Root
								variant="outline"
								size="sm"
								href={resolve('/watchers/[id]/logs/[logId]', {
									id: String(watcherId),
									logId: String(attempt.id)
								})}>View logs<ArrowUpRight class="size-3.5" /></Button.Root
							>{#if relationIDs(attempt).length}<Button.Root
									variant="ghost"
									size="sm"
									onclick={() => reveal(attempt)}
									aria-expanded={expanded.includes(attempt.id)}
									><Link class="size-3.5" />Related attempts<ChevronDown
										class="size-3.5"
									/></Button.Root
								>{/if}
						</div>
						{#if expanded.includes(attempt.id)}<div class="attempt-related space-y-3">
								{#if loading.includes(attempt.id)}<p
										role="status"
										class="text-xs text-muted-foreground"
									>
										Loading related attempts…
									</p>{/if}{#if errors[attempt.id]}<p role="alert" class="text-xs text-red-300">
										{errors[attempt.id]}
									</p>
									<Button.Root size="xs" variant="outline" onclick={() => loadRelated(attempt)}
										>Retry related attempts</Button.Root
									>{/if}{#each relationIDs(attempt) as id (id)}{@const entry = find(id)}
									<div class="section-toolbar text-xs">
										<div>
											<p class="text-muted-foreground">
												{id === attempt.parent_attempt_id ? 'Parent attempt' : 'Original attempt'} #{id}
											</p>
											{#if entry}<p class="mt-1 font-mono">
													{entry.kind || 'deploy'} · {entry.version} ·
													<span
														class={entry.status === 'failed' ? 'text-red-300' : 'text-foreground'}
														>{entry.status}</span
													>
												</p>{:else if !loading.includes(attempt.id)}<p
													class="mt-1 text-muted-foreground"
												>
													Details unavailable. Open logs to investigate.
												</p>{/if}
										</div>
										<Button.Root
											variant="link"
											size="xs"
											href={resolve('/watchers/[id]/logs/[logId]', {
												id: String(watcherId),
												logId: String(id)
											})}>Open attempt #{id}<ArrowUpRight class="size-3" /></Button.Root
										>
									</div>{/each}
							</div>{/if}
					</article>
				</li>
			{/each}
		</ol>
	{:else}<div class="console-empty">
			<Rocket class="size-6 text-primary" />
			<p class="mt-4 text-sm font-medium">No deployments yet</p>
			<p class="mt-2 text-xs text-muted-foreground">
				Deployments and their recovery attempts appear here after a release runs.
			</p>
		</div>{/if}
</div>
