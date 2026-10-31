<script lang="ts">
	import type { Snippet } from 'svelte';
	let {
		title,
		description,
		eyebrow = 'Workspace',
		actions
	}: {
		title: string;
		description: string;
		eyebrow?: string;
		actions?: Snippet;
	} = $props();
</script>

<header class="console-heading">
	<div class="min-w-0">
		<p class="eyebrow mb-3">{eyebrow}</p>
		<h1>{title}</h1>
		<p class="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">{description}</p>
	</div>
	{#if actions}<div class="page-actions">{@render actions()}</div>{/if}
</header>
