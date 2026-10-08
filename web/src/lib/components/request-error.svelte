<script lang="ts">
	import { AlertCircle } from '@lucide/svelte';
	import * as Button from '$lib/components/ui/button';
	let {
		message,
		title = 'Request could not be completed',
		onRetry
	}: {
		message: string;
		title?: string;
		onRetry?: () => void | Promise<void>;
	} = $props();
</script>

{#if message}
	<div
		role="alert"
		class="flex min-w-0 items-start gap-3 rounded-lg border border-red-500/30 bg-red-500/5 p-3"
	>
		<AlertCircle class="mt-0.5 size-4 shrink-0 text-red-400" />
		<div class="min-w-0 flex-1 space-y-1">
			<p class="text-sm font-medium">{title}</p>
			<p class="text-sm break-words whitespace-pre-wrap text-muted-foreground">{message}</p>
			{#if onRetry}<Button.Root variant="outline" size="sm" class="mt-2" onclick={onRetry}
					>Try again</Button.Root
				>{/if}
		</div>
	</div>
{/if}
