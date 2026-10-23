<script lang="ts">
	import type { CatalogDownload } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { Badge } from '$lib/components/ui/badge';
	import { ChevronDown, Download, SlidersHorizontal, Rocket } from '@lucide/svelte';
	import { formatDate } from '$lib/utils';

	let {
		downloads,
		busy = false,
		onConfigure,
		onDeploy
	}: {
		downloads: CatalogDownload[];
		busy?: boolean;
		onConfigure: (download: CatalogDownload) => Promise<void>;
		onDeploy: (download: CatalogDownload) => void;
	} = $props();
</script>

{#if downloads.length > 0}
	<Card.Root>
		<Card.Header>
			<Card.Title>Downloaded versions</Card.Title>
			<Card.Description
				>Artifacts saved in this watcher's downloads directory, ready to configure or deploy.</Card.Description
			>
		</Card.Header>
		<Card.Content class="divide-y divide-border">
			{#each downloads as download (download.id)}
				<article
					class="section-toolbar py-4 first:pt-0 last:pb-0"
					aria-label={`Downloaded version ${download.version}`}
				>
					<div class="min-w-0 space-y-1.5">
						<div class="flex flex-wrap items-center gap-2">
							<Download class="size-4 text-muted-foreground" />
							<h3 class="font-mono text-sm font-semibold break-all">{download.version}</h3>
							<Badge variant="secondary">Ready to deploy</Badge>
						</div>
						<p class="font-mono text-xs break-all text-muted-foreground">{download.asset_name}</p>
						<p class="text-xs text-muted-foreground">
							Tag {download.tag} · Downloaded {formatDate(download.downloaded_at)}
						</p>
					</div>
					<DropdownMenu.Root>
						<DropdownMenu.Trigger
							class="inline-flex h-8 shrink-0 items-center gap-2 rounded-md border border-border px-3 text-sm hover:bg-muted disabled:opacity-50"
							disabled={busy}
							aria-label={`Actions for downloaded ${download.version}`}
							>Actions<ChevronDown class="size-3.5" /></DropdownMenu.Trigger
						>
						<DropdownMenu.Content align="end">
							<DropdownMenu.Item onclick={() => onConfigure(download)}
								><SlidersHorizontal />Configure candidate</DropdownMenu.Item
							>
							<DropdownMenu.Item onclick={() => onDeploy(download)}
								><Rocket />Deploy release…</DropdownMenu.Item
							>
						</DropdownMenu.Content>
					</DropdownMenu.Root>
				</article>
			{/each}
		</Card.Content>
	</Card.Root>
{/if}
