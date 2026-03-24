<script lang="ts">
	import { page } from '$app/state';
	import { api, type DeployLog, type Watcher } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import {
		ArrowLeft,
		Clock,
		CheckCircle2,
		XCircle,
		Loader2,
		RotateCcw,
		Copy
	} from '@lucide/svelte';
	import { resolve } from '$app/paths';
	import { timeAgo } from '$lib/utils';
	import { onMount } from 'svelte';

	const id = Number(page.params.id);
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const logId = Number((page.params as any).logId);

	let watcher = $state<Watcher | null>(null);
	let deployLog = $state<DeployLog | null>(null);
	let error = $state('');
	let streamSource: EventSource | null = null;
	let liveLogs = $state('');
	let logContainer = $state<HTMLDivElement | null>(null);

	onMount(() => {
		const init = async () => {
			try {
				[watcher, deployLog] = await Promise.all([
					api.getWatcher(id),
					api.watcherDeployLog(id, logId)
				]);
				liveLogs = deployLog?.logs || '';
				if (deployLog && !deployLog.completed_at) {
					streamSource = new EventSource(`/api/watchers/${id}/deploys/${logId}/stream`);
					streamSource.onmessage = async (e) => {
						if (e.data === 'DONE') {
							streamSource?.close();
							streamSource = null;
							deployLog = await api.watcherDeployLog(id, logId);
							liveLogs = deployLog.logs || '';
							return;
						}
						liveLogs = liveLogs ? `${liveLogs}\n${e.data}` : e.data;
						if (deployLog) {
							deployLog = { ...deployLog, logs: liveLogs };
						}
					};
					streamSource.onerror = () => {
						// Browser will auto-reconnect for temporary disconnects.
					};
				}
			} catch (e) {
				error = e instanceof Error ? e.message : 'Failed to load deploy log';
			}
		};
		init();
		return () => {
			if (streamSource) {
				streamSource.close();
				streamSource = null;
			}
		};
	});

	function formatDate(ts: string | null): string {
		if (!ts) return '—';
		return new Date(ts).toLocaleString();
	}

	function formatDuration(ms: number): string {
		if (!ms) return '—';
		if (ms < 1000) return `${ms}ms`;
		return `${(ms / 1000).toFixed(1)}s`;
	}

	function statusColor(s: string) {
		switch (s) {
			case 'healthy':
				return 'bg-emerald-500/15 text-emerald-400 border-emerald-500/30';
			case 'deploying':
				return 'bg-blue-500/15 text-blue-400 border-blue-500/30';
			case 'failed':
				return 'bg-red-500/15 text-red-400 border-red-500/30';
			case 'rollback':
				return 'bg-amber-500/15 text-amber-400 border-amber-500/30';
			default:
				return 'bg-muted text-muted-foreground border-border';
		}
	}

	function deployIcon(s: string) {
		switch (s) {
			case 'healthy':
				return CheckCircle2;
			case 'failed':
				return XCircle;
			case 'deploying':
				return Loader2;
			case 'rollback':
				return RotateCcw;
			default:
				return Clock;
		}
	}

	function normalizedLines(input: string): string[] {
		if (!input) return [];
		return input.replace(/\r\n/g, '\n').split('\n').filter((line) => line.trim() !== '');
	}

	function lineTone(line: string): string {
		const lower = line.toLowerCase();
		if (lower.includes(' failed') || lower.includes('error') || lower.includes('panic')) {
			return 'text-red-300';
		}
		if (lower.includes('warn') || lower.includes('rollback')) {
			return 'text-amber-300';
		}
		if (lower.includes('success') || lower.includes('healthy') || lower.includes('complete')) {
			return 'text-emerald-300';
		}
		if (lower.includes('github_deployment:')) {
			return 'text-blue-300';
		}
		return 'text-zinc-300';
	}

	$effect(() => {
		if (!logContainer || !liveLogs) return;
		if (deployLog?.completed_at) return;
		logContainer.scrollTop = logContainer.scrollHeight;
	});

	async function copyLogs() {
		if (!liveLogs) return;
		try {
			await navigator.clipboard.writeText(liveLogs);
		} catch (e) {
			error = 'Failed to copy logs';
		}
	}
</script>

<svelte:head>
	<title>Deploy Log {deployLog?.version || '...'} | Watcher</title>
</svelte:head>

