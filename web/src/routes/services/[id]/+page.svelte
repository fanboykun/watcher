<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, type Service, type Watcher, type HealthEvent, type DeployLog } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Button from '$lib/components/ui/button';
	import { ArrowLeft, Play, Square, RefreshCw, Heart, AlertCircle, CheckCircle2, XCircle, Activity, FileText, ExternalLink, TerminalSquare, Save } from '@lucide/svelte';
	import { goto } from '$app/navigation';

	let service = $state<Service | null>(null);
	let watcher = $state<Watcher | null>(null);
	let healthHistory = $state<HealthEvent[]>([]);
	let deploys = $state<DeployLog[]>([]);
	let logLines = $state<string[]>([]);
	let error = $state('');
	let actionMsg = $state('');
	let logError = $state('');
	let logType = $state<'out' | 'err'>('out');
	let logCount = $state(100);
	
	let envContent = $state('');
	let savingEnv = $state(false);
	
	let activeTab = $state(page.url.searchParams.get('tab') || 'health');

	const id = Number(page.params.id);

	onMount(async () => {
		try {
			const detail = await api.getService(id);
			service = detail.service;
			watcher = detail.watcher;
			envContent = service.env_content || '';
			healthHistory = await api.healthHistory(id, 50);
			deploys = await api.serviceDeploys(id);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load service';
		}
		loadLogs();
	});

	async function loadLogs() {
		logError = '';
		try {
			const res = await api.serviceLogs(id, logCount, logType);
			logLines = res.lines ?? [];
		} catch (e) {
			logError = e instanceof Error ? e.message : 'Failed to load logs';
			logLines = [];
		}
	}

	async function runAction(fn: () => Promise<{ message: string }>) {
		try {
			const res = await fn();
			actionMsg = res.message;
			setTimeout(() => (actionMsg = ''), 4000);
			// Refresh state if needed
			if (service) {
				const detail = await api.getService(id);
				service = detail.service;
				envContent = service.env_content || '';
			}
		} catch (e) {
			actionMsg = e instanceof Error ? e.message : 'Action failed';
			setTimeout(() => (actionMsg = ''), 5000);
		}
	}

	async function saveEnv() {
		savingEnv = true;
		try {
			const res = await api.syncServiceEnv(id, envContent);
			actionMsg = res.message;
			setTimeout(() => (actionMsg = ''), 4000);
		} catch (e) {
			actionMsg = e instanceof Error ? e.message : 'Failed to save env';
		} finally {
			savingEnv = false;
		}
	}

	async function checkHealth() {
		try {
			const h = await api.serviceHealth(id);
			actionMsg = `Health: ${h.status} (HTTP ${h.http_status})${h.error ? ' — ' + h.error : ''}`;
			healthHistory = await api.healthHistory(id, 50);
			setTimeout(() => (actionMsg = ''), 5000);
		} catch (e) {
			actionMsg = e instanceof Error ? e.message : 'Health check failed';
		}
	}

	function healthColor(s: string) {
		switch (s) {
			case 'healthy':
				return 'text-emerald-400';
			case 'unhealthy':
				return 'text-red-400';
			default:
				return 'text-amber-400';
		}
	}

	function healthBadgeColor(s: string) {
		switch (s) {
			case 'healthy':
				return 'bg-emerald-500/15 text-emerald-400 border-emerald-500/30';
			case 'unhealthy':
				return 'bg-red-500/15 text-red-400 border-red-500/30';
			default:
				return 'bg-amber-500/15 text-amber-400 border-amber-500/30';
		}
	}

	function statusColor(s: string) {
		switch (s) {
			case 'healthy':
				return 'bg-emerald-500/15 text-emerald-400 border-emerald-500/30';
			case 'deploying':
				return 'bg-blue-500/15 text-blue-400 border-blue-500/30';
			case 'failed':
				return 'bg-red-500/15 text-red-400 border-red-500/30';
			default:
				return 'bg-muted text-muted-foreground border-border';
		}
	}

	function formatDate(ts: string | null): string {
		if (!ts) return '—';
		return new Date(ts).toLocaleString();
	}

	function formatDuration(ms: number): string {
		if (!ms) return '—';
		if (ms < 1000) return `${ms}ms`;
		return `${(ms / 1000).toFixed(1)}s`;
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center gap-4">
		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
		<a href="/services">
			<Button.Root variant="ghost" size="icon" class="h-8 w-8">
				<ArrowLeft class="h-4 w-4" />
			</Button.Root>
		</a>
		<div class="flex-1">
			<h1 class="text-2xl font-bold tracking-tight">
				{service?.windows_service_name ?? 'Loading...'}
			</h1>
			{#if watcher}
				<p class="text-sm text-muted-foreground">
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
					Watcher: <a href="/watchers/{watcher.id}" class="hover:underline">{watcher.name}</a>
				</p>
			{/if}
		</div>
		{#if service}
			<div class="flex items-center gap-2">
				<Button.Root
					variant="outline"
					size="sm"
					class="text-emerald-400"
					onclick={() => runAction(() => api.startService(id))}
				>
					<Play class="mr-1.5 h-4 w-4" /> Start
				</Button.Root>
				<Button.Root
					variant="outline"
					size="sm"
					class="text-red-400"
					onclick={() => runAction(() => api.stopService(id))}
				>
					<Square class="mr-1.5 h-4 w-4" /> Stop
				</Button.Root>
				<Button.Root
					variant="outline"
					size="sm"
					class="text-amber-400"
					onclick={() => runAction(() => api.restartService(id))}
				>
					<RefreshCw class="mr-1.5 h-4 w-4" /> Restart
				</Button.Root>
				<Button.Root variant="outline" size="sm" class="text-blue-400" onclick={checkHealth}>
					<Heart class="mr-1.5 h-4 w-4" /> Health
				</Button.Root>
			</div>
		{/if}
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

	{#if service}
		<!-- Service Info Card -->
		<Card.Root class="border-border bg-card">
			<Card.Content class="grid gap-4 p-6 sm:grid-cols-4">
				<div>
					<p class="text-xs text-muted-foreground">Binary</p>
					<p class="mt-1 font-mono text-sm">{service.binary_name}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">Env File</p>
					<p class="mt-1 font-mono text-sm">{service.env_file || '—'}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">Health URL</p>
					<p class="mt-1 font-mono text-sm">{service.health_check_url || '—'}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">Install Dir</p>
					<p class="mt-1 font-mono text-sm">{watcher?.install_dir ?? '—'}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">Public URL</p>
					<p class="mt-1 font-mono text-sm">
						{#if service.public_url}
							<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
							<a href={service.public_url} target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-1.5 hover:underline text-blue-400">
								{service.public_url} <ExternalLink class="h-3 w-3" />
							</a>
						{:else}
							—
						{/if}
					</p>
				</div>
			</Card.Content>
		</Card.Root>

		{#if service.service_type === 'static'}
			<div class="rounded-lg border border-blue-500/30 bg-blue-500/5 p-4">
				<div class="flex items-center gap-2 mb-2 text-blue-400 font-medium">
					<TerminalSquare class="h-5 w-5" />
					IIS Configuration Required
				</div>
				<p class="text-sm text-foreground/80 mb-4">
					The Watcher automatically manages extracting releases and updating target junctions for static sites, but it does <strong>not</strong> create the IIS website itself. Run these commands once in an Administrator PowerShell to link IIS to this deployment:
				</p>
				<div class="bg-black/50 p-3 rounded-md overflow-x-auto border border-border">
					<pre class="font-mono text-xs text-blue-300 leading-relaxed max-w-full"><span class="text-muted-foreground"># 1. Create the application pool</span>
appcmd.exe add apppool /name:"{service.iis_app_pool}"

<span class="text-muted-foreground"># 2. Create the site (change the port/host binding as needed)</span>
appcmd.exe add site /name:"{service.iis_site_name}" /bindings:http/*:8080: /physicalPath:"{watcher?.install_dir}\current"

<span class="text-muted-foreground"># 3. Assign the site to the application pool</span>
appcmd.exe set app "{service.iis_site_name}/" /applicationPool:"{service.iis_app_pool}"</pre>
				</div>
			</div>
		{/if}

		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
		<Tabs.Root bind:value={activeTab} onValueChange={(v) => { if (v) goto(`?tab=${v}`, { replaceState: true, keepFocus: true, noScroll: true }); }}>
			<Tabs.List>
				<Tabs.Trigger value="health">Health History ({healthHistory.length})</Tabs.Trigger>
				<Tabs.Trigger value="logs">Logs</Tabs.Trigger>
				<Tabs.Trigger value="env">Environment (.env)</Tabs.Trigger>
				<Tabs.Trigger value="deploys">Deploys ({deploys.length})</Tabs.Trigger>
			</Tabs.List>

			<!-- Health History -->
			<Tabs.Content value="health" class="mt-4">
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
										<Table.Cell class="text-muted-foreground">{formatDate(h.checked_at)}</Table.Cell
										>
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
			</Tabs.Content>

			<!-- Logs -->
			<Tabs.Content value="logs" class="mt-4">
				<div class="mb-3 flex items-center gap-2">
					<select
						class="rounded-md border border-border bg-card px-3 py-1.5 text-sm text-foreground"
						bind:value={logType}
						onchange={() => loadLogs()}
					>
						<option value="out">stdout</option>
						<option value="err">stderr</option>
					</select>
					<select
						class="rounded-md border border-border bg-card px-3 py-1.5 text-sm text-foreground"
						bind:value={logCount}
						onchange={() => loadLogs()}
					>
						<option value={50}>50 lines</option>
						<option value={100}>100 lines</option>
						<option value={200}>200 lines</option>
						<option value={500}>500 lines</option>
					</select>
					<Button.Root variant="outline" size="sm" onclick={loadLogs}>
						<RefreshCw class="mr-2 h-4 w-4" /> Refresh
					</Button.Root>
				</div>

				{#if logError}
					<div
						class="mb-3 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-400"
					>
						{logError}
					</div>
				{/if}

				<Card.Root class="border-border bg-card">
					<Card.Content class="p-0">
						{#if logLines.length > 0}
							<div class="max-h-[500px] overflow-auto">
								<pre
									class="p-4 font-mono text-xs leading-relaxed text-muted-foreground">{#each logLines as line, i (i)}{line}
									{/each}</pre>
							</div>
						{:else if !logError}
							<div class="flex flex-col items-center justify-center py-12 text-center">
								<FileText class="mb-3 h-8 w-8 text-muted-foreground/40" />
								<p class="text-sm text-muted-foreground">No log output</p>
							</div>
						{/if}
					</Card.Content>
				</Card.Root>
			</Tabs.Content>
|
			<!-- Environment -->
			<Tabs.Content value="env" class="mt-4">
				<Card.Root class="border-border bg-card">
					<Card.Header class="pb-3">
						<div class="flex items-center justify-between">
							<div class="space-y-1">
								<Card.Title class="text-lg">Environment Variables</Card.Title>
								<Card.Description>Edit the <code>{service.env_file || '.env'}</code> file for this service.</Card.Description>
							</div>
							<div class="flex items-center gap-2">
								<Button.Root variant="outline" size="sm" onclick={saveEnv} disabled={savingEnv}>
									{#if savingEnv}<RefreshCw class="mr-2 h-4 w-4 animate-spin" />{:else}<Save class="mr-2 h-4 w-4" />{/if}
									Save
								</Button.Root>
								<Button.Root 
									variant="default" 
									size="sm" 
									onclick={() => { 
										saveEnv().then(() => runAction(() => api.restartService(id)));
									}} 
									disabled={savingEnv}
									class="bg-amber-600 hover:bg-amber-700 text-white"
								>
									<RefreshCw class="mr-2 h-4 w-4" /> Save & Restart
								</Button.Root>
							</div>
						</div>
					</Card.Header>
					<Card.Content>
						<textarea
							bind:value={envContent}
							class="min-h-[400px] w-full rounded-md border border-border bg-black/50 p-4 font-mono text-sm text-blue-300 focus:outline-none focus:ring-1 focus:ring-blue-500/50"
							placeholder="KEY=VALUE"
						></textarea>
						<p class="mt-2 text-xs text-muted-foreground italic">
							Note: Environment variables are written to <code>{service.env_file}</code> in the service's installation directory.
						</p>
					</Card.Content>
				</Card.Root>
			</Tabs.Content>
|
			<!-- Deploys -->
			<Tabs.Content value="deploys" class="mt-4">
				{#if deploys.length > 0}
					<Card.Root class="border-border bg-card">
						<Table.Root>
							<Table.Header>
								<Table.Row class="border-border hover:bg-transparent">
									<Table.Head>Status</Table.Head>
									<Table.Head>Version</Table.Head>
									<Table.Head>From</Table.Head>
									<Table.Head>Duration</Table.Head>
									<Table.Head>Started</Table.Head>
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
										<Table.Cell class="font-mono text-sm">{d.version}</Table.Cell>
										<Table.Cell class="font-mono text-xs text-muted-foreground"
											>{d.from_version || '—'}</Table.Cell
										>
										<Table.Cell class="text-muted-foreground"
											>{formatDuration(d.duration_ms)}</Table.Cell
										>
										<Table.Cell class="text-muted-foreground">{formatDate(d.started_at)}</Table.Cell
										>
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
			</Tabs.Content>
		</Tabs.Root>
	{/if}
</div>
