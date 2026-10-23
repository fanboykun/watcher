<script lang="ts">
	import type { Component, Snippet } from 'svelte';
	let {
		title,
		description,
		icon: Icon,
		actions
	}: {
		title: string;
		description: string;
		icon: Component<{ class?: string }>;
		actions?: Snippet;
	} = $props();
</script>

<div class="console-empty">
	<div class="empty-symbol"><Icon class="size-6" /></div>
	<h2 class="mt-5 text-base font-semibold">{title}</h2>
	<p class="mt-2 max-w-md text-sm leading-relaxed text-muted-foreground">{description}</p>
	{#if actions}<div class="mt-5">{@render actions()}</div>{/if}
</div>
