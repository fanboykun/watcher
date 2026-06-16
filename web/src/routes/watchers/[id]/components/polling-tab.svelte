<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Button from '$lib/components/ui/button';
	import * as Select from '$lib/components/ui/select';
	import { Clock } from '@lucide/svelte';
	import { timeAgo } from '$lib/utils';

	let {
		polls,
		pollPage = $bindable(1),
		pollPageSize,
		pollTotal,
		pollStatus = $bindable('all'),
		onPageChange,
		onStatusChange
	}: {
		polls: import('$lib/api').PollEvent[];
		pollPage: number;
		pollPageSize: number;
		pollTotal: number;
		pollStatus: string;
		onPageChange: (page: number) => void;
		onStatusChange: (status: string) => void;
	} = $props();
</script>

{#if polls && polls.length > 0}
	<Card.Root class="border-border bg-card">
		<Table.Root>
			<Table.Header>
				<Table.Row class="border-border hover:bg-transparent">
					<Table.Head>Date</Table.Head>
					<Table.Head>Status</Table.Head>
					<Table.Head>Remote Version</Table.Head>
					<Table.Head>Error</Table.Head>
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#each polls as p (p.id)}
					<Table.Row class="border-border">
						<Table.Cell class="text-muted-foreground">
							<span title={p.checked_at}>{timeAgo(p.checked_at)}</span>
						</Table.Cell>
						<Table.Cell>
							<span
								class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-medium capitalize
								{p.status === 'new_release'
									? 'border-blue-500/30 bg-blue-500/15 text-blue-400'
									: p.status === 'error'
										? 'border-red-500/30 bg-red-500/15 text-red-400'
										: 'border-border bg-muted text-muted-foreground'}"
							>
								{p.status.replace('_', ' ')}
							</span>
						</Table.Cell>
						<Table.Cell class="font-mono text-sm">{p.remote_version || '—'}</Table.Cell>
						<Table.Cell class="max-w-[300px] truncate text-xs text-red-400" title={p.error}>
							{p.error || '—'}
						</Table.Cell>
					</Table.Row>
				{/each}
			</Table.Body>
		</Table.Root>
		<div class="mt-auto flex items-center justify-between border-t border-border px-4 py-4">
			<div class="flex items-center gap-2 text-xs text-muted-foreground">
				<span>Status Filter:</span>
				<Select.Root
					type="single"
					value={pollStatus}
					onValueChange={(v) => {
						if (v) {
							pollStatus = v;
							onStatusChange(v);
						}
					}}
				>
					<Select.Trigger class="h-8 w-32 text-xs" />
					<Select.Content>
						<Select.Item value="all">All</Select.Item>
						<Select.Item value="new_release">New Release</Select.Item>
						<Select.Item value="up_to_date">Up To Date</Select.Item>
						<Select.Item value="error">Error</Select.Item>
					</Select.Content>
				</Select.Root>
			</div>
			<div class="flex items-center gap-4 text-xs">
				<span class="text-muted-foreground">
					Page {pollPage} of {Math.ceil(pollTotal / pollPageSize) || 1}
					({pollTotal} total)
				</span>
				<div class="flex items-center gap-1.5">
					<Button.Root
						variant="outline"
						size="sm"
						class="h-7 px-2"
						disabled={pollPage <= 1}
						onclick={() => onPageChange(pollPage - 1)}
					>
						Prev
					</Button.Root>
					<Button.Root
						variant="outline"
						size="sm"
						class="h-7 px-2"
						disabled={pollPage * pollPageSize >= pollTotal}
						onclick={() => onPageChange(pollPage + 1)}
					>
						Next
					</Button.Root>
				</div>
			</div>
		</div>
	</Card.Root>
{:else}
	<Card.Root class="border-dashed border-border bg-card">
		<Card.Content class="flex flex-col items-center justify-center py-12 text-center">
			<Clock class="mb-3 h-8 w-8 text-muted-foreground/40" />
			<p class="text-sm text-muted-foreground">No polling history yet</p>
		</Card.Content>
	</Card.Root>
{/if}
