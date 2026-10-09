<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type AgentLogSource, type LogFileResponse } from '$lib/api';
	import { parseLogLines } from '$lib/logs';
	import LogEntries from './log-entries.svelte';
	import RequestError from './request-error.svelte';
	import RequestLoading from './request-loading.svelte';
	import * as Button from '$lib/components/ui/button';
	import * as Select from '$lib/components/ui/select';
	import { Input } from '$lib/components/ui/input';
	import { Activity, RefreshCw, Search, ArrowDownWideNarrow } from '@lucide/svelte';

	let { watcherId, initialTraceId = '' }: { watcherId?: number; initialTraceId?: string } =
		$props();
	let traceId = $state('');
	let source = $state<AgentLogSource>('agent');
	let lineCount = $state('200');
	let response = $state.raw<LogFileResponse | null>(null);
	let loading = $state(true);
	let error = $state('');
	let search = $state('');
	let level = $state('all');
	let rawView = $state(false);
	let newestFirst = $state(true);
	let controller: AbortController | null = null;
	const sources = {
		agent: 'Structured agent log',
		stdout: 'Windows service · stdout',
		stderr: 'Windows service · stderr'
	};
	const entries = $derived(parseLogLines(response?.lines ?? []));
	const filtered = $derived.by(() => {
		const term = traceId ? '' : search.trim().toLowerCase();
		const result = entries.filter(
			(entry) =>
				(level === 'all' ||
					(level === 'errors'
						? entry.level === 'error' || entry.level === 'fatal'
						: entry.level === level)) &&
				(!term || entry.raw.toLowerCase().includes(term))
		);
		return newestFirst ? result.toReversed() : result;
	});
	const errorCount = $derived(
		entries.filter((entry) => entry.level === 'error' || entry.level === 'fatal').length
	);

	onMount(() => {
		search = initialTraceId;
		traceId = initialTraceId;
		void loadLogs();
		return () => controller?.abort();
	});

	async function loadLogs() {
		controller?.abort();
		const request = new AbortController();
		controller = request;
		loading = true;
		error = '';
		try {
			const result =
				watcherId !== undefined
					? await api.watcherLogs(watcherId, Number(lineCount), request.signal, traceId)
					: await api.agentLogs(Number(lineCount), source, request.signal, traceId);
			if (!request.signal.aborted) response = result;
		} catch (e) {
			if (!request.signal.aborted) error = e instanceof Error ? e.message : 'Failed to load logs';
		} finally {
			if (!request.signal.aborted) loading = false;
		}
	}
</script>

