<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Button from '$lib/components/ui/button';
	import { CheckCircle2, RotateCcw, Trash2, Server } from '@lucide/svelte';
	import { filesize } from 'filesize';
	import { formatDate } from '$lib/utils';

	let {
		versions,
		onRollback,
		onDeleteVersion
	}: {
		versions: import('$lib/api').ReleaseInfo[];
		onRollback: (version: string) => void;
		onDeleteVersion: (version: string) => void;
	} = $props();
</script>

{#if versions && versions.length > 0}
	<Card.Root class="border-border bg-card">
		<Table.Root>
			<Table.Header>
				<Table.Row class="border-border hover:bg-transparent">
					<Table.Head>Version</Table.Head>
					<Table.Head>Modified At</Table.Head>
					<Table.Head>Size</Table.Head>
					<Table.Head>Status</Table.Head>
					<Table.Head class="text-right">Action</Table.Head>
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#each versions as v (v.version)}
					<Table.Row class="border-border">
						<Table.Cell class="font-mono text-sm font-medium">{v.version}</Table.Cell>
						<Table.Cell class="text-muted-foreground">{formatDate(v.mod_time)}</Table.Cell>
						<Table.Cell class="text-muted-foreground">
							{v.size_bytes > 0 ? filesize(v.size_bytes) : v.size_human || '0 B'}
						</Table.Cell>
						<Table.Cell>
							{#if v.is_current}
								<span class="inline-flex items-center gap-1 rounded bg-emerald-500/15 px-2 py-0.5 text-xs font-medium text-emerald-400">
									<CheckCircle2 class="h-3 w-3" />
									Current
								</span>
							{:else}
								<span class="inline-flex items-center gap-1 rounded bg-muted/50 px-2 py-0.5 text-xs font-medium text-muted-foreground">
									Inactive
								</span>
							{/if}
						</Table.Cell>
						<Table.Cell class="text-right">
							<div class="flex items-center justify-end gap-2">
								{#if !v.is_current}
									<Button.Root
										variant="outline"
										size="sm"
										class="h-8"
										onclick={() => onRollback(v.version)}
									>
										<RotateCcw class="mr-1.5 h-3 w-3" />
										Rollback
									</Button.Root>
									<Button.Root
										variant="default"
										size="sm"
										class="h-8 bg-red-500/10 text-red-500 hover:bg-red-500/20"
										title="Delete Version"
										onclick={() => onDeleteVersion(v.version)}
									>
										<Trash2 class="h-3 w-3" />
									</Button.Root>
								{/if}
							</div>
						</Table.Cell>
					</Table.Row>
				{/each}
			</Table.Body>
		</Table.Root>
	</Card.Root>
{:else}
	<Card.Root class="border-dashed border-border bg-card">
		<Card.Content class="flex flex-col items-center justify-center py-12 text-center">
			<Server class="mb-3 h-8 w-8 text-muted-foreground/40" />
			<p class="text-sm text-muted-foreground">No extracted versions on disk</p>
		</Card.Content>
	</Card.Root>
{/if}
