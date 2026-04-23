<script lang="ts">
	import { onMount } from 'svelte';
	import { api, isIISService, serviceTypeLabel, iisAppKindLabel, type ServiceWithWatcher } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Button from '$lib/components/ui/button';
	import { Server, Play, Square, RefreshCw, Heart, AlertCircle } from '@lucide/svelte';
	import { resolve } from '$app/paths';

	let services = $state<ServiceWithWatcher[]>([]);
	let error = $state('');
	let actionMsg = $state('');

	onMount(load);

	async function load() {
		try {
			services = await api.listServices();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load services';
		}
	}

	async function serviceAction(fn: () => Promise<{ message: string }>) {
		try {
			const res = await fn();
			actionMsg = res.message;
			setTimeout(() => (actionMsg = ''), 3000);
		} catch (e) {
			actionMsg = e instanceof Error ? e.message : 'Action failed';
			setTimeout(() => (actionMsg = ''), 5000);
		}
	}
</script>

<div class="space-y-6">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">Services</h1>
		<p class="text-sm text-muted-foreground">All managed services across NSSM and IIS watchers</p>
	</div>

	{#if error}
		<div class="rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400">
			<AlertCircle class="mr-2 inline h-4 w-4" />
			{error}
		</div>
	{/if}

	{#if actionMsg}
		<div class="rounded-lg border border-blue-500/30 bg-blue-500/10 p-4 text-sm text-blue-400">
			{actionMsg}
		</div>
	{/if}

	{#if services.length > 0}
		<Card.Root class="border-border bg-card">
			<Table.Root>
				<Table.Header>
					<Table.Row class="border-border hover:bg-transparent">
						<Table.Head>Service</Table.Head>
						<Table.Head>Watcher</Table.Head>
						<Table.Head>Mode</Table.Head>
						<Table.Head>Target</Table.Head>
						<Table.Head>Health URL</Table.Head>
						<Table.Head class="text-right">Actions</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each services as svc (svc.id)}
						<Table.Row class="border-border">
							<Table.Cell>
								<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
								<a href="/services/{svc.id}" class="font-medium hover:underline"
									>{svc.windows_service_name}</a
								>
							</Table.Cell>
							<Table.Cell>
								<a
									href={resolve(`/watchers/${svc.watcher_id}`)}
									class="text-sm text-muted-foreground hover:underline"
								>
									{svc.watcher_name}
								</a>
							</Table.Cell>
							<Table.Cell class="font-mono text-xs text-muted-foreground">
								{serviceTypeLabel(svc.service_type)}
							</Table.Cell>
							<Table.Cell class="font-mono text-xs text-muted-foreground">
								{#if isIISService(svc.service_type)}
									{iisAppKindLabel(svc.iis_app_kind || 'static')}
								{:else}
									{svc.binary_name}
								{/if}
							</Table.Cell>
							<Table.Cell class="font-mono text-xs text-muted-foreground"
								>{svc.health_check_url || '—'}</Table.Cell
							>
							<Table.Cell>
								<div class="flex items-center justify-end gap-1">
									{#if !isIISService(svc.service_type)}
										<Button.Root
											variant="ghost"
											size="icon"
											class="h-8 w-8 text-emerald-400"
											onclick={() => serviceAction(() => api.startService(svc.id))}
											title="Start"
										>
											<Play class="h-4 w-4" />
										</Button.Root>
										<Button.Root
											variant="ghost"
											size="icon"
											class="h-8 w-8 text-red-400"
											onclick={() => serviceAction(() => api.stopService(svc.id))}
											title="Stop"
										>
											<Square class="h-4 w-4" />
										</Button.Root>
										<Button.Root
											variant="ghost"
											size="icon"
											class="h-8 w-8 text-amber-400"
											onclick={() => serviceAction(() => api.restartService(svc.id))}
											title="Restart"
										>
											<RefreshCw class="h-4 w-4" />
										</Button.Root>
									{/if}
									<Button.Root
										variant="ghost"
										size="icon"
										class="h-8 w-8 text-blue-400"
										onclick={() =>
											serviceAction(() =>
												api
													.serviceHealth(svc.id)
													.then((h) => ({ message: `${svc.windows_service_name}: ${h.status}` }))
											)}
										title="Health check"
									>
										<Heart class="h-4 w-4" />
									</Button.Root>
								</div>
							</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		</Card.Root>
	{:else if !error}
		<Card.Root class="border-dashed border-border bg-card">
			<Card.Content class="flex flex-col items-center justify-center py-16 text-center">
				<Server class="mb-3 h-10 w-10 text-muted-foreground/40" />
				<h3 class="text-sm font-medium text-muted-foreground">No services registered</h3>
				<p class="mt-1 text-xs text-muted-foreground/60">Services are created under watchers</p>
			</Card.Content>
		</Card.Root>
	{/if}
</div>