<div class="space-y-3">
	<div class="flex flex-wrap items-center justify-between gap-3">
		{#if watcherId === undefined}
			<Select.Root
				type="single"
				value={source}
				onValueChange={(v) => {
					if (v) {
						source = v as AgentLogSource;
						response = null;
						void loadLogs();
					}
				}}
			>
				<Select.Trigger class="w-full bg-card sm:w-64" aria-label="Log source"
					>{sources[source]}</Select.Trigger
				>
				<Select.Content
					>{#each Object.entries(sources) as [value, label] (value)}<Select.Item {value} {label}
							>{label}</Select.Item
						>{/each}</Select.Content
				>
			</Select.Root>
		{:else}
			<div>
				<h3 class="text-sm font-semibold">Watcher activity</h3>
				<p class="mt-0.5 text-xs text-muted-foreground">
					Polling, release checks, and deployment diagnostics
				</p>
			</div>
		{/if}
		<div class="flex items-center gap-2">
			<Select.Root
				type="single"
				value={lineCount}
				onValueChange={(v) => {
					if (v) {
						lineCount = v;
						void loadLogs();
					}
				}}
			>
				<Select.Trigger class="w-28 text-xs" aria-label="Log line limit"
					>{lineCount} lines</Select.Trigger
				>
				<Select.Content
					>{#each ['100', '200', '500', '1000'] as value (value)}<Select.Item
							{value}
							label={`${value} lines`}>{value} lines</Select.Item
						>{/each}</Select.Content
				>
			</Select.Root>
			<Button.Root variant="outline" size="sm" onclick={loadLogs} disabled={loading}
				><RefreshCw
					class={loading ? 'size-3.5 animate-spin motion-reduce:animate-none' : 'size-3.5'}
				/>{loading ? 'Loading…' : 'Refresh'}</Button.Root
			>
		</div>
	</div>
	<RequestError message={error} onRetry={loadLogs} />
	<section class="overflow-hidden rounded-xl border border-border bg-card" aria-label="Log records">
		<div class="flex flex-wrap items-center gap-2 border-b border-border p-3">
			<div class="relative min-w-0 basis-full sm:flex-1 sm:basis-0">
				<Search class="absolute top-2.5 left-3 size-3.5 text-muted-foreground" /><Input
					aria-label="Search logs"
					placeholder="Search logs or paste a request / poll ID…"
					value={search}
					oninput={(event) => {
						search = event.currentTarget.value;
						const value = search.trim().toLowerCase();
						const nextTrace = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(
							value
						)
							? value
							: '';
						if (nextTrace !== traceId) {
							traceId = nextTrace;
							response = null;
							void loadLogs();
						}
					}}
					class="h-9 w-full min-w-0 pl-9 text-xs"
				/>
			</div>
			<Select.Root type="single" bind:value={level}
				><Select.Trigger class="h-9 w-28 text-xs" aria-label="Log level"
					>{level === 'all'
						? 'All levels'
						: level === 'errors'
							? 'Errors'
							: level === 'warn'
								? 'Warnings'
								: level === 'info'
									? 'Info'
									: 'Debug'}</Select.Trigger
				><Select.Content
					>{#each [['all', 'All levels'], ['errors', 'Errors'], ['warn', 'Warnings'], ['info', 'Info'], ['debug', 'Debug']] as [value, label] (value)}<Select.Item
							{value}
							{label}>{label}</Select.Item
						>{/each}</Select.Content
				></Select.Root
			>
			<Button.Root
				variant="ghost"
				size="sm"
				class="h-9 text-xs"
				onclick={() => (newestFirst = !newestFirst)}
				><ArrowDownWideNarrow class="size-3.5" />{newestFirst
					? 'Newest first'
					: 'Oldest first'}</Button.Root
			>
			<div class="flex rounded-md border border-border p-0.5">
				<Button.Root
					variant={!rawView ? 'secondary' : 'ghost'}
					size="sm"
					class="h-7 px-2 text-xs"
					aria-pressed={!rawView}
					onclick={() => (rawView = false)}>Events</Button.Root
				><Button.Root
					variant={rawView ? 'secondary' : 'ghost'}
					size="sm"
					class="h-7 px-2 text-xs"
					aria-pressed={rawView}
					onclick={() => (rawView = true)}>Raw</Button.Root
				>
			</div>
		</div>
		{#if traceId}<div
				class="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-2 text-xs"
			>
				<p class="min-w-0 font-mono break-all text-muted-foreground">
					Trace: {traceId} · retained files
				</p>
				<Button.Root
					variant="ghost"
					size="sm"
					onclick={() => {
						search = '';
						traceId = '';
						response = null;
						void loadLogs();
					}}>Clear</Button.Root
				>
			</div>{/if}
		<div
			class="flex flex-wrap items-center justify-between gap-2 border-b border-border bg-muted/20 px-4 py-2 text-[11px] text-muted-foreground"
		>
			<p>
				{filtered.length} / {entries.length} events{#if errorCount}<span class="ml-2 text-red-400"
						>{errorCount} errors</span
					>{/if}
			</p>
			<p class="max-w-full truncate font-mono" title={response?.log_file}>
				{response?.log_file || 'Waiting for log file…'}
			</p>
		</div>
		{#if loading && !response}<RequestLoading label="Loading logs…" />
		{:else if filtered.length}
			<div class="max-h-[65vh] overflow-auto">
				{#if rawView}<pre class="p-4 font-mono text-xs leading-relaxed">{filtered
							.map((entry) => entry.raw)
							.join('\n')}</pre>{:else}<LogEntries entries={filtered} />{/if}
			</div>
		{:else if !error}
			<div class="flex flex-col items-center gap-2 px-4 py-14 text-center">
				<Activity class="mb-1 size-6 text-muted-foreground/40" />
				<p class="text-sm">{entries.length ? 'No matching events' : 'No logs yet'}</p>
				<p class="text-xs text-muted-foreground">
					{entries.length
						? 'Try a different search or level.'
						: traceId
							? 'No records for this trace in retained log files. Check the ID and log source.'
							: watcherId !== undefined
								? 'Records appear after this watcher runs a check.'
								: source === 'agent'
									? 'Agent activity will appear here.'
									: 'Output appears when the Windows service writes to this stream.'}
				</p>
			</div>
		{/if}
	</section>
</div>
