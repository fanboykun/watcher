<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, type Watcher, type DeployLog, type Service, type ServiceConfigFile } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Select } from '$lib/components/ui/select';
	import { Textarea } from '$lib/components/ui/textarea';
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
	import { filesize } from 'filesize';

	let watcher = $state<Watcher | null>(null);
	let deploys = $state<DeployLog[]>([]);
	let polls = $state<import('$lib/api').PollEvent[]>([]);
	let versions = $state<import('$lib/api').ReleaseInfo[]>([]);
	let deployPage = $state(1);
	let deployPageSize = $state(10);
	let deployTotal = $state(0);
	let pollPage = $state(1);
	let pollPageSize = $state(10);
	let pollStatus = $state('all');
	let pollTotal = $state(0);
	let error = $state('');
let triggerMsg = $state('');
let showAddService = $state(false);
let showRollbackDialog = $state(false);
let showConfirmDialog = $state(false);
let addingService = $state(false);
let confirming = $state(false);
let editing = $state(false);
let saving = $state(false);
let rollbackTargetVersion = $state('');
let rollbackReportGitHub = $state(true);
let confirmTitle = $state('');
let confirmDescription = $state('');
let confirmActionLabel = $state('Confirm');
let confirmActionClass = $state('');
let confirmAction: (() => Promise<void> | void) | null = null;

	// Add service form
	let svcType = $state<'nssm' | 'static'>('nssm');
	let svcName = $state('');
	let svcBinary = $state('');
	let svcEnvFile = $state('');
	let svcEnvContent = $state('');
	let svcConfigFiles = $state<ServiceConfigFile[]>([]);
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
	let editMaxKeptVersions = $state(3);
	let editDeploymentEnvironment = $state('');
	let editGitHubToken = $state('');
	let editUseGlobalToken = $state(false);

	let activeTab = $state(page.url.searchParams.get('tab') || 'overview');

