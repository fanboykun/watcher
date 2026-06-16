<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import {
		api,
		iisAppKindLabel,
		isIISService,
		serviceTypeLabel,
		type AuthenticatedEventStream,
		type DeployLog,
		type IISAppKind,
		type Service,
		type WebhookDelivery,
		type Watcher
	} from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import {
		ArrowLeft,
		Zap,
		AlertCircle,
		Play,
		Pause,
		RefreshCw,
		RotateCcw,
		Pencil,
		X,
		Save
	} from '@lucide/svelte';
	import { resolve } from '$app/paths';
	import { goto } from '$app/navigation';
	import { timeAgo, formatDate, formatDuration, statusColor, compareSemver } from '$lib/utils';

	// Sub-components
	import OverviewTab from './components/overview-tab.svelte';
	import ServicesTab from './components/services-tab.svelte';
	import DeploysTab from './components/deploys-tab.svelte';
	import VersionsTab from './components/versions-tab.svelte';
	import PollingTab from './components/polling-tab.svelte';
	import WebhooksTab from './components/webhooks-tab.svelte';
	import AddServiceDialog from './components/add-service-dialog.svelte';
	import EditServiceDialog from './components/edit-service-dialog.svelte';

	let watcher = $state<Watcher | null>(null);
	let deploys = $state<DeployLog[]>([]);
	let polls = $state<import('$lib/api').PollEvent[]>([]);
	let versions = $state<import('$lib/api').ReleaseInfo[]>([]);
	let webhookDeliveries = $state<WebhookDelivery[]>([]);
	let deployPage = $state(1);
	let deployPageSize = $state(10);
	let deployTotal = $state(0);
	let deliveryPage = $state(1);
	let deliveryPageSize = $state(20);
	let deliveryTotal = $state(0);
	let pollPage = $state(1);
	let pollPageSize = $state(10);
	let pollStatus = $state('all');
	let pollTotal = $state(0);
	let error = $state('');
	let triggerMsg = $state('');

	let showAddService = $state(false);
	let showEditService = $state(false);
	let showRollbackDialog = $state(false);
	let showConfirmDialog = $state(false);
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

	let editSvc = $state<Service | null>(null);

	// Edit form
	let editInterval = $state(60);
	let editMetadataURL = $state('');
	let editInstallDir = $state('');
	let editReleaseRef = $state('latest');
	let editHcEnabled = $state(false);
	let editHcURL = $state('');
	let editMaxKeptVersions = $state(3);
	let editDeploymentEnvironment = $state('');
	let editGitHubToken = $state('');
	let editUseGlobalToken = $state(false);
	let editWebhookEnabled = $state(false);
	let editWebhookURL = $state('');
	let editWebhookBearerToken = $state('');
	let editUseGlobalWebhookToken = $state(false);
	let editNotifyVersionFound = $state(false);
	let editNotifyDeploymentSucceeded = $state(false);
	let editNotifyDeploymentFailed = $state(false);
	let editNotifyRollbackSucceeded = $state(false);
	let editNotifyRollbackFailed = $state(false);
	let editNotifyServiceHealthChanged = $state(false);

	let activeTab = $state(page.url.searchParams.get('tab') || 'overview');

	let watcherEventSource: AuthenticatedEventStream | null = null;
	let refreshTimer: ReturnType<typeof setTimeout> | null = null;

	const iisAppKinds: Array<{ value: IISAppKind; label: string; hint: string }> = [
		{ value: 'static', label: 'Static Site', hint: 'Frontend build or static files served directly by IIS.' },
		{ value: 'php', label: 'PHP', hint: 'PHP app on IIS with FastCGI and PHP already installed.' },
		{ value: 'aspnet_classic', label: 'ASP.NET Classic', hint: 'Classic ASP.NET app using the managed CLR app pool.' }
	];

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

	const loadWebhookDeliveries = async () => {
		const res = await api.watcherWebhookDeliveries(id, deliveryPage, deliveryPageSize);
		webhookDeliveries = res.data;
		deliveryTotal = res.total;
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
				if (activeTab === 'webhooks') {
					tasks.push(loadWebhookDeliveries());
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
				watcher = await api.getWatcher(id);
				syncEditForm();

				void Promise.allSettled([
					api.watcherDeploys(id, deployPage, deployPageSize).then((res) => {
						deploys = res.data;
						deployTotal = res.total;
					}),
					api.watcherVersions(id).then((v) => {
						versions = v;
					}),
					loadPolls(),
					loadWebhookDeliveries()
				]).then((results) => {
					if (results[0]?.status === 'rejected') {
						deploys = [];
						deployTotal = 0;
					}
					if (results[1]?.status === 'rejected') {
						versions = [];
					}
				});
			} catch (e) {
				error = e instanceof Error ? e.message : 'Failed to load watcher';
			}
		};
		init();

		watcherEventSource = api.streamWatcherEvents(
			id,
			(data) => {
				try {
					const ev = JSON.parse(data) as { type?: string };
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
			},
			() => {
				// The fetch stream helper reconnects watcher events after temporary disconnects.
			}
		);

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
		editReleaseRef = watcher.release_ref || 'latest';
		editHcEnabled = watcher.hc_enabled;
		editHcURL = watcher.hc_url;
		editMaxKeptVersions = watcher.max_kept_versions;
		editDeploymentEnvironment = watcher.deployment_environment || '';
		editGitHubToken = '';
		editUseGlobalToken = !watcher.has_github_token;
		editWebhookEnabled = watcher.webhook_enabled;
		editWebhookURL = watcher.webhook_url || '';
		editWebhookBearerToken = '';
		editUseGlobalWebhookToken = !watcher.has_webhook_bearer_token;
		editNotifyVersionFound = watcher.notify_version_found;
		editNotifyDeploymentSucceeded = watcher.notify_deployment_succeeded;
		editNotifyDeploymentFailed = watcher.notify_deployment_failed;
		editNotifyRollbackSucceeded = watcher.notify_rollback_succeeded;
		editNotifyRollbackFailed = watcher.notify_rollback_failed;
		editNotifyServiceHealthChanged = watcher.notify_service_health_changed;
	}

	async function saveEdit() {
		saving = true;
		try {
			watcher = await api.updateWatcher(id, {
				check_interval_sec: editInterval,
				metadata_url: editMetadataURL,
				release_ref: editReleaseRef.trim() || 'latest',
				deployment_environment: editDeploymentEnvironment,
				github_token: editUseGlobalToken ? '' : (editGitHubToken.trim() !== '' ? editGitHubToken : undefined),
				webhook_enabled: editWebhookEnabled,
				webhook_url: editWebhookURL,
				webhook_bearer_token: editUseGlobalWebhookToken ? '' : (editWebhookBearerToken.trim() !== '' ? editWebhookBearerToken : undefined),
				notify_version_found: editNotifyVersionFound,
				notify_deployment_succeeded: editNotifyDeploymentSucceeded,
				notify_deployment_failed: editNotifyDeploymentFailed,
				notify_rollback_succeeded: editNotifyRollbackSucceeded,
				notify_rollback_failed: editNotifyRollbackFailed,
				notify_service_health_changed: editNotifyServiceHealthChanged,
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

	async function handleServiceAdded(data: any) {
		await api.createService(id, data);
		watcher = await api.getWatcher(id);
	}

	async function handleServiceUpdated(svcId: number, data: any) {
		await api.updateService(id, svcId, data);
		watcher = await api.getWatcher(id);
	}

	function openEditServiceDialog(svc: Service) {
		editSvc = svc;
		showEditService = true;
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

	async function sendWebhookTest() {
		try {
			const res = await api.sendWatcherWebhookTest(id);
			triggerMsg = res.message;
			await loadWebhookDeliveries();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to send webhook test';
		}
	}

	async function resumeWebhook(replaySuppressed = false) {
		try {
			const res = await api.resumeWatcherWebhook(id, replaySuppressed);
			triggerMsg = res.message;
			watcher = await api.getWatcher(id);
			await loadWebhookDeliveries();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to resume webhook delivery';
		}
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
		<div class="rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400 flex items-center">
			<AlertCircle class="mr-2 h-4 w-4 shrink-0" />
			<span>{error}</span>
		</div>
	{/if}

	{#if triggerMsg}
		<div class="rounded-lg border border-blue-500/30 bg-blue-500/10 p-4 text-sm text-blue-400 flex items-center">
			<Zap class="mr-2 h-4 w-4 shrink-0" />
			<span>{triggerMsg}</span>
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
						<div class="space-y-2">
							<Label for="editReleaseRef">Release Ref</Label>
							<Input id="editReleaseRef" bind:value={editReleaseRef} placeholder="latest or v1.2.3" />
							<p class="text-xs text-muted-foreground">Use <code>latest</code> to follow new releases, or pin this watcher to a specific release tag.</p>
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
								<div class="flex items-center gap-2 mt-2">
									<Checkbox id="editUseGlobalToken" bind:checked={editUseGlobalToken} />
									<Label for="editUseGlobalToken">Use global `GITHUB_TOKEN`</Label>
								</div>
								<p class="text-xs text-muted-foreground mt-1">Current: {watcher.has_github_token ? (watcher.github_token_masked || 'set') : 'using global token'}</p>
							</div>
						</div>
						<div class="rounded-lg border border-border/60 p-4 space-y-4">
							<div class="flex items-center gap-2">
								<Checkbox id="editWebhookEnabled" bind:checked={editWebhookEnabled} />
								<Label for="editWebhookEnabled">Enable webhook delivery for this watcher</Label>
							</div>
							<div class="grid gap-4 sm:grid-cols-2">
								<div class="space-y-2">
									<Label for="editWebhookURL">Webhook URL</Label>
									<Input id="editWebhookURL" bind:value={editWebhookURL} placeholder="https://example.com/hooks/watcher" />
									<p class="text-xs text-muted-foreground">Leave empty to inherit the global default URL.</p>
								</div>
								<div class="space-y-2">
									<Label for="editWebhookBearerToken">Webhook Bearer Token Override</Label>
									<Input id="editWebhookBearerToken" type="password" bind:value={editWebhookBearerToken} placeholder="Paste new token to replace override" disabled={editUseGlobalWebhookToken} />
									<div class="flex items-center gap-2 mt-2">
										<Checkbox id="editUseGlobalWebhookToken" bind:checked={editUseGlobalWebhookToken} />
										<Label for="editUseGlobalWebhookToken">Use global default bearer token</Label>
									</div>
									<p class="text-xs text-muted-foreground mt-1">Current: {watcher.has_webhook_bearer_token ? (watcher.webhook_bearer_token_masked || 'set') : 'using global webhook token'}</p>
								</div>
							</div>
							<div class="grid gap-2 sm:grid-cols-2 text-sm">
								<label class="flex items-center gap-2"><Checkbox bind:checked={editNotifyVersionFound} /> Version found</label>
								<label class="flex items-center gap-2"><Checkbox bind:checked={editNotifyDeploymentSucceeded} /> Deployment succeeded</label>
								<label class="flex items-center gap-2"><Checkbox bind:checked={editNotifyDeploymentFailed} /> Deployment failed</label>
								<label class="flex items-center gap-2"><Checkbox bind:checked={editNotifyRollbackSucceeded} /> Rollback succeeded</label>
								<label class="flex items-center gap-2"><Checkbox bind:checked={editNotifyRollbackFailed} /> Rollback failed</label>
								<label class="flex items-center gap-2"><Checkbox bind:checked={editNotifyServiceHealthChanged} /> Service health changed</label>
							</div>
						</div>
						<div class="flex items-center gap-2 py-2">
							<Checkbox id="editHcEnabled" bind:checked={editHcEnabled} />
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

		<Tabs.Root
			bind:value={activeTab}
			onValueChange={(v) => {
				if (v) {
					goto(resolve(`/watchers/[id]?tab=${v}`, { id: String(id) }), {
						replaceState: true,
						keepFocus: true,
						noScroll: true
					}).catch(() => {});
				}
			}}
		>
			<Tabs.List>
				<Tabs.Trigger value="overview">Overview</Tabs.Trigger>
				<Tabs.Trigger value="services">Services ({watcher.services.length})</Tabs.Trigger>
				<Tabs.Trigger value="deploys">Deploy History ({deployTotal})</Tabs.Trigger>
				<Tabs.Trigger value="versions">Versions ({versions.length})</Tabs.Trigger>
				<Tabs.Trigger value="polling">Polling History</Tabs.Trigger>
				<Tabs.Trigger value="webhooks">Webhooks ({deliveryTotal})</Tabs.Trigger>
			</Tabs.List>

			<Tabs.Content value="overview" class="mt-4">
				<OverviewTab {watcher} />
			</Tabs.Content>

			<Tabs.Content value="services" class="mt-4">
				<ServicesTab
					{watcher}
					onAddService={() => (showAddService = true)}
					onEditService={openEditServiceDialog}
					onDeleteService={deleteService}
				/>
			</Tabs.Content>

			<Tabs.Content value="deploys" class="mt-4">
				<DeploysTab
					{deploys}
					bind:deployPage
					bind:deployPageSize
					{deployTotal}
					onPageChange={async (p) => {
						deployPage = p;
						await loadDeploys();
					}}
					onPageSizeChange={async (size) => {
						deployPageSize = size;
						deployPage = 1;
						await loadDeploys();
					}}
					watcherId={id}
				/>
			</Tabs.Content>

			<Tabs.Content value="versions" class="mt-4">
				<VersionsTab
					{versions}
					onRollback={openRollbackDialog}
					onDeleteVersion={deleteVersion}
				/>
			</Tabs.Content>

			<Tabs.Content value="polling" class="mt-4">
				<PollingTab
					{polls}
					bind:pollPage
					{pollPageSize}
					{pollTotal}
					bind:pollStatus
					onPageChange={async (p) => {
						pollPage = p;
						await loadPolls();
					}}
					onStatusChange={async (status) => {
						pollStatus = status;
						pollPage = 1;
						await loadPolls();
					}}
				/>
			</Tabs.Content>

			<Tabs.Content value="webhooks" class="mt-4">
				<WebhooksTab
					{watcher}
					deliveries={webhookDeliveries}
					bind:deliveryPage
					bind:deliveryPageSize
					deliveryTotal={deliveryTotal}
					onPageChange={async (p) => {
						deliveryPage = p;
						await loadWebhookDeliveries();
					}}
					onPageSizeChange={async (size) => {
						deliveryPageSize = size;
						deliveryPage = 1;
						await loadWebhookDeliveries();
					}}
					onSendTest={sendWebhookTest}
					onResume={() => resumeWebhook(false)}
					onResumeReplay={() => resumeWebhook(true)}
				/>
			</Tabs.Content>
		</Tabs.Root>
	{/if}
</div>

<AddServiceDialog
	bind:open={showAddService}
	onServiceAdded={handleServiceAdded}
/>

<EditServiceDialog
	bind:open={showEditService}
	service={editSvc}
	onServiceUpdated={handleServiceUpdated}
/>

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
			<div class="flex items-center gap-2 py-1">
				<Checkbox id="rollbackReportGitHub" bind:checked={rollbackReportGitHub} />
				<Label for="rollbackReportGitHub" class="text-sm text-muted-foreground select-none">
					Report rollback to GitHub Deployment API
				</Label>
			</div>
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
