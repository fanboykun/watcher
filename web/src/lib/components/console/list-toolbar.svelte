<script lang="ts">
	import { Search, X } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	let {
		value = $bindable(''),
		label,
		count,
		total,
		children
	}: {
		value?: string;
		label: string;
		count: number;
		total: number;
		children?: Snippet;
	} = $props();
</script>

<div class="list-toolbar">
	<div class="search-field">
		<Search class="size-4 shrink-0 text-muted-foreground" />
		<input type="search" aria-label={label} placeholder={label} bind:value />
		{#if value}<button type="button" aria-label="Clear search" onclick={() => (value = '')}
				><X class="size-4" /></button
			>{/if}
	</div>
	{#if children}{@render children()}{/if}
	<p class="ml-auto font-mono text-xs whitespace-nowrap text-muted-foreground" aria-live="polite">
		{count} / {total} shown
	</p>
</div>
