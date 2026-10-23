<script lang="ts">
	import type { Component } from 'svelte';
	let {
		label,
		value,
		detail,
		icon: Icon,
		accent = false
	}: {
		label: string;
		value: string | number;
		detail: string;
		icon: Component<{ class?: string }>;
		accent?: boolean;
	} = $props();
</script>

<div class={['console-metric', accent && 'console-metric-accent']}>
	<div class="flex items-center justify-between gap-3">
		<p class="eyebrow">{label}</p>
		<Icon class="size-4 text-muted-foreground" />
	</div>
	<p class="metric-value">{value}</p>
	<p class="text-xs text-muted-foreground">{detail}</p>
</div>