<div class="flex h-[calc(100vh-8rem)] flex-col space-y-6">
	<div class="flex items-center gap-4">
		<a href={resolve(`/watchers/${id}?tab=deploys`)}>
			<Button.Root variant="ghost" size="icon" class="h-8 w-8">
				<ArrowLeft class="h-4 w-4" />
			</Button.Root>
		</a>
		<div class="flex-1">
			<h1 class="text-2xl font-bold tracking-tight">Deploy Log: {deployLog?.version || '...'}</h1>
			{#if watcher}
				<p class="font-mono text-sm text-muted-foreground">
					<a href={resolve(`/watchers/${id}`)} class="hover:underline">{watcher.name}</a>
					({watcher.service_name})
				</p>
			{/if}
		</div>
		{#if deployLog}
			{@const Icon = deployIcon(deployLog.status)}
			<span
				class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-sm font-medium capitalize {statusColor(
					deployLog.status
				)}"
			>
				<Icon class="h-4 w-4" />
				{deployLog.status}
			</span>
			{#if deployLog.github_deployment_id > 0}
				<span
					title="Reported to GitHub"
					class="inline-flex max-w-max items-center rounded border border-muted-foreground/30 bg-muted px-2 py-1 text-xs font-medium text-muted-foreground"
				>
					<svg class="mr-1.5 h-3 w-3" fill="currentColor" viewBox="0 0 24 24"
						><path
							d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"
						/></svg
					>
					GitHub Deploy #{deployLog.github_deployment_id}
				</span>
			{/if}
			<Button.Root variant="outline" size="sm" onclick={copyLogs}>
				<Copy class="mr-2 h-4 w-4" /> Copy Logs
			</Button.Root>
		{/if}
	</div>

	{#if error}
		<div
			class="flex items-center justify-between rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400"
		>
			<span><XCircle class="mr-2 inline h-4 w-4" /> {error}</span>
		</div>
	{/if}

	{#if deployLog}
		<div class="grid flex-none grid-cols-2 gap-4 md:grid-cols-5">
			<Card.Root class="bg-card">
				<Card.Content class="p-4">
					<div class="mb-1 text-xs text-muted-foreground">Previous Version</div>
					<div class="font-mono text-sm">{deployLog.from_version || 'N/A'}</div>
				</Card.Content>
			</Card.Root>
			<Card.Root class="bg-card">
				<Card.Content class="p-4">
					<div class="mb-1 text-xs text-muted-foreground">Triggered By</div>
					<div class="text-sm capitalize">{deployLog.triggered_by || 'agent'}</div>
				</Card.Content>
			</Card.Root>
			<Card.Root class="bg-card">
				<Card.Content class="p-4">
					<div class="mb-1 text-xs text-muted-foreground">Duration</div>
					<div class="text-sm font-medium">{formatDuration(deployLog.duration_ms)}</div>
				</Card.Content>
			</Card.Root>
			<Card.Root class="bg-card">
				<Card.Content class="p-4">
					<div class="mb-1 text-xs text-muted-foreground">Started</div>
					<div class="text-sm">{formatDate(deployLog.started_at)}</div>
				</Card.Content>
			</Card.Root>
			<Card.Root class="bg-card">
				<Card.Content class="p-4">
					<div class="mb-1 text-xs text-muted-foreground">Completed</div>
					<div class="text-sm">{formatDate(deployLog.completed_at)}</div>
				</Card.Content>
			</Card.Root>
		</div>

		{#if deployLog.error}
			<Card.Root class="flex-none border-red-500/30 bg-red-500/5">
				<Card.Header class="border-b border-border/50 pb-3">
					<Card.Title class="flex items-center text-sm font-medium text-red-500">
						<XCircle class="mr-2 h-4 w-4" />
						Deployment Error
					</Card.Title>
				</Card.Header>
				<Card.Content class="p-4">
					<p class="font-mono text-sm wrap-break-word text-red-400">{deployLog.error}</p>
				</Card.Content>
			</Card.Root>
		{/if}

		<Card.Root class="flex flex-1 flex-col overflow-hidden border-border bg-[#0a0a0a]">
			<Card.Header
				class="flex flex-row items-center justify-between border-b border-border/50 bg-card/50 px-4 py-3"
			>
				<Card.Title class="flex items-center gap-2 text-sm font-medium">
					<Clock class="h-4 w-4" />
					Raw Execution Logs
				</Card.Title>
			</Card.Header>
			<Card.Content class="flex-1 overflow-y-auto p-0 font-mono text-sm text-muted-foreground">
				<div class="p-0 leading-relaxed" bind:this={logContainer}>
					{#if liveLogs}
						{#each normalizedLines(liveLogs) as line, idx (idx)}
							<div class="grid grid-cols-[56px_1fr] border-b border-border/40 px-3 py-1.5 text-xs">
								<div class="select-none pr-3 text-right text-zinc-500">{idx + 1}</div>
								<div class={`whitespace-pre-wrap wrap-break-word ${lineTone(line)}`}>{line}</div>
							</div>
						{/each}
					{:else}
						<div class="py-10 text-center text-muted-foreground/50">
							No logs recorded for this deployment.
						</div>
					{/if}
				</div>
			</Card.Content>
		</Card.Root>
	{:else if !error}
		<div class="flex flex-1 items-center justify-center text-muted-foreground">
			<Loader2 class="h-8 w-8 animate-spin" />
		</div>
	{/if}
</div>
