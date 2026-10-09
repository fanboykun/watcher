<script lang="ts">
	import { fieldText, logTime, type LogEntry } from '$lib/logs';
	import { ChevronRight, Braces, Layers } from '@lucide/svelte';
	let { entries }: { entries: LogEntry[] } = $props();
	function levelColor(level: string) {
		if (level === 'error' || level === 'fatal')
			return 'border-red-500/20 bg-red-500/10 text-red-400';
		if (level === 'warn') return 'border-amber-500/20 bg-amber-500/10 text-amber-400';
		if (level === 'info') return 'border-blue-500/20 bg-blue-500/10 text-blue-400';
		return 'border-border bg-muted/50 text-muted-foreground';
	}
</script>

<div class="divide-y divide-border">
	{#each entries as entry (entry.id)}
		<details class="group" data-log-level={entry.level}>
			<summary
				class="flex cursor-pointer list-none items-start gap-3 px-4 py-3 transition-colors hover:bg-muted/30 [&::-webkit-details-marker]:hidden"
			>
				<ChevronRight
					class="mt-1 size-3.5 shrink-0 text-muted-foreground transition-transform group-open:rotate-90"
				/>
				<div class="min-w-0 flex-1 space-y-1.5">
					<div class="flex flex-wrap items-center gap-2">
						<time
							class="font-mono text-[11px] text-muted-foreground tabular-nums"
							title={entry.time}>{logTime(entry.time)}</time
						>
						<span
							class={`rounded border px-1.5 py-0.5 font-mono text-[10px] uppercase ${levelColor(entry.level)}`}
							>{entry.level === 'unknown' ? 'text' : entry.level}</span
						>
						{#if entry.component}<span class="text-[11px] text-muted-foreground"
								>{entry.component}</span
							>{/if}
						{#if entry.stack}<span class="ml-auto flex items-center gap-1 text-[10px] text-red-400"
								><Layers class="size-3" />Stack trace</span
							>{/if}
					</div>
					<p class="font-mono text-xs leading-relaxed break-words whitespace-pre-wrap">
						{entry.message}
					</p>
				</div>
			</summary>
			<div class="space-y-4 border-t border-border bg-background/40 px-4 py-4 sm:pl-11">
				{#if Object.keys(entry.fields).length}
					<dl class="grid min-w-0 gap-x-5 gap-y-2 text-xs sm:grid-cols-[minmax(100px,auto)_1fr]">
						{#each Object.entries(entry.fields) as [key, value] (key)}
							<dt class="font-mono text-muted-foreground">{key}</dt>
							<dd class="min-w-0 font-mono break-words whitespace-pre-wrap">{fieldText(value)}</dd>
						{/each}
					</dl>
				{/if}
				{#if entry.stack}
					<div class="overflow-hidden rounded-lg border border-red-500/20">
						<p
							class="border-b border-red-500/20 bg-red-500/5 px-3 py-2 text-[11px] font-medium text-red-400"
						>
							Stack trace / continuation
						</p>
						<pre
							class="max-h-80 overflow-auto p-3 font-mono text-xs leading-relaxed text-foreground/80">{entry.stack}</pre>
					</div>
				{/if}
				<details class="text-xs text-muted-foreground">
					<summary class="flex cursor-pointer list-none items-center gap-1.5"
						><Braces class="size-3.5" />Original record</summary
					>
					<pre
						class="mt-2 max-h-80 overflow-auto rounded-md border border-border p-3 font-mono text-[11px] leading-relaxed">{entry.raw}</pre>
				</details>
			</div>
		</details>
	{/each}
</div>
