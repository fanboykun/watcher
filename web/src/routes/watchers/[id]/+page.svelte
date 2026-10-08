<script lang="ts">
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import RequestError from '$lib/components/request-error.svelte';
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
	import CandidateTab from './components/candidate-tab.svelte';
	import RollbackDialog from './components/rollback-dialog.svelte';
	import ConfirmationDialog from './components/confirmation-dialog.svelte';

	import RequestLoading from '$lib/components/request-loading.svelte';

	let pendingAction = $state('');
	async function runPending(name: string, action: () => Promise<void>) {
		if (pendingAction) return;
		pendingAction = name;
		error = '';
		try {
			await action();
		} finally {
			pendingAction = '';
		}
	}
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
	let dataError = $state('');
	let loading = $state(true);
	let triggerMsg = $state('');

	let showRollbackDialog = $state(false);
	let showConfirmDialog = $state(false);
	let confirming = $state(false);
	const actionBusy = $derived(
		Boolean(pendingAction) || confirming || watcher?.status === 'deploying'
	);
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
			dataError = `Polling history could not be loaded. ${err instanceof Error ? err.message : 'Try again.'}`;
		}
	};

	const loadDeploys = async () => {
		try {
			const res = await api.watcherDeploys(id, deployPage, deployPageSize);
			deploys = res.data;
			deployTotal = res.total;
		} catch (e) {
			dataError = `Deployment history could not be loaded. ${e instanceof Error ? e.message : 'Try again.'}`;
		}
	};

	const loadWebhookDeliveries = async () => {
		try {
			const res = await api.watcherWebhookDeliveries(id, deliveryPage, deliveryPageSize);
			webhookDeliveries = Array.isArray(res.data) ? res.data : [];
			deliveryTotal = res.total;
		} catch (e) {
			dataError = `Webhook history could not be loaded. ${e instanceof Error ? e.message : 'Try again.'}`;
		}
	};

	async function refreshDetails() {
		dataError = '';
		const results = await Promise.allSettled([
			loadDeploys(),
			api.watcherVersions(id).then((v) => (versions = v)),
			loadPolls(),
			loadWebhookDeliveries()
		]);
		const failed = results.find((result) => result.status === 'rejected');
		if (failed?.status === 'rejected') {
			dataError = `Some history could not be loaded. ${failed.reason instanceof Error ? failed.reason.message : 'Try again.'}`;
		}
	}

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
					tasks.push(api.watcherVersions(id).then((v) => (versions = v)));
				}
				if (includePolls || activeTab === 'polling') {
					tasks.push(loadPolls());
				}
				if (activeTab === 'webhooks') {
					tasks.push(loadWebhookDeliveries());
				}
				await Promise.all(tasks);
			} catch (e) {
				dataError = `Some details could not be refreshed. ${e instanceof Error ? e.message : 'Try again.'}`;
			}
		}, 200);
	}

	onMount(() => {
		const init = async () => {
			try {
				watcher = await api.getWatcher(id);

				await refreshDetails();
			} catch (e) {
				error = e instanceof Error ? e.message : 'Failed to load watcher';
			} finally {
				loading = false;
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
		error = '';
		confirmTitle = opts.title;
		confirmDescription = opts.description;
		confirmActionLabel = opts.actionLabel;
		confirmActionClass = opts.actionClass || '';
		confirmAction = opts.action;
		showConfirmDialog = true;
	}

	async function runConfirmAction() {
		if (!confirmAction || confirming) return;
		error = '';
		confirming = true;
		try {
			await confirmAction();
			if (!error) showConfirmDialog = false;
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
			error = e instanceof Error ? e.message : 'Poll could not be requested';
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
			triggerMsg = `Starting rollback to ${version}...`;
			const res = await api.rollbackWatcher(id, version, reportGithub);
			showRollbackDialog = false;
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
					versions = await api.watcherVersions(id);
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
			scheduleRefresh();
		} catch (e) {
			if (replaySuppressed) throw e;
			error = e instanceof Error ? e.message : 'Failed to resume webhook delivery';
		}
	}

	async function toggleIntercept() {
		try {
			if (!watcher) return;
			const nextVal = !watcher.intercept_next_release;
			await api.interceptWatcher(id, nextVal);
			triggerMsg = nextVal ? 'Will intercept next release.' : 'Intercept disabled.';
			watcher = await api.getWatcher(id);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to toggle intercept';
		}
	}

	async function approveRelease(version?: string) {
		// The candidate editor owns approval errors and keeps its review state.
		const target = version || watcher?.pending_version;
		if (!target) return;
		await api.approveRelease(id, target);
		triggerMsg = 'Release approved! Deploying...';
		scheduleRefresh();
	}

	async function discardRelease() {
		openConfirmDialog({
			title: 'Discard Release Candidate',
			description: `Discard release candidate "${watcher?.pending_version}"? It will not be deployed, its staged candidate configurations will be removed, and polling will resume without deploying this version.`,
			actionLabel: 'Discard Release',
			actionClass: 'bg-red-600 text-white hover:bg-red-700',
			action: async () => {
				try {
					const res = await api.discardRelease(id);
					triggerMsg = res.message;
					watcher = await api.getWatcher(id);
				} catch (e) {
					error = e instanceof Error ? e.message : 'Failed to discard release';
				}
			}
		});
	}

	function hasActiveRollbackPin(w: Watcher | null): boolean {
		if (!w) return false;
		const ignored = (w.max_ignored_version || '').trim();
		if (!ignored) return false;
		return compareSemver(ignored, w.current_version || '') > 0;
	}
</script>

<div class="space-y-6">
	<header class="page-header">
		<div class="page-identity">
			<Button.Root
				href={resolve('/watchers')}
				variant="ghost"
				size="icon"
				aria-label="Back to watchers"><ArrowLeft /></Button.Root
			>
			<div class="min-w-0">
				<h1 class="text-2xl font-semibold tracking-tight">{watcher?.name || 'Watcher'}</h1>
				{#if watcher && watcher.service_name !== watcher.name}<p
						class="text-sm text-muted-foreground"
					>
						{watcher.service_name}
					</p>{/if}
			</div>
			{#if watcher}<span
					class="shrink-0 rounded-full border px-2.5 py-1 text-xs font-medium capitalize {statusColor(
						watcher.status
					)}">{watcher.status.replaceAll('_', ' ')}</span
				>{/if}
		</div>
		{#if watcher}
			<div class="page-actions">
				<Button.Root
					variant="outline"
					size="sm"
					disabled={actionBusy}
					loading={pendingAction === 'pause'}
					onclick={() => runPending('pause', togglePause)}
					>{#if watcher.paused}<Play /> Resume{:else}<Pause /> Pause{/if}</Button.Root
				>
				<Button.Root
					variant="outline"
					size="sm"
					disabled={actionBusy || watcher.paused}
					loading={pendingAction === 'poll'}
					onclick={() => runPending('poll', triggerCheck)}><RefreshCw /> Poll now</Button.Root
				>
				<Button.Root
					href={resolve(`/watchers/${id}/edit`)}
					variant="outline"
					size="sm"
					disabled={actionBusy}><Pencil /> Edit</Button.Root
				>
				<DropdownMenu.Root>
					<DropdownMenu.Trigger
						class={Button.buttonVariants({ variant: 'outline', size: 'sm' })}
						disabled={actionBusy}
						>{#if pendingAction === 'intercept'}<RefreshCw
								class="animate-spin"
							/>Updating…{:else}More actions{/if}</DropdownMenu.Trigger
					>
					<DropdownMenu.Content align="end" class="w-56">
						<DropdownMenu.Item
							disabled={actionBusy}
							onSelect={() => {
								void runPending('intercept', toggleIntercept);
							}}
							><Zap />Intercept next: {watcher.intercept_next_release
								? 'On'
								: 'Off'}</DropdownMenu.Item
						>
						<DropdownMenu.Item
							class="text-amber-400"
							disabled={actionBusy || watcher.status === 'pending_approval'}
							onSelect={triggerRedeploy}><RotateCcw />Redeploy</DropdownMenu.Item
						>
					</DropdownMenu.Content>
				</DropdownMenu.Root>
			</div>
		{/if}
	</header>

	{#if !showConfirmDialog && !showRollbackDialog}<RequestError message={error} />{/if}
	<RequestError
		message={dataError}
		title="Details could not be refreshed"
		onRetry={refreshDetails}
	/>

	{#if triggerMsg}
		<div
			class="flex items-center rounded-lg border border-blue-500/30 bg-blue-500/10 p-4 text-sm text-blue-400"
		>
			<Zap class="mr-2 h-4 w-4 shrink-0" />
			<span>{triggerMsg}</span>
		</div>
	{/if}

	{#if loading}
		<RequestLoading label="Loading watcher details…" />
	{:else if watcher}
		{#if watcher.status === 'pending_approval' && activeTab !== 'candidates'}
			<div class="section-toolbar rounded-lg border border-amber-500/30 bg-amber-500/10 p-4">
				<div class="min-w-0">
					<p class="text-sm font-medium">
						Release <span class="font-mono">{watcher.pending_version}</span> awaits approval
					</p>
					<p class="mt-1 text-xs text-muted-foreground">
						Review its configuration before deploying.
					</p>
				</div>
				<Button.Root
					size="sm"
					disabled={actionBusy}
					onclick={() => {
						activeTab = 'candidates';
						return goto(resolve(`/watchers/[id]?tab=candidates`, { id: String(id) }), {
							replaceState: true,
							noScroll: true
						});
					}}>Review candidate</Button.Root
				>
			</div>
		{/if}

		{#if hasActiveRollbackPin(watcher)}
			<div
				class="section-toolbar mb-4 rounded-lg border border-amber-500/30 bg-amber-500/10 p-4 text-sm text-amber-500"
			>
				<div class="flex items-center gap-2">
					<AlertCircle class="h-4 w-4" />
					<span>
						<strong>Manual rollback pin is active.</strong>
						Current is <code>{watcher.current_version || 'unknown'}</code>; auto-update ignores
						versions
						<code>&lt;= {watcher.max_ignored_version}</code>.
					</span>
				</div>
				<Button.Root
					variant="outline"
					size="sm"
					class="border-amber-500/30 hover:bg-amber-500/20"
					disabled={actionBusy}
					loading={pendingAction === 'resume'}
					onclick={() => runPending('resume', resumeAutoDeploy)}
				>
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
				<Tabs.Trigger value="candidates" class="relative">
					Candidates
					{#if watcher.status === 'pending_approval'}
						<span class="ml-1.5 flex h-2 w-2 rounded-full bg-purple-500"></span>
					{/if}
				</Tabs.Trigger>
				<Tabs.Trigger value="deploys">Deploys ({deployTotal})</Tabs.Trigger>
				<Tabs.Trigger value="versions">Versions ({versions.length})</Tabs.Trigger>
				<Tabs.Trigger value="polling">Polls</Tabs.Trigger>
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

			<Tabs.Content value="candidates" class="mt-4">
				<CandidateTab
					{watcher}
					busy={actionBusy}
					onApprove={(version) => runPending('approve', () => approveRelease(version))}
					onDiscard={discardRelease}
					onRefreshWatcher={async () => {
						watcher = await api.getWatcher(id);
					}}
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
					busy={actionBusy}
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
					{deliveryTotal}
					onPageChange={async (p) => {
						deliveryPage = p;
						await loadWebhookDeliveries();
					}}
					onPageSizeChange={async (size) => {
						deliveryPageSize = size;
						deliveryPage = 1;
						await loadWebhookDeliveries();
					}}
					busy={actionBusy}
					onSendTest={() => runPending('webhook-test', sendWebhookTest)}
					onResume={() => runPending('webhook-resume', () => resumeWebhook(false))}
					onResumeReplay={() => runPending('webhook-replay', () => resumeWebhook(true))}
				/>
			</Tabs.Content>
		</Tabs.Root>
	{/if}
</div>

<!-- Rollback Dialog -->
<RollbackDialog
	errorMessage={error}
	onRollback={(version, report) => runPending('rollback', () => rollback(version, report))}
	pending={pendingAction === 'rollback'}
	bind:open={showRollbackDialog}
	{rollbackTargetVersion}
	bind:rollbackReportGitHub
/>

<!-- Confirm Action Dialog -->
<ConfirmationDialog
	errorMessage={error}
	bind:open={showConfirmDialog}
	bind:confirmTitle
	bind:confirmDescription
	bind:confirming
	{confirmActionClass}
	{confirmActionLabel}
	onConfirm={runConfirmAction}
/>
