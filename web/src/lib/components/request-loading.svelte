<script lang="ts">
	import { LoaderCircle } from '@lucide/svelte';

	let { label = 'Loading…' } = $props<{ label?: string }>();
</script>

<div
	class="flex min-h-40 flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border bg-card/40 p-6 text-center"
	role="status"
	aria-live="polite"
>
	<LoaderCircle class="h-6 w-6 animate-spin text-primary" />
	<p class="text-sm text-muted-foreground">{label}</p>
</div>
