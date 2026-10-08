<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import * as Button from '$lib/components/ui/button';
	import { CheckCircle2, RotateCcw, Trash2, Server, Archive } from '@lucide/svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { filesize } from 'filesize';
	import { formatDate } from '$lib/utils';

	let {
		busy = false,
		versions,
		onRollback,
		onDeleteVersion
	}: {
		versions: import('$lib/api').ReleaseInfo[];
		busy?: boolean;
		onRollback: (version: string) => void;
		onDeleteVersion: (version: string) => void;
	} = $props();
</script>

{#if versions && versions.length > 0}
	<Card.Root>
		<Card.Header
			><Card.Title>Retained releases</Card.Title><Card.Description
				>Inspect saved configuration or restore a release.</Card.Description
			></Card.Header
		>
		<Card.Content class="divide-y divide-border">
			{#each versions as v (v.version)}
				<article class="space-y-4 py-4 first:pt-0 last:pb-0" aria-label={`Release ${v.version}`}>
					<div class="section-toolbar">
						<div class="min-w-0 space-y-1">
							<div class="flex flex-wrap items-center gap-2">
								<h3 class="font-mono text-sm font-semibold break-all">{v.version}</h3>
								{#if v.is_current}<Badge
										variant="secondary"
										class="bg-emerald-500/15 text-emerald-400"><CheckCircle2 />Current</Badge
									>{/if}
							</div>
							<p class="text-xs text-muted-foreground">
								{formatDate(v.mod_time)} · {v.size_bytes > 0
									? filesize(v.size_bytes)
									: v.size_human || '0 B'}
							</p>
						</div>
						<div class="flex flex-wrap items-center gap-2">
							{#if v.has_snapshot}
								<Button.Root
									href={resolve('/watchers/[id]/versions/[version]', {
										id: page.params.id!,
										version: encodeURIComponent(v.version)
									})}
									variant="outline"
									size="sm"
									aria-label={`View configuration for ${v.version}`}
									><Archive />View configuration</Button.Root
								>
							{:else}<span class="text-xs text-muted-foreground">No configuration snapshot</span
								>{/if}
							{#if !v.is_current}
								<div class="flex flex-wrap gap-2 border-l border-border pl-2">
									<Button.Root
										variant="outline"
										size="sm"
										disabled={busy}
										aria-label={`Roll back to ${v.version}`}
										onclick={() => onRollback(v.version)}
										><RotateCcw class="text-amber-400" />Roll back</Button.Root
									>
									<Button.Root
										variant="destructive"
										size="sm"
										disabled={busy}
										aria-label={`Delete version ${v.version}`}
										onclick={() => onDeleteVersion(v.version)}><Trash2 />Delete version</Button.Root
									>
								</div>
							{/if}
						</div>
					</div>
				</article>
			{/each}
		</Card.Content>
	</Card.Root>
{:else}
	<Card.Root class="border-dashed border-border bg-card">
		<Card.Content class="flex flex-col items-center justify-center py-12 text-center">
			<Server class="mb-3 h-8 w-8 text-muted-foreground/40" />
			<p class="text-sm text-muted-foreground">No extracted versions on disk</p>
		</Card.Content>
	</Card.Root>
{/if}
