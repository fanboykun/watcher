<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import type { SelfUpdateCheckResponse } from '$lib/api';
	import { ArrowUpRight, Download, X } from '@lucide/svelte';
	import * as Button from '$lib/components/ui/button';
	let {
		info,
		busy,
		ondismiss
	}: { info: SelfUpdateCheckResponse; busy: boolean; ondismiss: () => void } = $props();
</script>

<div class="update-notice" role="status">
	<Download class="size-4 shrink-0 text-primary" />
	<p class="flex-1 text-sm">
		Watcher <span class="font-mono text-primary">{info.latest_version}</span> is ready.
		<span class="text-muted-foreground">Running {info.current_version}.</span>
	</p>
	<Button.Root
		onclick={() => goto(resolve('/settings?tab=maintenance&action=update'))}
		variant="outline"
		size="sm"
		disabled={busy}
		>{busy ? 'Updating…' : 'Update Watcher'}<ArrowUpRight class="size-3" /></Button.Root
	>
	<Button.Root variant="ghost" size="icon-sm" onclick={ondismiss} aria-label="Dismiss update notice"
		><X class="size-4" /></Button.Root
	>
</div>
