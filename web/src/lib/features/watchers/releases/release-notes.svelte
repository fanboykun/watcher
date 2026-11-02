<script lang="ts">
	import type { ReleaseMark } from '$lib/api';
	import * as Button from '$lib/components/ui/button';
	import { Ban, ExternalLink } from '@lucide/svelte';
	import { formatDate } from '$lib/utils';
	import { isBlocked, safeReleaseLink } from './release-marks';

	let {
		mark,
		showNotes = true
	}: {
		mark: ReleaseMark | undefined;
		/** The catalog already shows GitHub's notes, so it only needs the block. */
		showNotes?: boolean;
	} = $props();

	const link = $derived(safeReleaseLink(mark?.url));
</script>

{#if mark && isBlocked(mark)}
	<div
		role="note"
		class="flex items-start gap-2 rounded-md border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-400"
	>
		<Ban class="mt-0.5 size-4 shrink-0" />
		<p class="min-w-0 break-words">
			<strong>Blocked{mark.blocked_at ? ` ${formatDate(mark.blocked_at)}` : ''}.</strong>
			{mark.block_reason}
			<span class="block text-xs text-red-400/80"
				>Watcher will not deploy, approve, or roll back to this version.</span
			>
		</p>
	</div>
{/if}
{#if showNotes && mark && (mark.title || mark.notes || link)}
	<div class="space-y-1.5">
		<div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
			{#if mark.title}<span class="font-medium text-foreground">{mark.title}</span>{/if}
			{#if mark.published_at}<span>Published {formatDate(mark.published_at)}</span>{/if}
			{#if link}
				<Button.Root
					href={link}
					target="_blank"
					rel="noopener noreferrer"
					variant="ghost"
					size="xs"
					class="text-muted-foreground"><ExternalLink />Open release</Button.Root
				>
			{/if}
		</div>
		{#if mark.notes}<details class="text-sm">
				<summary class="cursor-pointer text-xs text-muted-foreground hover:text-foreground"
					>Release notes</summary
				>
				<pre
					class="mt-2 max-h-80 overflow-auto rounded-md bg-muted/30 p-3 font-sans text-sm leading-relaxed whitespace-pre-wrap">{mark.notes}</pre>
			</details>{/if}
	</div>
{/if}
