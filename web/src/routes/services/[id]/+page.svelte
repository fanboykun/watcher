<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, type Service, type Watcher, type HealthEvent, type DeployLog, type ServiceConfigFile } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Button from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Select } from '$lib/components/ui/select';
	import {
		ArrowLeft,
		Play,
		Square,
		RefreshCw,
		Heart,
		AlertCircle,
		CheckCircle2,
		XCircle,
		Activity,
		FileText,
		ExternalLink,
		TerminalSquare,
		Save
	} from '@lucide/svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';

	let service = $state<Service | null>(null);
	let watcher = $state<Watcher | null>(null);
	let healthHistory = $state<HealthEvent[]>([]);
	let deploys = $state<DeployLog[]>([]);
	let deployPage = $state(1);
	let deployPageSize = $state(10);
	let deployTotal = $state(0);
	let logLines = $state<string[]>([]);
	let error = $state('');
	let actionMsg = $state('');
	let logError = $state('');
	let logType = $state<'out' | 'err'>('out');
	let logCount = $state(100);

	let envContent = $state('');
	let configFiles = $state<ServiceConfigFile[]>([]);
	let savingEnv = $state(false);

	let activeTab = $state(page.url.searchParams.get('tab') || 'health');

	const id = Number(page.params.id);

	onMount(async () => {
		try {
			const detail = await api.getService(id);
			service = detail.service;
			watcher = detail.watcher;
			envContent = service.env_content || '';
			configFiles = [...(service.config_files || []).map((file) => ({ ...file }))];
			healthHistory = await api.healthHistory(id, 50);
			await loadDeploys();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load service';
		}
		loadLogs();
	});

	async function loadDeploys() {
		const res = await api.serviceDeploys(id, deployPage, deployPageSize);
		deploys = res.data;
		deployTotal = res.total;
	}

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
				configFiles = [...(service.config_files || []).map((file) => ({ ...file }))];
			}
		} catch (e) {
			actionMsg = e instanceof Error ? e.message : 'Action failed';
			setTimeout(() => (actionMsg = ''), 5000);
		}
	}

	async function saveEnv() {
		if (!service || !watcher) return;
		savingEnv = true;
		try {
			service = await api.updateService(watcher.id, service.id, {
				env_content: envContent,
				config_files: configFiles.filter((file) => file.file_path.trim() !== '')
			});
			envContent = service.env_content || '';
			configFiles = [...(service.config_files || []).map((file) => ({ ...file }))];
			actionMsg = 'Service files saved';
			setTimeout(() => (actionMsg = ''), 4000);
		} catch (e) {
			actionMsg = e instanceof Error ? e.message : 'Failed to save env';
		} finally {
			savingEnv = false;
		}
	}

	function addConfigFile() {
		configFiles = [...configFiles, { file_path: '', content: '' }];
	}

	function removeConfigFile(index: number) {
		configFiles = configFiles.filter((_, i) => i !== index);
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
							<a
								href={service.public_url}
								target="_blank"
								rel="noopener noreferrer"
								class="inline-flex items-center gap-1.5 text-blue-400 hover:underline"
							>
								{service.public_url}
								<ExternalLink class="h-3 w-3" />
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
				<div class="mb-2 flex items-center gap-2 font-medium text-blue-400">
					<TerminalSquare class="h-5 w-5" />
					IIS Configuration Required
				</div>
				<p class="mb-4 text-sm text-foreground/80">
					The Watcher automatically manages extracting releases and updating target junctions for
					static sites, but it does <strong>not</strong> create the IIS website itself. Run these commands
					once in an Administrator PowerShell to link IIS to this deployment:
				</p>
				<div class="overflow-x-auto rounded-md border border-border bg-black/50 p-3">
					<pre class="max-w-full font-mono text-xs leading-relaxed text-blue-300"><span
							class="text-muted-foreground"># 1. Create the application pool</span
						>
appcmd.exe add apppool /name:"{service.iis_app_pool}"

<span class="text-muted-foreground"
							># 2. Create the site (change the port/host binding as needed)</span
						>
appcmd.exe add site /name:"{service.iis_site_name}" /bindings:http/*:8080: /physicalPath:"{watcher?.install_dir}\current"

<span class="text-muted-foreground"># 3. Assign the site to the application pool</span>
appcmd.exe set app "{service.iis_site_name}/" /applicationPool:"{service.iis_app_pool}"</pre>
				</div>
			</div>
		{/if}

		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
		<Tabs.Root
			bind:value={activeTab}
			onValueChange={(v) => {
				if (v) goto(`?tab=${v}`, { replaceState: true, keepFocus: true, noScroll: true });
			}}
		>
			<Tabs.List>
				<Tabs.Trigger value="health">Health History ({healthHistory.length})</Tabs.Trigger>
				<Tabs.Trigger value="logs">Logs</Tabs.Trigger>
				<Tabs.Trigger value="env">Environment (.env)</Tabs.Trigger>
				<Tabs.Trigger value="deploys">Deploys ({deployTotal})</Tabs.Trigger>
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
					<Select
						class="w-auto min-w-[120px] text-sm"
						bind:value={logType}
						onchange={() => loadLogs()}
					>
						<option value="out">stdout</option>
						<option value="err">stderr</option>
					</Select>
					<Select
						class="w-auto min-w-[120px] text-sm"
						bind:value={logCount}
						onchange={() => loadLogs()}
					>
						<option value={50}>50 lines</option>
						<option value={100}>100 lines</option>
						<option value={200}>200 lines</option>
						<option value={500}>500 lines</option>
					</Select>
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

			<!-- Environment -->
			<Tabs.Content value="env" class="mt-4">
				<Card.Root class="border-border bg-card">
					<Card.Header class="pb-3">
						<div class="flex items-center justify-between">
							<div class="space-y-1">
								<Card.Title class="text-lg">Service Files</Card.Title>
								<Card.Description
									>Manage <code>{service.env_file || '.env'}</code> and any additional runtime config files for this service.</Card.Description
								>
							</div>
							<div class="flex items-center gap-2">
								<Button.Root variant="outline" size="sm" onclick={saveEnv} disabled={savingEnv}>
									{#if savingEnv}<RefreshCw class="mr-2 h-4 w-4 animate-spin" />{:else}<Save
											class="mr-2 h-4 w-4"
										/>{/if}
									Save
								</Button.Root>
								<Button.Root
									variant="default"
									size="sm"
									onclick={() => {
										saveEnv().then(() => runAction(() => api.restartService(id)));
									}}
									disabled={savingEnv}
									class="bg-amber-600 text-white hover:bg-amber-700"
								>
									<RefreshCw class="mr-2 h-4 w-4" /> Save & Restart
								</Button.Root>
							</div>
						</div>
					</Card.Header>
					<Card.Content class="space-y-4">
						<div class="space-y-2">
							<p class="text-sm text-muted-foreground">Primary env file</p>
							<Input value={service.env_file || '.env'} disabled />
						</div>
						<Textarea
							bind:value={envContent}
							class="min-h-[280px] font-mono text-sm text-blue-300"
							placeholder="KEY=VALUE"
						/>
						<p class="mt-2 text-xs text-muted-foreground italic">
							Note: Environment variables are written to <code>{service.env_file}</code> in the service's
							installation directory.
						</p>
						<div class="space-y-3 border-t border-border pt-4">
							<div class="flex items-center justify-between">
								<div>
									<h3 class="text-sm font-medium">Additional managed config files</h3>
									<p class="text-xs text-muted-foreground">Use this for files like <code>config.json</code>, <code>appsettings.json</code>, or any other dynamic config.</p>
								</div>
								<Button.Root variant="outline" size="sm" onclick={addConfigFile}>
									Add file
								</Button.Root>
							</div>
							{#if configFiles.length > 0}
								<div class="space-y-3">
									{#each configFiles as file, index (index)}
										<Card.Root class="border-border/70 bg-background/60">
											<Card.Content class="space-y-3 p-4">
												<div class="flex items-center justify-between">
													<p class="text-sm font-medium">Config file #{index + 1}</p>
													<Button.Root
														variant="ghost"
														size="icon"
														class="h-8 w-8 text-red-400 hover:text-red-300"
														onclick={() => removeConfigFile(index)}
													>
														<XCircle class="h-4 w-4" />
													</Button.Root>
												</div>
												<Input bind:value={file.file_path} placeholder="config.json or settings/appsettings.json" />
												<Textarea
													class="min-h-[180px] font-mono text-sm text-blue-300"
													bind:value={file.content}
													placeholder={'{\n  "featureFlag": true\n}'}
												/>
											</Card.Content>
										</Card.Root>
									{/each}
								</div>
							{:else}
								<div class="rounded-md border border-dashed border-border bg-muted/20 p-4 text-sm text-muted-foreground">
									No extra config files yet.
								</div>
							{/if}
						</div>
					</Card.Content>
				</Card.Root>
			</Tabs.Content>

			<!-- Deploys -->
			<Tabs.Content value="deploys" class="mt-4">
				<div class="mb-3 flex items-center justify-between gap-2">
					<div class="text-xs text-muted-foreground">Showing {(deploys.length === 0 ? 0 : ((deployPage - 1) * deployPageSize + 1))} - {Math.min(deployPage * deployPageSize, deployTotal)} of {deployTotal}</div>
					<div class="flex items-center gap-2">
						<Select
							class="w-auto min-w-[110px] text-xs"
							bind:value={deployPageSize}
							onchange={async () => {
								deployPage = 1;
								await loadDeploys();
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
								await loadDeploys();
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
								await loadDeploys();
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
										<Table.Cell class="text-xs capitalize text-muted-foreground">{d.triggered_by || 'agent'}</Table.Cell>
										<Table.Cell class="font-mono text-sm">{d.version}</Table.Cell>
										<Table.Cell class="font-mono text-xs text-muted-foreground"
											>{d.from_version || '—'}</Table.Cell
										>
										<Table.Cell class="text-muted-foreground"
											>{formatDuration(d.duration_ms)}</Table.Cell
										>
										<Table.Cell class="text-muted-foreground">{formatDate(d.started_at)}</Table.Cell
										>
										<Table.Cell class="max-w-[250px] truncate text-xs text-red-400">{d.error || ''}</Table.Cell>
										<Table.Cell class="text-right">
											<div class="flex items-center justify-end gap-2">
												{#if d.github_deployment_id > 0}
													<span title="Reported to GitHub" class="inline-flex items-center rounded border border-muted-foreground/20 bg-muted/30 px-1 py-0.5 text-[10px] text-muted-foreground/70">
														<svg class="mr-1 h-3 w-3" fill="currentColor" viewBox="0 0 24 24"><path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/></svg>
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
			</Tabs.Content>
		</Tabs.Root>
	{/if}
</div>
