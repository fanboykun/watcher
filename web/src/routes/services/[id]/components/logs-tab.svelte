<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import * as Select from '$lib/components/ui/select';
	import { RefreshCw, FileText } from '@lucide/svelte';

	let {
		logLines = $bindable([]),
		logError = $bindable(''),
		logType = $bindable<'out' | 'err'>('out'),
		logCount = $bindable(100),
		onLoadLogs
	}: {
		logLines: string[];
		logError: string;
		logType: 'out' | 'err';
		logCount: number;
		onLoadLogs: () => Promise<void>;
	} = $props();
</script>

<div class="mb-3 flex items-center gap-2">
	<Select.Root
		type="single"
		value={logType}
		onValueChange={(v) => {
			if (v === 'out' || v === 'err') {
				logType = v;
				onLoadLogs();
			}
		}}
	>
		<Select.Trigger class="w-[120px] text-sm" />
		<Select.Content>
			<Select.Item value="out">stdout</Select.Item>
			<Select.Item value="err">stderr</Select.Item>
		</Select.Content>
	</Select.Root>
	<Select.Root
		type="single"
		value={String(logCount)}
		onValueChange={(v) => {
			if (v) {
				logCount = Number(v);
				onLoadLogs();
			}
		}}
	>
		<Select.Trigger class="w-[120px] text-sm" />
		<Select.Content>
			<Select.Item value="50">50 lines</Select.Item>
			<Select.Item value="100">100 lines</Select.Item>
			<Select.Item value="200">200 lines</Select.Item>
			<Select.Item value="500">500 lines</Select.Item>
		</Select.Content>
	</Select.Root>
	<Button.Root variant="outline" size="sm" onclick={onLoadLogs}>
		<RefreshCw class="mr-2 h-4 w-4" /> Refresh
	</Button.Root>
</div>

{#if logError}
	<div
		class="mb-3 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-400"
	>
		{logError}
	</div>
{/if}

<Card.Root class="border-border bg-card">
	<Card.Content class="p-0">
		{#if logLines.length > 0}
			<div class="max-h-[500px] overflow-auto">
				<pre class="p-4 font-mono text-xs leading-relaxed text-muted-foreground">{#each logLines as line, i (i)}{line}
{/each}</pre>
			</div>
		{:else if !logError}
			<div class="flex flex-col items-center justify-center py-12 text-center">
				<FileText class="mb-3 h-8 w-8 text-muted-foreground/40" />
				<p class="text-sm text-muted-foreground">No log output</p>
			</div>
		{/if}
	</Card.Content>
</Card.Root>
