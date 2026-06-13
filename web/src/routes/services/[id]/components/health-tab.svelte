<script lang="ts">
	import type { HealthEvent } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import { Heart, CheckCircle2, XCircle } from '@lucide/svelte';
	import { healthBadgeColor, formatDate } from '$lib/utils';

	let { healthHistory = [] }: { healthHistory: HealthEvent[] } = $props();
</script>

{#if healthHistory.length > 0}
	<Card.Root class="border-border bg-card">
		<Table.Root>
			<Table.Header>
				<Table.Row class="border-border hover:bg-transparent">
					<Table.Head>Status</Table.Head>
					<Table.Head>HTTP</Table.Head>
					<Table.Head>Error</Table.Head>
					<Table.Head>Checked At</Table.Head>
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#each healthHistory as h (h.id)}
					<Table.Row class="border-border">
						<Table.Cell>
							<span
								class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-medium capitalize {healthBadgeColor(
									h.status
								)}"
							>
								{#if h.status === 'healthy'}<CheckCircle2 class="h-3 w-3" />{:else}<XCircle
										class="h-3 w-3"
									/>{/if}
								{h.status}
							</span>
						</Table.Cell>
						<Table.Cell class="font-mono text-sm text-muted-foreground"
							>{h.http_status || '—'}</Table.Cell
						>
						<Table.Cell class="max-w-[250px] truncate text-xs text-red-400"
							>{h.error || ''}</Table.Cell
						>
						<Table.Cell class="text-muted-foreground">{formatDate(h.checked_at)}</Table.Cell>
					</Table.Row>
				{/each}
			</Table.Body>
		</Table.Root>
	</Card.Root>
{:else}
	<Card.Root class="border-dashed border-border bg-card">
		<Card.Content class="flex flex-col items-center justify-center py-12 text-center">
			<Heart class="mb-3 h-8 w-8 text-muted-foreground/40" />
			<p class="text-sm text-muted-foreground">No health checks recorded</p>
			<p class="mt-1 text-xs text-muted-foreground/60">Click "Health" to run a check</p>
		</Card.Content>
	</Card.Root>
{/if}
