<script lang="ts">
	import type { CatalogDownload } from '$lib/api';
	import { Badge } from '$lib/components/ui/badge';
	import * as Button from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import {
		Archive,
		CheckCircle2,
		ChevronDown,
		RotateCcw,
		Rocket,
		SlidersHorizontal,
		Trash2
	} from '@lucide/svelte';
	import { resolve } from '$app/paths';
	import { filesize } from 'filesize';
	import { formatDate } from '$lib/utils';
	import ReleaseNotes from './release-notes.svelte';
	import ReleaseBlockControl from './release-block-control.svelte';
	import type { VersionRow } from './version-rows';

	let {
		row,
		watcherId,
		busy = false,
		onRollback,
		onDelete,
		onConfigure,
		onDeploy
	}: {
		row: VersionRow;
		watcherId: number;
		busy?: boolean;
		onRollback: (version: string) => void;
		onDelete: (version: string) => void;
		onConfigure: (download: CatalogDownload) => void;
		onDeploy: (download: CatalogDownload) => void;
	} = $props();

	const retained = $derived(row.retained);
	const download = $derived(row.download);
	const canRollback = $derived(Boolean(retained) && !row.current);
	const hasActions = $derived(canRollback || Boolean(download));
</script>

<article class="space-y-3 py-4 first:pt-0 last:pb-0" aria-label={`Version ${row.version}`}>
	<div class="section-toolbar">
		<div class="min-w-0 space-y-1">
			<div class="flex flex-wrap items-center gap-2">
				<h3 class="font-mono text-sm font-semibold break-all">{row.version}</h3>
				{#if row.current}<Badge variant="secondary" class="bg-emerald-500/15 text-emerald-400"
						><CheckCircle2 />Current</Badge
					>{:else if retained}<Badge variant="outline">Deployed before</Badge>{/if}
				{#if download && !retained}<Badge variant="outline">Downloaded</Badge>{/if}
				{#if !retained && !download}<Badge variant="outline">Not on this host</Badge>{/if}
				{#if row.blocked}<Badge variant="destructive">Blocked</Badge>{/if}
				{#if row.hasDraft}<Badge variant="secondary">Edited configuration</Badge>{/if}
			</div>
			<p class="text-xs text-muted-foreground">
				{#if retained}Retained {formatDate(retained.mod_time)} · {retained.size_bytes > 0
						? filesize(retained.size_bytes)
						: retained.size_human || '0 B'}{/if}
				{#if retained && download}<span aria-hidden="true"> · </span>{/if}
				{#if download}Downloaded {formatDate(download.downloaded_at)} ·
					<span class="font-mono break-all">{download.asset_name}</span>{/if}
			</p>
		</div>
		<div class="flex flex-wrap items-center gap-2">
			{#if retained?.has_snapshot}
				<Button.Root
					href={resolve('/watchers/[id]/versions/[version]', {
						id: String(watcherId),
						version: encodeURIComponent(row.version)
					})}
					variant="outline"
					size="sm"
					><Archive />{row.current || row.blocked
						? 'View configuration'
						: 'Edit configuration'}</Button.Root
				>
			{:else if retained}<span class="text-xs text-muted-foreground">No configuration snapshot</span
				>{/if}
			<ReleaseBlockControl {watcherId} version={row.version} mark={row.mark} disabled={busy} />
			{#if hasActions}
				<DropdownMenu.Root>
					<DropdownMenu.Trigger
						class="inline-flex h-8 shrink-0 items-center gap-2 rounded-md border border-border px-3 text-sm hover:bg-muted disabled:opacity-50"
						disabled={busy}
						aria-label={`Actions for ${row.version}`}
						>Actions<ChevronDown class="size-3.5" /></DropdownMenu.Trigger
					>
					<DropdownMenu.Content align="end">
						{#if canRollback}<DropdownMenu.Item
								disabled={row.blocked}
								onclick={() => onRollback(row.version)}
								><RotateCcw class="text-amber-400" />Roll back…</DropdownMenu.Item
							>{/if}
						{#if download}
							<DropdownMenu.Item disabled={row.blocked} onclick={() => onConfigure(download)}
								><SlidersHorizontal />Configure candidate</DropdownMenu.Item
							>
							<DropdownMenu.Item disabled={row.blocked} onclick={() => onDeploy(download)}
								><Rocket />Deploy downloaded artifact…</DropdownMenu.Item
							>
						{/if}
						{#if canRollback}<DropdownMenu.Item
								variant="destructive"
								onclick={() => onDelete(row.version)}><Trash2 />Delete from disk</DropdownMenu.Item
							>{/if}
					</DropdownMenu.Content>
				</DropdownMenu.Root>
			{/if}
		</div>
	</div>
	<ReleaseNotes mark={row.mark} />
</article>
