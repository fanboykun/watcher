<script lang="ts">
	import type { DeployLog, Watcher } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Button from '$lib/components/ui/button';
	import { Select } from '$lib/components/ui/select';
	import { Activity, ExternalLink } from '@lucide/svelte';
	import { resolve } from '$app/paths';
	import { statusColor, formatDate, formatDuration } from '$lib/utils';

	let {
		deploys = $bindable([]),
		deployPage = $bindable(1),
		deployPageSize = $bindable(10),
		deployTotal = 0,
		watcher,
		onLoadDeploys
	}: {
		deploys: DeployLog[];
		deployPage: number;
		deployPageSize: number;
		deployTotal: number;
		watcher: Watcher | null;
		onLoadDeploys: () => Promise<void>;
	} = $props();
</script>

<div class="mb-3 flex items-center justify-between gap-2">
	<div class="text-xs text-muted-foreground">
		Showing {deploys.length === 0
			? 0
			: (deployPage - 1) * deployPageSize + 1} - {Math.min(
			deployPage * deployPageSize,
			deployTotal
		)} of {deployTotal}
	</div>
	<div class="flex items-center gap-2">
		<Select
			class="w-auto min-w-[110px] text-xs"
			bind:value={deployPageSize}
			onchange={async () => {
				deployPage = 1;
				await onLoadDeploys();
			}}
		>
			<option value={10}>10 / page</option>
			<option value={25}>25 / page</option>
			<option value={50}>50 / page</option>
		</Select>
		<Button.Root
			variant="outline"
			size="sm"
			disabled={deployPage <= 1}
			onclick={async () => {
				if (deployPage <= 1) return;
				deployPage -= 1;
				await onLoadDeploys();
			}}
		>
			Prev
		</Button.Root>
		<Button.Root
			variant="outline"
			size="sm"
			disabled={deployPage * deployPageSize >= deployTotal}
			onclick={async () => {
				if (deployPage * deployPageSize >= deployTotal) return;
				deployPage += 1;
				await onLoadDeploys();
			}}
		>
			Next
		</Button.Root>
	</div>
</div>
{#if deploys.length > 0}
	<Card.Root class="border-border bg-card">
		<Table.Root>
			<Table.Header>
				<Table.Row class="border-border hover:bg-transparent">
					<Table.Head>Status</Table.Head>
					<Table.Head>Triggered By</Table.Head>
					<Table.Head>Version</Table.Head>
					<Table.Head>From</Table.Head>
					<Table.Head>Duration</Table.Head>
					<Table.Head>Started</Table.Head>
					<Table.Head>Error</Table.Head>
					<Table.Head class="text-right">Action</Table.Head>
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#each deploys as d (d.id)}
					<Table.Row class="border-border">
						<Table.Cell>
							<span
								class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-medium capitalize {statusColor(
									d.status
								)}"
							>
								{d.status}
							</span>
						</Table.Cell>
						<Table.Cell class="text-xs capitalize text-muted-foreground"
							>{d.triggered_by || 'agent'}</Table.Cell
						>
						<Table.Cell class="font-mono text-sm">{d.version}</Table.Cell>
						<Table.Cell class="font-mono text-xs text-muted-foreground"
							>{d.from_version || '—'}</Table.Cell
						>
						<Table.Cell class="text-muted-foreground">{formatDuration(d.duration_ms)}</Table.Cell>
						<Table.Cell class="text-muted-foreground">{formatDate(d.started_at)}</Table.Cell>
						<Table.Cell class="max-w-[250px] truncate text-xs text-red-400"
							>{d.error || ''}</Table.Cell
						>
						<Table.Cell class="text-right">
							<div class="flex items-center justify-end gap-2">
								{#if d.github_deployment_id > 0}
									<span
										title="Reported to GitHub"
										class="inline-flex items-center rounded border border-muted-foreground/20 bg-muted/30 px-1 py-0.5 text-[10px] text-muted-foreground/70"
									>
										<svg class="mr-1 h-3 w-3" fill="currentColor" viewBox="0 0 24 24"
											><path
												d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"
											/></svg
										>
										GitHub
									</span>
								{/if}
								{#if watcher}
									<a
										href={resolve(`/watchers/${watcher.id}/logs/${d.id}`)}
										class="inline-flex h-8 items-center justify-center rounded-md border border-input bg-background px-3 text-xs font-medium hover:bg-accent hover:text-accent-foreground"
									>
										Logs <ExternalLink class="ml-1.5 h-3 w-3 text-muted-foreground" />
									</a>
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
			<Activity class="mb-3 h-8 w-8 text-muted-foreground/40" />
			<p class="text-sm text-muted-foreground">No deployments</p>
		</Card.Content>
	</Card.Root>
{/if}
