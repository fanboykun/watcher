<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import * as Tabs from '$lib/components/ui/tabs';
	import { Activity, AlertCircle, RefreshCw } from '@lucide/svelte';

	let agentLines = $state<string[]>([]);
	let error = $state('');
	let loading = $state(false);
	let lineCount = $state(100);

	onMount(() => loadLogs());

	async function loadLogs() {
		loading = true;
		try {
			const res = await api.agentLogs(lineCount);
			agentLines = res.lines ?? [];
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load logs';
			agentLines = [];
		} finally {
			loading = false;
		}
	}
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold tracking-tight">Logs</h1>
			<p class="text-sm text-muted-foreground">Agent log output</p>
		</div>
		<div class="flex items-center gap-2">
			<select
				class="rounded-md border border-border bg-card px-3 py-1.5 text-sm text-foreground"
				bind:value={lineCount}
				onchange={() => loadLogs()}
			>
				<option value={50}>50 lines</option>
				<option value={100}>100 lines</option>
				<option value={200}>200 lines</option>
				<option value={500}>500 lines</option>
			</select>
			<Button.Root variant="outline" size="sm" onclick={loadLogs} disabled={loading}>
				<RefreshCw class="mr-2 h-4 w-4 {loading ? 'animate-spin' : ''}" />
				Refresh
			</Button.Root>
		</div>
	</div>

	{#if error}
		<div class="rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400">
			<AlertCircle class="mr-2 inline h-4 w-4" /> {error}
		</div>
	{/if}

	<Card.Root class="border-border bg-card">
		<Card.Content class="p-0">
			{#if agentLines.length > 0}
				<div class="max-h-[600px] overflow-auto">
					<pre class="p-4 font-mono text-xs leading-relaxed text-muted-foreground">{#each agentLines as line}{line}
{/each}</pre>
				</div>
			{:else if !error}
				<div class="flex flex-col items-center justify-center py-16 text-center">
					<Activity class="mb-3 h-8 w-8 text-muted-foreground/40" />
					<p class="text-sm text-muted-foreground">No log output found</p>
					<p class="mt-1 text-xs text-muted-foreground/60">
						Agent logs will appear here when available
					</p>
				</div>
			{/if}
		</Card.Content>
	</Card.Root>
</div>
