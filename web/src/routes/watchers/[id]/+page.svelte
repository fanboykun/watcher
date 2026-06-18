<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import {
		api,
		type AuthenticatedEventStream,
		type DeployLog,
		type WebhookDelivery,
		type Watcher
	} from '$lib/api';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
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
		Pencil
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
	import RollbackDialog from './components/rollback-dialog.svelte';
	import ConfirmationDialog from './components/confirmation-dialog.svelte';

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

	let showRollbackDialog = $state(false);
	let showConfirmDialog = $state(false);
	let confirming = $state(false);
	let rollbackTargetVersion = $state('');
	let rollbackReportGitHub = $state(true);
	let confirmTitle = $state('');
	let confirmDescription = $state('');
	let confirmActionLabel = $state('Confirm');
	let confirmActionClass = $state('');
	let confirmAction: (() => Promise<void> | void) | null = null;

	let activeTab = $state(page.url.searchParams.get('tab') || 'overview');

	let watcherEventSource: AuthenticatedEventStream | null = null;
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
				const tasks: Array<Promise<unknown>> = [
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
			<a href={resolve(`/watchers/${id}/edit`)}>
				<Button.Root variant="outline" size="sm">
					<Pencil class="mr-2 h-4 w-4" /> Edit Settings
				</Button.Root>
			</a>

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
					readonly={true}
					manageHref={resolve(`/watchers/${id}/edit#services`)}
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

<!-- Rollback Dialog -->
<RollbackDialog 
	onRollback={rollback} 
	bind:open={showRollbackDialog}
	{rollbackTargetVersion} 
	bind:rollbackReportGitHub
/>


<!-- Confirm Action Dialog -->
 <ConfirmationDialog
	bind:open={showConfirmDialog}
	bind:confirmTitle
	bind:confirmDescription
	bind:confirming
	{confirmActionClass}
	{confirmActionLabel}
	onConfirm={runConfirmAction}
 />