let watcherEventSource: EventSource | null = null;
let refreshTimer: ReturnType<typeof setTimeout> | null = null;

	const id = Number(page.params.id);

	const loadPolls = async () => {
		try {
			const res = await api.watcherPolls(id, pollPage, pollPageSize, pollStatus);
			polls = res.data;
			pollTotal = res.total;
		} catch (err) {
			// ignore logs
		}
	};

	const loadDeploys = async () => {
		const res = await api.watcherDeploys(id, deployPage, deployPageSize);
		deploys = res.data;
		deployTotal = res.total;
	};

	function scheduleRefresh(includeVersions = false, includePolls = false) {
		if (refreshTimer) return;
		refreshTimer = setTimeout(async () => {
			refreshTimer = null;
			try {
				const tasks: Promise<unknown>[] = [
					api.getWatcher(id).then((w) => (watcher = w)),
					loadDeploys()
				];
				if (includeVersions) {
					tasks.push(api.watcherVersions(id).then((v) => (versions = v)).catch(() => []));
				}
				if (includePolls || activeTab === 'polling') {
					tasks.push(loadPolls());
				}
				await Promise.all(tasks);
			} catch {
				// ignore transient stream refresh errors
			}
		}, 200);
	}

	onMount(() => {
		const init = async () => {
			try {
				[watcher, deploys, versions] = await Promise.all([
					api.getWatcher(id), 
					api.watcherDeploys(id, deployPage, deployPageSize).then((res) => {
						deploys = res.data;
						deployTotal = res.total;
						return res.data;
					}),
					api.watcherVersions(id).catch(() => []) // Catch if missing dir error
				]);
				await loadPolls();
				syncEditForm();
			} catch (e) {
				error = e instanceof Error ? e.message : 'Failed to load watcher';
			}
		};
		init();

		watcherEventSource = new EventSource(`/api/watchers/${id}/events`);
		watcherEventSource.onmessage = (e) => {
			try {
				const ev = JSON.parse(e.data) as { type?: string };
				switch (ev.type) {
					case 'deploy_started':
						scheduleRefresh(false, false);
						break;
					case 'deploy_finished':
					case 'version_changed':
						scheduleRefresh(true, false);
						break;
					case 'poll_event':
						scheduleRefresh(false, true);
						break;
					case 'status_changed':
						scheduleRefresh(false, false);
						break;
					default:
						scheduleRefresh(false, false);
				}
			} catch {
				scheduleRefresh(false, false);
			}
		};
		watcherEventSource.onerror = () => {
			// Browser SSE auto-reconnect handles temporary disconnects.
		};

		return () => {
			if (watcherEventSource) {
				watcherEventSource.close();
				watcherEventSource = null;
			}
			if (refreshTimer) {
				clearTimeout(refreshTimer);
				refreshTimer = null;
			}
		};
	});

	function syncEditForm() {
		if (!watcher) return;
		editInterval = watcher.check_interval_sec;
		editMetadataURL = watcher.metadata_url;
		editInstallDir = watcher.install_dir;
		editHcEnabled = watcher.hc_enabled;
		editHcURL = watcher.hc_url;
		editMaxKeptVersions = watcher.max_kept_versions;
		editDeploymentEnvironment = watcher.deployment_environment || '';
		editGitHubToken = '';
		editUseGlobalToken = !watcher.has_github_token;
	}

	async function saveEdit() {
		saving = true;
		try {
			watcher = await api.updateWatcher(id, {
				check_interval_sec: editInterval,
				metadata_url: editMetadataURL,
				deployment_environment: editDeploymentEnvironment,
				github_token: editUseGlobalToken ? '' : (editGitHubToken.trim() !== '' ? editGitHubToken : undefined),
				install_dir: editInstallDir,
				hc_enabled: editHcEnabled,
				hc_url: editHcURL,
				max_kept_versions: editMaxKeptVersions
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
				env_content: svcEnvContent,
				config_files: svcConfigFiles.filter((file) => file.file_path.trim() !== ''),
				health_check_url: svcHealthURL,
				iis_app_pool: svcIISAppPool,
				iis_site_name: svcIISSiteName,
				public_url: svcPublicURL
			});
			showAddService = false;
			svcType = 'nssm';
			svcName =
				svcBinary =
				svcEnvFile =
				svcEnvContent =
				svcHealthURL =
				svcIISAppPool =
				svcIISSiteName =
				svcPublicURL =
					'';
			svcConfigFiles = [];
			watcher = await api.getWatcher(id);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to add service';
		} finally {
			addingService = false;
		}
	}

	function openConfirmDialog(opts: {
		title: string;
		description: string;
		actionLabel: string;
		actionClass?: string;
		action: () => Promise<void> | void;
	}) {
		confirmTitle = opts.title;
		confirmDescription = opts.description;
		confirmActionLabel = opts.actionLabel;
		confirmActionClass = opts.actionClass || '';
		confirmAction = opts.action;
		showConfirmDialog = true;
	}

	function addSvcConfigFile() {
		svcConfigFiles = [...svcConfigFiles, { file_path: '', content: '' }];
	}

	function removeSvcConfigFile(index: number) {
		svcConfigFiles = svcConfigFiles.filter((_, i) => i !== index);
	}

	async function runConfirmAction() {
		if (!confirmAction) return;
		confirming = true;
		try {
			await confirmAction();
			showConfirmDialog = false;
		} finally {
			confirming = false;
		}
	}

	function deleteService(svcId: number, name: string) {
		openConfirmDialog({
			title: 'Delete Service',
			description: `Delete service "${name}"?`,
			actionLabel: 'Delete',
			actionClass: 'bg-red-600 text-white hover:bg-red-700',
			action: async () => {
				try {
					await api.deleteService(id, svcId);
					watcher = await api.getWatcher(id);
				} catch (e) {
					error = e instanceof Error ? e.message : 'Delete failed';
				}
			}
		});
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

	function triggerRedeploy() {
		openConfirmDialog({
			title: 'Force Redeploy',
			description: `Force redeployment for "${watcher?.name}"? This will restart its services.`,
			actionLabel: 'Redeploy',
			actionClass: 'bg-amber-600 text-white hover:bg-amber-700',
			action: async () => {
				try {
					const res = await api.redeployWatcher(id);
					const fallback = resolve(`/watchers/${id}/logs/${res.deploy_log_id}`);
					if (res.log_url && /^https?:\/\//i.test(res.log_url)) {
						window.location.href = res.log_url;
						return;
					}
					await goto(fallback);
				} catch (e) {
					const msg = e instanceof Error ? e.message : 'Redeploy failed';
					const m = /deploy_log_id["'\s:]+(\d+)/i.exec(msg);
					if (m && m[1]) {
						triggerMsg = 'Deployment is already running. Opening current deployment log...';
						setTimeout(() => (triggerMsg = ''), 3500);
						await goto(resolve(`/watchers/${id}/logs/${Number(m[1])}`));
						return;
					}
					error = msg;
				}
			}
		});
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

	function openRollbackDialog(version: string) {
		rollbackTargetVersion = version;
		rollbackReportGitHub = true;
		showRollbackDialog = true;
	}

	async function rollback(version: string, reportGithub = true) {
		try {
			showRollbackDialog = false;
			triggerMsg = `Starting rollback to ${version}...`;
			const res = await api.rollbackWatcher(id, version, reportGithub);
			const fallback = resolve(`/watchers/${id}/logs/${res.deploy_log_id}`);
			if (res.log_url && /^https?:\/\//i.test(res.log_url)) {
				window.location.href = res.log_url;
				return;
			}
			await goto(fallback);
		} catch (e) {
			error = e instanceof Error ? e.message : `Rollback to ${version} failed`;
		}
	}

	function deleteVersion(version: string) {
		openConfirmDialog({
			title: 'Delete Version',
			description: `Delete version ${version} from disk? This cannot be undone.`,
			actionLabel: 'Delete',
			actionClass: 'bg-red-600 text-white hover:bg-red-700',
			action: async () => {
				try {
					await api.deleteWatcherVersion(id, version);
					versions = await api.watcherVersions(id).catch(() => []);
				} catch (e) {
					error = e instanceof Error ? e.message : `Delete ${version} failed`;
				}
			}
		});
	}

	async function resumeAutoDeploy() {
		try {
			await api.resumeWatcherUpdates(id);
			triggerMsg = `Auto-deploy resumed!`;
			watcher = await api.getWatcher(id);
		} catch (e) {
			error = e instanceof Error ? e.message : `Failed to resume auto deploy`;
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

	function compareSemver(a: string, b: string): number {
		const parse = (v: string): [number, number, number] => {
			const cleaned = (v || '').trim().replace(/^v/i, '');
			const parts = cleaned.split('.');
			const out: [number, number, number] = [0, 0, 0];
			for (let i = 0; i < 3 && i < parts.length; i++) {
				const n = Number.parseInt(parts[i], 10);
				out[i] = Number.isFinite(n) ? n : 0;
			}
			return out;
		};

		const pa = parse(a);
		const pb = parse(b);
		for (let i = 0; i < 3; i++) {
			if (pa[i] > pb[i]) return 1;
			if (pa[i] < pb[i]) return -1;
		}
		return 0;
	}

	function hasActiveRollbackPin(w: Watcher | null): boolean {
		if (!w) return false;
		const ignored = (w.max_ignored_version || '').trim();
		if (!ignored) return false;
		return compareSemver(ignored, w.current_version || '') > 0;
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
			<Button.Root
				variant="outline"
				size="sm"
				class="border-orange-500/30 text-orange-500 hover:bg-orange-500/10 hover:text-orange-600"
				onclick={triggerRedeploy}
			>
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
			{#if hasActiveRollbackPin(watcher)}
				<div class="mb-4 flex items-center justify-between rounded-lg border border-amber-500/30 bg-amber-500/10 p-4 text-sm text-amber-500">
					<div class="flex items-center gap-2">
						<AlertCircle class="h-4 w-4" />
						<span>
							<strong>Manual rollback pin is active.</strong>
							Current is <code>{watcher.current_version || 'unknown'}</code>; auto-update ignores versions
							<code>&lt;= {watcher.max_ignored_version}</code>.
						</span>
					</div>
					<Button.Root variant="outline" size="sm" class="border-amber-500/30 hover:bg-amber-500/20" onclick={resumeAutoDeploy}>
						Resume Updates
				</Button.Root>
			</div>
		{/if}

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
							<div class="space-y-2">
								<Label for="editMaxKeptVersions">Max Kept Versions</Label>
								<Input id="editMaxKeptVersions" type="number" min="1" max="10" bind:value={editMaxKeptVersions} />
							</div>
						</div>
						<div class="grid gap-4 sm:grid-cols-2">
							<div class="space-y-2">
								<Label for="editDeploymentEnvironment">Deployment Environment (GitHub)</Label>
								<Input id="editDeploymentEnvironment" bind:value={editDeploymentEnvironment} placeholder="production" />
								<p class="text-xs text-muted-foreground">Optional. Falls back to global `ENVIRONMENT` if empty.</p>
							</div>
							<div class="space-y-2">
								<Label for="editGitHubToken">GitHub Access Token Override</Label>
								<Input id="editGitHubToken" type="password" bind:value={editGitHubToken} placeholder="Paste new token to replace override" disabled={editUseGlobalToken} />
								<div class="flex items-center gap-2">
									<input type="checkbox" id="editUseGlobalToken" bind:checked={editUseGlobalToken} class="rounded border-border" />
									<Label for="editUseGlobalToken">Use global `GITHUB_TOKEN`</Label>
								</div>
								<p class="text-xs text-muted-foreground">Current: {watcher.has_github_token ? (watcher.github_token_masked || 'set') : 'using global token'}</p>
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

		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
		<Tabs.Root
			bind:value={activeTab}
			onValueChange={(v) => {
				if (v) {
					goto(`${page.url.pathname}?tab=${v}`, { replaceState: true, keepFocus: true, noScroll: true }).catch(
						() => {}
					);
				}
			}}
		>
			<Tabs.List>
				<Tabs.Trigger value="overview">Overview</Tabs.Trigger>
				<Tabs.Trigger value="services">Services ({watcher.services.length})</Tabs.Trigger>
				<Tabs.Trigger value="deploys">Deploy History ({deployTotal})</Tabs.Trigger>
				<Tabs.Trigger value="versions">Versions ({versions.length})</Tabs.Trigger>
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
							<div class="flex justify-between">
								<span class="text-muted-foreground">Deploy Environment</span><span>{watcher.deployment_environment || 'Global default'}</span>
							</div>
							<div class="flex justify-between">
								<span class="text-muted-foreground">GitHub Token</span><span>{watcher.has_github_token ? (watcher.github_token_masked || 'Configured') : 'Global default'}</span>
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
												<a href={svc.public_url}
													target="_blank"
													rel="noopener noreferrer"
													class="ml-1.5 inline-flex items-center text-muted-foreground hover:text-foreground"
													title="Open Public URL"
												>
													<ExternalLink class="h-3 w-3" />
												</a>
											{/if}
										</Table.Cell>
										<Table.Cell>
											<span
												class="inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium {svc.service_type ===
												'static'
													? 'border-blue-500/30 bg-blue-500/10 text-blue-400'
													: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-400'}"
											>
												{svc.service_type === 'static' ? 'Static (IIS)' : 'Binary (NSSM)'}
											</span>
										</Table.Cell>
										<Table.Cell class="font-mono text-xs text-muted-foreground">
											{svc.service_type === 'static' ? svc.iis_app_pool || '—' : svc.binary_name}
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
				<div class="mb-3 flex items-center justify-between gap-2">
					<div class="text-xs text-muted-foreground">Showing {(deploys.length === 0 ? 0 : ((deployPage - 1) * deployPageSize + 1))} - {Math.min(deployPage * deployPageSize, deployTotal)} of {deployTotal}</div>
					<div class="flex items-center gap-2">
						<select
							class="rounded-md border border-border bg-card px-2 py-1 text-xs text-foreground"
							bind:value={deployPageSize}
							onchange={async () => {
								deployPage = 1;
								await loadDeploys();
							}}
						>
							<option value={10}>10 / page</option>
							<option value={25}>25 / page</option>
							<option value={50}>50 / page</option>
						</select>
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
										<Table.Cell class="max-w-[200px] truncate text-xs text-red-400"
											>{d.error || ''}</Table.Cell
										>
										<Table.Cell class="text-right">
											<div class="flex items-center justify-end gap-2">
												{#if d.github_deployment_id > 0}
													<span title="Reported to GitHub" class="text-muted-foreground/60 border border-muted-foreground/20 rounded bg-muted/30 px-1 py-0.5 text-[10px] inline-flex items-center">
														<svg class="h-3 w-3 mr-1" fill="currentColor" viewBox="0 0 24 24"><path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/></svg>
														GitHub
													</span>
												{/if}
												{#if d.logs}
													<a
														href={resolve(`/watchers/${id}/logs/${d.id}`)}
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
							<Rocket class="mb-3 h-8 w-8 text-muted-foreground/40" />
							<p class="text-sm text-muted-foreground">No deployments yet</p>
						</Card.Content>
					</Card.Root>
				{/if}
			</Tabs.Content>

			<Tabs.Content value="versions" class="mt-4">
				{#if versions.length > 0}
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
											<Table.Cell class="text-muted-foreground">{v.size_bytes > 0 ? filesize(v.size_bytes) : (v.size_human || '0 B')}</Table.Cell>
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
														onclick={() => openRollbackDialog(v.version)}
													>
														<RotateCcw class="mr-1.5 h-3 w-3" />
														Rollback
													</Button.Root>
													<Button.Root
														variant="default"
														size="sm"
														class="h-8 bg-red-500/10 text-red-500 hover:bg-red-500/20"
														title="Delete Version"
														onclick={() => deleteVersion(v.version)}
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
										<Table.Cell class="max-w-[300px] truncate text-xs text-red-400" title={p.error}
											>{p.error || '—'}</Table.Cell
										>
									</Table.Row>
								{/each}
							</Table.Body>
						</Table.Root>
						<div class="mt-auto flex items-center justify-between border-t border-border px-4 py-4">
							<div class="flex items-center gap-2 text-xs text-muted-foreground">
								<span>Status Filter:</span>
								<select
									class="h-7 rounded border border-input bg-transparent px-2 text-xs focus:ring-1 focus:ring-ring"
									bind:value={pollStatus}
									onchange={() => {
										pollPage = 1;
										loadPolls();
									}}
								>
									<option value="all">All</option>
									<option value="new_release">New Release</option>
									<option value="up_to_date">Up To Date</option>
									<option value="error">Error</option>
								</select>
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
										onclick={() => {
											pollPage--;
											loadPolls();
										}}
									>
										Prev
									</Button.Root>
									<Button.Root
										variant="outline"
										size="sm"
										class="h-7 px-2"
										disabled={pollPage * pollPageSize >= pollTotal}
										onclick={() => {
											pollPage++;
											loadPolls();
										}}
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
				<Select
					id="svcType"
					bind:value={svcType}
				>
					<option value="nssm">Binary (NSSM)</option>
					<option value="static">Static Site (IIS)</option>
				</Select>
			</div>
			<div class="space-y-2">
				<Label for="svcName"
					>{svcType === 'static' ? 'Service Identifier' : 'Windows Service Name'}</Label
				>
				<Input
					id="svcName"
					placeholder={svcType === 'static' ? 'my-frontend' : 'my-app-web-1'}
					bind:value={svcName}
					required
				/>
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
				<div class="space-y-2">
					<Label for="svcEnvContent">Env Content (optional)</Label>
					<Textarea
						id="svcEnvContent"
						class="min-h-[140px] font-mono text-xs text-blue-300"
						bind:value={svcEnvContent}
						placeholder="KEY=VALUE&#10;API_URL=https://example.com"
					/>
					<p class="text-xs text-muted-foreground">
						If set, watcher writes this content into <code>{svcEnvFile || '.env'}</code> during service sync/deploy.
					</p>
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
			<div class="space-y-2">
				<div class="flex items-center justify-between">
					<Label>Additional managed config files</Label>
					<Button.Root variant="outline" size="sm" type="button" class="h-8" onclick={addSvcConfigFile}>
						<Plus class="mr-1.5 h-3 w-3" /> Add file
					</Button.Root>
				</div>
				{#if svcConfigFiles.length > 0}
					<div class="space-y-3 rounded-md border border-border/70 bg-background/50 p-3">
						{#each svcConfigFiles as file, fileIndex (fileIndex)}
							<div class="space-y-2 rounded-md border border-border/60 bg-card/60 p-3">
								<div class="flex items-center justify-between">
									<Label>Config file #{fileIndex + 1}</Label>
									<Button.Root
										variant="ghost"
										size="icon"
										type="button"
										class="h-7 w-7 text-red-400 hover:text-red-300"
										onclick={() => removeSvcConfigFile(fileIndex)}
									>
										<Trash2 class="h-3 w-3" />
									</Button.Root>
								</div>
								<Input bind:value={file.file_path} placeholder="config.json or settings/appsettings.json" />
								<Textarea
									class="min-h-[120px] font-mono text-xs text-blue-300"
									bind:value={file.content}
									placeholder={'{\n  "featureFlag": true\n}'}
								/>
							</div>
						{/each}
					</div>
				{:else}
					<p class="text-xs text-muted-foreground">
						Use this for runtime files like <code>config.json</code>, <code>appsettings.json</code>, or other generated config.
					</p>
				{/if}
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

<!-- Rollback Dialog -->
<Dialog.Root bind:open={showRollbackDialog}>
	<Dialog.Content class="sm:max-w-[480px]">
		<Dialog.Header>
			<Dialog.Title>Confirm Rollback</Dialog.Title>
			<Dialog.Description>
				This will stop running services, swap the current release, and restart services.
			</Dialog.Description>
		</Dialog.Header>
		<div class="space-y-3">
			<p class="text-sm">
				Target version: <span class="font-mono font-medium">{rollbackTargetVersion}</span>
			</p>
			<label class="inline-flex items-center gap-2 text-sm text-muted-foreground">
				<input type="checkbox" bind:checked={rollbackReportGitHub} />
				Report rollback to GitHub Deployment API
			</label>
		</div>
		<Dialog.Footer>
			<Button.Root variant="outline" type="button" onclick={() => (showRollbackDialog = false)}>
				Cancel
			</Button.Root>
			<Button.Root
				type="button"
				class="bg-amber-600 hover:bg-amber-700 text-white"
				onclick={() => rollback(rollbackTargetVersion, rollbackReportGitHub)}
			>
				Proceed Rollback
			</Button.Root>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<!-- Confirm Action Dialog -->
<Dialog.Root bind:open={showConfirmDialog}>
	<Dialog.Content class="sm:max-w-[460px]">
		<Dialog.Header>
			<Dialog.Title>{confirmTitle}</Dialog.Title>
			<Dialog.Description>{confirmDescription}</Dialog.Description>
		</Dialog.Header>
		<Dialog.Footer>
			<Button.Root variant="outline" type="button" onclick={() => (showConfirmDialog = false)} disabled={confirming}>
				Cancel
			</Button.Root>
			<Button.Root type="button" class={confirmActionClass} onclick={runConfirmAction} disabled={confirming}>
				{confirming ? 'Processing...' : confirmActionLabel}
			</Button.Root>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
