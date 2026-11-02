<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import { Server } from '@lucide/svelte';
	import type { CatalogDownload } from '$lib/api';
	import VersionRowView from '$lib/features/watchers/releases/version-row.svelte';
	import ReleaseBlockControl from '$lib/features/watchers/releases/release-block-control.svelte';
	import type { VersionRow } from '$lib/features/watchers/releases/version-rows';

	let {
		watcherId,
		rows,
		busy = false,
		onRollback,
		onDeleteVersion,
		onConfigure,
		onDeploy
	}: {
		watcherId: number;
		rows: VersionRow[];
		busy?: boolean;
		onRollback: (version: string) => void;
		onDeleteVersion: (version: string) => void;
		onConfigure: (download: CatalogDownload) => void;
		onDeploy: (download: CatalogDownload) => void;
	} = $props();
</script>

<Card.Root>
	<Card.Header class="section-toolbar">
		<div class="space-y-1.5">
			<Card.Title>Versions on this host</Card.Title>
			<Card.Description
				>Releases deployed before, downloaded from GitHub, or blocked. Roll back, edit a version's
				configuration, or block a version Watcher must never deploy.</Card.Description
			>
		</div>
		<ReleaseBlockControl {watcherId} disabled={busy} />
	</Card.Header>
	<Card.Content class="divide-y divide-border">
		{#each rows as row (row.version)}
			<VersionRowView
				{row}
				{watcherId}
				{busy}
				{onRollback}
				onDelete={onDeleteVersion}
				{onConfigure}
				{onDeploy}
			/>
		{:else}
			<div class="flex flex-col items-center justify-center py-12 text-center">
				<Server class="mb-3 h-8 w-8 text-muted-foreground/40" />
				<p class="text-sm text-muted-foreground">No versions on this host yet</p>
			</div>
		{/each}
	</Card.Content>
</Card.Root>
