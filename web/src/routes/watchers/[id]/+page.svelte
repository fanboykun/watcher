<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, type Watcher, type DeployLog, type Service } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import {
		ArrowLeft,
		Clock,
		Rocket,
		Server,
		Zap,
		AlertCircle,
		CheckCircle2,
		XCircle,
		Loader2,
		RotateCcw,
		Plus,
		Pencil,
		Trash2,
		Save,
		X,
		ExternalLink,
		Pause,
		Play,
		RefreshCw
	} from '@lucide/svelte';
	import { resolve } from '$app/paths';
	import { goto } from '$app/navigation';
	import { timeAgo } from '$lib/utils';

	let watcher = $state<Watcher | null>(null);
	let deploys = $state<DeployLog[]>([]);
	let polls = $state<import('$lib/api').PollEvent[]>([]);
	let error = $state('');
	let triggerMsg = $state('');
	let showAddService = $state(false);
	let addingService = $state(false);
	let editing = $state(false);
	let saving = $state(false);

	// Add service form
	let svcType = $state<'nssm' | 'static'>('nssm');
	let svcName = $state('');
	let svcBinary = $state('');
	let svcEnvFile = $state('');
	let svcHealthURL = $state('');
	let svcIISAppPool = $state('');
	let svcIISSiteName = $state('');
	let svcPublicURL = $state('');

	// Edit form
	let editInterval = $state(60);
	let editMetadataURL = $state('');
	let editInstallDir = $state('');
	let editHcEnabled = $state(false);
	let editHcURL = $state('');

	let activeTab = $state(page.url.searchParams.get('tab') || 'overview');

	let liveLogLines = $state<string[]>([]);
	let liveLogSource: EventSource | null = null;
	let logContainer: HTMLElement | undefined = $state();

	const id = Number(page.params.id);

	onMount(() => {
		const init = async () => {
			try {
				[watcher, deploys, polls] = await Promise.all([
					api.getWatcher(id),
					api.watcherDeploys(id),
					api.watcherPolls(id)
				]);
				syncEditForm();
			} catch (e) {
				error = e instanceof Error ? e.message : 'Failed to load watcher';
			}
		};
		init();

		const poll = setInterval(async () => {
			if (!watcher) return;
			try {
				watcher = await api.getWatcher(id);
				if (activeTab === 'polling') {
					polls = await api.watcherPolls(id);
				}
			} catch (err) {
				// ignore polling errors
			}
		}, 5000);

		return () => clearInterval(poll);
	});

	$effect(() => {
		if (watcher?.status === 'deploying') {
			if (!liveLogSource) {
				liveLogLines = [];
				liveLogSource = new EventSource('/api/logs/stream?type=out');
				liveLogSource.onmessage = (e) => {
					liveLogLines = [...liveLogLines, e.data];
				};
				liveLogSource.onerror = () => {
					// Fallback or retry logic (browser auto-retries SSE)
				};
			}
		} else {
			if (liveLogSource) {
				liveLogSource.close();
				liveLogSource = null;
			}
		}

		return () => {
			if (liveLogSource) {
				liveLogSource.close();
				liveLogSource = null;
			}
		};
	});

	$effect(() => {
		// Auto-scroll logic
		if (logContainer && liveLogLines.length > 0) {
			logContainer.scrollTop = logContainer.scrollHeight;
		}
	});

	function syncEditForm() {
		if (!watcher) return;
		editInterval = watcher.check_interval_sec;
		editMetadataURL = watcher.metadata_url;
		editInstallDir = watcher.install_dir;
		editHcEnabled = watcher.hc_enabled;
		editHcURL = watcher.hc_url;
	}

	async function saveEdit() {
		saving = true;
		try {
			watcher = await api.updateWatcher(id, {
				check_interval_sec: editInterval,
				metadata_url: editMetadataURL,
				install_dir: editInstallDir,
				hc_enabled: editHcEnabled,
				hc_url: editHcURL
			});
			editing = false;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Save failed';
		} finally {
			saving = false;
		}
	}

	async function addService() {
		addingService = true;
		try {
			await api.createService(id, {
				service_type: svcType,
				windows_service_name: svcName,
				binary_name: svcBinary,
				env_file: svcEnvFile,
				health_check_url: svcHealthURL,
				iis_app_pool: svcIISAppPool,
				iis_site_name: svcIISSiteName,
				public_url: svcPublicURL
			});
			showAddService = false;
			svcType = 'nssm';
			svcName = svcBinary = svcEnvFile = svcHealthURL = svcIISAppPool = svcIISSiteName = svcPublicURL = '';
			watcher = await api.getWatcher(id);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to add service';
		} finally {
			addingService = false;
		}
	}

	async function deleteService(svcId: number, name: string) {
		if (!confirm(`Delete service "${name}"?`)) return;
		try {
			await api.deleteService(id, svcId);
			watcher = await api.getWatcher(id);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Delete failed';
		}
	}

	async function triggerCheck() {
		try {
			const res = await api.triggerCheck(id);
			triggerMsg = res.message;
			setTimeout(() => (triggerMsg = ''), 3000);
		} catch (e) {
			triggerMsg = e instanceof Error ? e.message : 'Trigger failed';
		}
	}

	async function triggerRedeploy() {
		if (!confirm(`Are you sure you want to force a redeployment for "${watcher?.name}"? This will restart its services.`)) return;
		try {
			const res = await api.redeployWatcher(id);
			triggerMsg = res.message;
			setTimeout(() => (triggerMsg = ''), 3000);
		} catch (e) {
			triggerMsg = e instanceof Error ? e.message : 'Redeploy failed';
		}
	}

	async function togglePause() {
		if (!watcher) return;
		const newPaused = !watcher.paused;
		try {
			watcher = await api.updateWatcher(id, { paused: newPaused });
		} catch (e) {
			error = e instanceof Error ? e.message : 'Toggle pause failed';
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
			case 'rollback':
				return 'bg-amber-500/15 text-amber-400 border-amber-500/30';
			default:
				return 'bg-muted text-muted-foreground border-border';
		}
	}

	function deployIcon(s: string) {
		switch (s) {
			case 'healthy':
				return CheckCircle2;
			case 'failed':
				return XCircle;
			case 'deploying':
				return Loader2;
			case 'rollback':
				return RotateCcw;
			default:
				return Clock;
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
		<a href={resolve('/watchers')}>
			<Button.Root variant="ghost" size="icon" class="h-8 w-8">
				<ArrowLeft class="h-4 w-4" />
			</Button.Root>
		</a>
		<div class="flex-1">
			<h1 class="text-2xl font-bold tracking-tight">{watcher?.name ?? 'Loading...'}</h1>
			{#if watcher}
				<p class="font-mono text-sm text-muted-foreground">{watcher.service_name}</p>
			{/if}
		</div>
		{#if watcher}
			<span
				class="inline-flex items-center rounded-full border px-3 py-1 text-sm font-medium capitalize {statusColor(
					watcher.status
				)}"
			>
				{watcher.status}
			</span>
			<Button.Root
				variant="outline"
				size="sm"
				onclick={() => {
					editing = !editing;
					if (editing) syncEditForm();
				}}
			>
				{#if editing}<X class="mr-2 h-4 w-4" /> Cancel{:else}<Pencil class="mr-2 h-4 w-4" /> Edit{/if}
			</Button.Root>

			{#if watcher.paused}
				<Button.Root variant="outline" size="sm" onclick={togglePause}>
					<Play class="mr-2 h-4 w-4" /> Resume
				</Button.Root>
			{:else}
				<Button.Root variant="outline" size="sm" onclick={togglePause}>
					<Pause class="mr-2 h-4 w-4" /> Pause
				</Button.Root>
			{/if}

			<Button.Root variant="outline" size="sm" onclick={triggerCheck} disabled={watcher.paused}>
				<RefreshCw class="mr-2 h-4 w-4" /> Poll Now
			</Button.Root>
			<Button.Root variant="outline" size="sm" class="text-orange-500 hover:bg-orange-500/10 hover:text-orange-600 border-orange-500/30" onclick={triggerRedeploy}>
				<RotateCcw class="mr-2 h-4 w-4" /> Redeploy
			</Button.Root>
		{/if}
	</div>

	{#if error}
		<div class="rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400">
			<AlertCircle class="mr-2 inline h-4 w-4" />
			{error}
		</div>
	{/if}

	{#if triggerMsg}
		<div class="rounded-lg border border-blue-500/30 bg-blue-500/10 p-4 text-sm text-blue-400">
			<Zap class="mr-2 inline h-4 w-4" />
			{triggerMsg}
		</div>
	{/if}

	{#if watcher}
		<!-- Inline Edit Form -->
		{#if editing}
			<Card.Root class="border-blue-500/30 bg-card">
				<Card.Header class="pb-3">
					<Card.Title class="text-sm font-medium">Edit Watcher Configuration</Card.Title>
				</Card.Header>
				<Card.Content>
					<form
						class="space-y-4"
						onsubmit={(e) => {
							e.preventDefault();
							saveEdit();
						}}
					>
						<div class="space-y-2">
							<Label for="editMetadataURL">Metadata URL</Label>
							<Input id="editMetadataURL" bind:value={editMetadataURL} />
						</div>
						<div class="grid gap-4 sm:grid-cols-3">
							<div class="space-y-2">
								<Label for="editInstallDir">Install Directory</Label>
								<Input id="editInstallDir" bind:value={editInstallDir} />
							</div>
							<div class="space-y-2">
								<Label for="editInterval">Check Interval (s)</Label>
								<Input id="editInterval" type="number" min="10" bind:value={editInterval} />
							</div>
							<div class="space-y-2">
								<Label for="editHcURL">Health Check URL</Label>
								<Input id="editHcURL" bind:value={editHcURL} />
							</div>
						</div>
						<div class="flex items-center gap-2">
							<input
								type="checkbox"
								id="editHcEnabled"
								bind:checked={editHcEnabled}
								class="rounded border-border"
							/>
							<Label for="editHcEnabled">Enable health checks</Label>
						</div>
						<div class="flex justify-end">
							<Button.Root type="submit" disabled={saving}>
								<Save class="mr-2 h-4 w-4" />
								{saving ? 'Saving...' : 'Save Changes'}
							</Button.Root>
						</div>
					</form>
				</Card.Content>
			</Card.Root>
		{/if}

		{#if watcher.status === 'deploying'}
			<Card.Root class="border-blue-500/30 bg-blue-500/5 mb-6">
				<Card.Header class="pb-3 border-b border-border/50">
					<Card.Title class="text-sm font-medium flex items-center gap-2 text-blue-400">
						<Loader2 class="h-4 w-4 animate-spin" />
						Deployment in Progress...
					</Card.Title>
				</Card.Header>
				<Card.Content class="p-0">
					<div class="h-[300px] w-full bg-[#0a0a0a] overflow-y-auto p-4 font-mono text-xs text-blue-300 leading-relaxed scroll-smooth" bind:this={logContainer}>
						{#if liveLogLines.length === 0}
							<div class="text-muted-foreground">Connecting to agent stream...</div>
						{/if}
						{#each liveLogLines as line, i (i)}
							<div class="whitespace-pre-wrap break-all">{line}</div>
						{/each}
					</div>
				</Card.Content>
			</Card.Root>
		{/if}

		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
		<Tabs.Root bind:value={activeTab} onValueChange={(v) => { if (v) goto(`?tab=${v}`, { replaceState: true, keepFocus: true, noScroll: true }); }}>
			<Tabs.List>
				<Tabs.Trigger value="overview">Overview</Tabs.Trigger>
				<Tabs.Trigger value="services">Services ({watcher.services.length})</Tabs.Trigger>
				<Tabs.Trigger value="deploys">Deploy History ({deploys.length})</Tabs.Trigger>
				<Tabs.Trigger value="polling">Polling History</Tabs.Trigger>
			</Tabs.List>

			<Tabs.Content value="overview" class="mt-4">
				<div class="grid gap-4 sm:grid-cols-2">
					<Card.Root class="border-border bg-card">
						<Card.Header class="pb-3">
							<Card.Title class="text-sm font-medium text-muted-foreground"
								>Configuration</Card.Title
							>
						</Card.Header>
						<Card.Content class="space-y-2 text-sm">
							<div class="flex justify-between">
								<span class="text-muted-foreground">Metadata URL</span><span
									class="max-w-[220px] truncate font-mono text-xs">{watcher.metadata_url}</span
								>
							</div>
							<div class="flex justify-between">
								<span class="text-muted-foreground">Check Interval</span><span
									>{watcher.check_interval_sec}s</span
								>
							</div>
							<div class="flex justify-between">
								<span class="text-muted-foreground">Install Dir</span><span
									class="font-mono text-xs">{watcher.install_dir}</span
								>
							</div>
							<div class="flex justify-between">
								<span class="text-muted-foreground">Download Retries</span><span
									>{watcher.download_retries}</span
								>
							</div>
							<div class="flex justify-between">
								<span class="text-muted-foreground">Health Check</span><span
									>{watcher.hc_enabled ? 'Enabled' : 'Disabled'}</span
								>
							</div>
						</Card.Content>
					</Card.Root>

					<Card.Root class="border-border bg-card">
						<Card.Header class="pb-3">
							<Card.Title class="text-sm font-medium text-muted-foreground">Deploy State</Card.Title
							>
						</Card.Header>
						<Card.Content class="space-y-2 text-sm">
							<div class="flex justify-between">
								<span class="text-muted-foreground">Current Version</span><span class="font-mono"
									>{watcher.current_version || '—'}</span
								>
							</div>
							<div class="flex justify-between">
								<span class="text-muted-foreground">Last Checked</span><span
									>{watcher.last_checked ? timeAgo(watcher.last_checked) : 'Never'}</span
								>
							</div>
							<div class="flex justify-between">
								<span class="text-muted-foreground">Last Deployed</span><span
									>{watcher.last_deployed ? timeAgo(watcher.last_deployed) : 'Never'}</span
								>
							</div>
							{#if watcher.last_error}
								<div
									class="mt-2 rounded border border-red-500/30 bg-red-500/10 p-2 text-xs text-red-400"
								>
									{watcher.last_error}
								</div>
							{/if}
						</Card.Content>
					</Card.Root>
				</div>
			</Tabs.Content>

			<Tabs.Content value="services" class="mt-4">
				<div class="mb-4 flex justify-end">
					<Button.Root size="sm" onclick={() => (showAddService = true)}>
						<Plus class="mr-2 h-4 w-4" /> Add Service
					</Button.Root>
				</div>
				{#if watcher.services.length > 0}
					<Card.Root class="border-border bg-card">
						<Table.Root>
							<Table.Header>
								<Table.Row class="border-border hover:bg-transparent">
									<Table.Head>Service Name</Table.Head>
									<Table.Head>Type</Table.Head>
									<Table.Head>Binary / App Pool</Table.Head>
									<Table.Head>Health URL</Table.Head>
									<Table.Head class="text-right">Actions</Table.Head>
								</Table.Row>
							</Table.Header>
							<Table.Body>
								{#each watcher.services as svc (svc.id)}
									<Table.Row class="border-border">
										<Table.Cell>
											<a href={resolve(`/services/${svc.id}`)} class="font-medium hover:underline"
												>{svc.windows_service_name}</a
											>
											{#if svc.public_url}
												<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
												<a href={svc.public_url} target="_blank" rel="noopener noreferrer" class="ml-1.5 inline-flex items-center text-muted-foreground hover:text-foreground" title="Open Public URL">
													<ExternalLink class="h-3 w-3" />
												</a>
											{/if}
										</Table.Cell>
										<Table.Cell>
											<span class="inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium {svc.service_type === 'static' ? 'border-blue-500/30 bg-blue-500/10 text-blue-400' : 'border-emerald-500/30 bg-emerald-500/10 text-emerald-400'}">
												{svc.service_type === 'static' ? 'Static (IIS)' : 'Binary (NSSM)'}
											</span>
										</Table.Cell>
										<Table.Cell class="font-mono text-xs text-muted-foreground">
											{svc.service_type === 'static' ? (svc.iis_app_pool || '—') : svc.binary_name}
										</Table.Cell>
										<Table.Cell class="font-mono text-xs text-muted-foreground"
											>{svc.health_check_url || '—'}</Table.Cell
										>
										<Table.Cell class="text-right">
											<Button.Root
												variant="ghost"
												size="icon"
												class="h-8 w-8 text-red-400 hover:text-red-300"
												onclick={() => deleteService(svc.id, svc.windows_service_name)}
												title="Delete"
											>
												<Trash2 class="h-4 w-4" />
											</Button.Root>
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
							<p class="text-sm text-muted-foreground">No services configured</p>
							<p class="mt-1 text-xs text-muted-foreground/60">
								Click "Add Service" to link a Windows service
							</p>
						</Card.Content>
					</Card.Root>
				{/if}
			</Tabs.Content>

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
									<Table.Head>Error</Table.Head>
								</Table.Row>
							</Table.Header>
							<Table.Body>
								{#each deploys as d (d.id)}
									{@const Icon = deployIcon(d.status)}
									<Table.Row class="border-border">
										<Table.Cell>
											<span
												class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-medium capitalize {statusColor(
													d.status
												)}"
											>
												<Icon class="h-3 w-3" />
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
										<Table.Cell class="max-w-[200px] truncate text-xs text-red-400"
											>{d.error || ''}</Table.Cell
										>
									</Table.Row>
								{/each}
							</Table.Body>
						</Table.Root>
					</Card.Root>
				{:else}
					<Card.Root class="border-dashed border-border bg-card">
						<Card.Content class="flex flex-col items-center justify-center py-12 text-center">
							<Rocket class="mb-3 h-8 w-8 text-muted-foreground/40" />
							<p class="text-sm text-muted-foreground">No deployments yet</p>
						</Card.Content>
					</Card.Root>
				{/if}
			</Tabs.Content>

			<Tabs.Content value="polling" class="mt-4">
				{#if polls.length > 0}
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
												{p.status === 'new_release' ? 'bg-blue-500/15 text-blue-400 border-blue-500/30' : 
												 p.status === 'error' ? 'bg-red-500/15 text-red-400 border-red-500/30' : 
												 'bg-muted text-muted-foreground border-border'}"
											>
												{p.status.replace('_', ' ')}
											</span>
										</Table.Cell>
										<Table.Cell class="font-mono text-sm">{p.remote_version || '—'}</Table.Cell>
										<Table.Cell class="max-w-[300px] truncate text-xs text-red-400" title={p.error}
											>{p.error || '—'}</Table.Cell
										>
									</Table.Row>
								{/each}
							</Table.Body>
						</Table.Root>
					</Card.Root>
				{:else}
					<Card.Root class="border-dashed border-border bg-card">
						<Card.Content class="flex flex-col items-center justify-center py-12 text-center">
							<Clock class="mb-3 h-8 w-8 text-muted-foreground/40" />
							<p class="text-sm text-muted-foreground">No polling history yet</p>
						</Card.Content>
					</Card.Root>
				{/if}
			</Tabs.Content>
		</Tabs.Root>
	{/if}
</div>

<!-- Add Service Dialog -->
<Dialog.Root bind:open={showAddService}>
	<Dialog.Content class="sm:max-w-[450px]">
		<Dialog.Header>
			<Dialog.Title>Add Service</Dialog.Title>
			<Dialog.Description>Register a service for this watcher to manage</Dialog.Description>
		</Dialog.Header>
		<form
			class="space-y-4"
			onsubmit={(e) => {
				e.preventDefault();
				addService();
			}}
		>
			<div class="space-y-2">
				<Label for="svcType">Service Type</Label>
				<select
					id="svcType"
					class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
					bind:value={svcType}
				>
					<option value="nssm">Binary (NSSM)</option>
					<option value="static">Static Site (IIS)</option>
				</select>
			</div>
			<div class="space-y-2">
				<Label for="svcName">{svcType === 'static' ? 'Service Identifier' : 'Windows Service Name'}</Label>
				<Input id="svcName" placeholder={svcType === 'static' ? 'my-frontend' : 'my-app-web-1'} bind:value={svcName} required />
			</div>

			{#if svcType === 'nssm'}
				<div class="space-y-2">
					<Label for="svcBinary">Binary Name</Label>
					<Input id="svcBinary" placeholder="my-app.exe" bind:value={svcBinary} required />
				</div>
				<div class="space-y-2">
					<Label for="svcEnvFile">Env File (optional)</Label>
					<Input id="svcEnvFile" placeholder="C:\apps\my-app\.env.web.1" bind:value={svcEnvFile} />
				</div>
			{:else}
				<div class="space-y-2">
					<Label for="svcIISAppPool">IIS App Pool Name</Label>
					<Input id="svcIISAppPool" placeholder="my-frontend" bind:value={svcIISAppPool} />
				</div>
				<div class="space-y-2">
					<Label for="svcIISSiteName">IIS Site Name</Label>
					<Input id="svcIISSiteName" placeholder="my-frontend" bind:value={svcIISSiteName} />
				</div>
			{/if}

			<div class="space-y-2">
				<Label for="svcHealthURL">Health Check URL (optional)</Label>
				<Input
					id="svcHealthURL"
					placeholder="http://localhost:3000/health"
					bind:value={svcHealthURL}
				/>
			</div>
			<div class="space-y-2">
				<Label for="svcPublicURL">Public URL (optional)</Label>
				<Input
					id="svcPublicURL"
					placeholder="https://my-app.example.com"
					bind:value={svcPublicURL}
				/>
			</div>
			<Dialog.Footer>
				<Button.Root variant="outline" type="button" onclick={() => (showAddService = false)}
					>Cancel</Button.Root
				>
				<Button.Root type="submit" disabled={addingService}>
					{addingService ? 'Adding...' : 'Add Service'}
				</Button.Root>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>

