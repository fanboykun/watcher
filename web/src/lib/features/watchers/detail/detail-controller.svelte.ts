import { tick } from 'svelte';
import { createWatcherActions } from './detail-actions';
import { createQuery, useQueryClient } from '@tanstack/svelte-query';
import { queries, keys } from '$lib/query/options';
import { invalidate, message } from '$lib/query/client';
import { createCommand } from '$lib/query/command.svelte';
import { page } from '$app/state';
import { api, type Watcher } from '$lib/api';
import { resolve } from '$app/paths';
import { goto } from '$app/navigation';
import { compareSemver } from '$lib/utils';
export function createWatcherDetail() {
	// Sub-components

	const client = useQueryClient();
	const id = $derived(Number(page.params.id));
	const detail = createQuery(() => queries.watcher(id));
	const watcher = $derived(detail.data ?? null);
	const operation = createCommand(() => [keys.watchers, keys.services, keys.status]);
	const confirmation = createCommand(() => [keys.watchers, keys.services, keys.status]);
	const pendingAction = $derived(operation.isPending ? (operation.variables?.name ?? '') : '');
	async function runPending(name: string, action: () => Promise<void>) {
		if (operation.isPending) return;
		ui.error = '';
		await operation.mutateAsync({ name, run: action });
	}

	const reviewableCandidate = $derived(
		watcher &&
			(watcher.status === 'pending_approval' ||
				(watcher.pending_catalog_id > 0 &&
					watcher.status !== 'approved' &&
					watcher.status !== 'deploying'))
	);
	const ui = $state({
		deployPage: 1,
		deployPageSize: 10,
		deliveryPage: 1,
		deliveryPageSize: 20,
		pollPage: 1,
		pollPageSize: 10,
		pollStatus: 'all',
		error: '',
		triggerMsg: '',
		showRollbackDialog: false,
		showConfirmDialog: false,
		rollbackTargetVersion: '',
		rollbackReportGitHub: true,
		confirmTitle: '',
		confirmDescription: '',
		confirmActionLabel: 'Confirm',
		confirmActionClass: '',
		activeTab: page.url.searchParams.get('tab') || 'overview'
	});

	const confirming = $derived(confirmation.isPending);
	const actionBusy = $derived(
		Boolean(pendingAction) || confirming || watcher?.status === 'deploying'
	);

	let confirmAction: (() => Promise<void> | void) | null = null;

	const deployQuery = createQuery(() => ({
		...queries.deploys(id, ui.deployPage, ui.deployPageSize),
		enabled: !!watcher
	}));
	const pollQuery = createQuery(() => ({
		...queries.polls(id, ui.pollPage, ui.pollPageSize, ui.pollStatus),
		enabled: !!watcher
	}));
	const versionQuery = createQuery(() => ({ ...queries.versions(id), enabled: !!watcher }));
	const deliveryQuery = createQuery(() => ({
		...queries.deliveries(id, ui.deliveryPage, ui.deliveryPageSize),
		enabled: !!watcher
	}));
	const deploys = $derived(deployQuery.data?.data ?? []);
	const deployTotal = $derived(deployQuery.data?.total ?? 0);
	const polls = $derived(pollQuery.data?.data ?? []);
	const pollTotal = $derived(pollQuery.data?.total ?? 0);
	const versions = $derived(versionQuery.data?.versions ?? []);
	const downloads = $derived(versionQuery.data?.downloads ?? []);
	const webhookDeliveries = $derived(deliveryQuery.data?.data ?? []);
	const deliveryTotal = $derived(deliveryQuery.data?.total ?? 0);
	const loading = $derived(detail.isPending);
	const dataError = $derived(
		message(deployQuery.error || pollQuery.error || versionQuery.error || deliveryQuery.error)
	);
	async function fetchWatcher() {
		const result = await detail.refetch({ throwOnError: true });
		return result.data!;
	}
	async function loadVersions() {
		await versionQuery.refetch();
	}

	async function loadPolls() {
		await tick();
		await pollQuery.refetch({ cancelRefetch: false });
	}
	async function loadDeploys() {
		await tick();
		await deployQuery.refetch({ cancelRefetch: false });
	}
	async function loadWebhookDeliveries() {
		await tick();
		await deliveryQuery.refetch({ cancelRefetch: false });
	}
	async function refreshDetails() {
		await Promise.all([loadDeploys(), loadVersions(), loadPolls(), loadWebhookDeliveries()]);
	}
	async function loadWatcher() {
		await detail.refetch();
	}
	function scheduleRefresh(includeVersions = false, includePolls = false) {
		void invalidate(
			client,
			queries.watcher(id).queryKey,
			[...keys.watcher(id), 'deploys'],
			keys.watchers,
			keys.services,
			keys.status
		);
		if (includeVersions) void invalidate(client, [...keys.watcher(id), 'versions']);
		if (includePolls || ui.activeTab === 'polling')
			void invalidate(client, [...keys.watcher(id), 'polls']);
		if (ui.activeTab === 'webhooks') void invalidate(client, [...keys.watcher(id), 'deliveries']);
	}
	$effect(() => {
		const resourceID = id;
		const watcherEventSource = api.streamWatcherEvents(
			id,
			async (data) => {
				try {
					const ev = JSON.parse(data) as {
						type?: string;
						watcher_id?: number;
						data?: { deploy_log_id?: number };
					};
					if (resourceID !== id || (ev.watcher_id && ev.watcher_id !== resourceID)) return;
					switch (ev.type) {
						case 'deploy_started': {
							const logId = ev.data?.deploy_log_id;
							if (typeof logId === 'number' && Number.isSafeInteger(logId) && logId > 0) {
								await goto(
									resolve('/watchers/[id]/logs/[logId]', { id: String(id), logId: String(logId) })
								);
							} else scheduleRefresh(false, false);
							break;
						}
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

		return () => watcherEventSource.close();
	});

	function openConfirmDialog(opts: {
		title: string;
		description: string;
		actionLabel: string;
		actionClass?: string;
		action: () => Promise<void> | void;
	}) {
		ui.error = '';
		ui.confirmTitle = opts.title;
		ui.confirmDescription = opts.description;
		ui.confirmActionLabel = opts.actionLabel;
		ui.confirmActionClass = opts.actionClass || '';
		confirmAction = opts.action;
		ui.showConfirmDialog = true;
	}

	async function runConfirmAction() {
		if (!confirmAction || confirming) return;
		ui.error = '';
		try {
			await confirmation.mutateAsync({
				name: 'confirm',
				run: async () => {
					await confirmAction?.();
				}
			});
			if (!ui.error) ui.showConfirmDialog = false;
		} catch (cause) {
			ui.error = message(cause);
		}
	}

	function hasActiveRollbackPin(w: Watcher | null): boolean {
		if (!w) return false;
		const ignored = (w.max_ignored_version || '').trim();
		if (!ignored) return false;
		return compareSemver(ignored, w.current_version || '') > 0;
	}
	const {
		configureDownload,
		deployDownload,
		deleteService,
		triggerCheck,
		triggerRedeploy,
		togglePause,
		openRollbackDialog,
		rollback,
		deleteVersion,
		resumeAutoDeploy,
		sendWebhookTest,
		resumeWebhook,
		toggleIntercept,
		approveRelease,
		discardRelease
	} = createWatcherActions({
		get id() {
			return id;
		},
		get watcher() {
			return watcher;
		},
		ui,
		client,
		fetchWatcher,
		loadDeploys,
		loadVersions,
		loadWebhookDeliveries,
		scheduleRefresh,
		openConfirmDialog
	});

	return {
		ui,
		get watcher() {
			return watcher;
		},
		get actionBusy() {
			return actionBusy;
		},
		get pendingAction() {
			return pendingAction;
		},
		runPending,
		togglePause,
		triggerCheck,
		get id() {
			return id;
		},
		toggleIntercept,
		triggerRedeploy,
		get detail() {
			return detail;
		},
		loadWatcher,
		get dataError() {
			return dataError;
		},
		refreshDetails,
		get loading() {
			return loading;
		},
		get reviewableCandidate() {
			return reviewableCandidate;
		},
		hasActiveRollbackPin,
		resumeAutoDeploy,
		loadVersions,
		get deployTotal() {
			return deployTotal;
		},
		get versions() {
			return versions;
		},
		get downloads() {
			return downloads;
		},
		get deliveryTotal() {
			return deliveryTotal;
		},
		fetchWatcher,
		loadDeploys,
		approveRelease,
		discardRelease,
		get deploys() {
			return deploys;
		},
		configureDownload,
		deployDownload,
		openRollbackDialog,
		deleteVersion,
		get polls() {
			return polls;
		},
		get pollTotal() {
			return pollTotal;
		},
		loadPolls,
		get webhookDeliveries() {
			return webhookDeliveries;
		},
		loadWebhookDeliveries,
		sendWebhookTest,
		resumeWebhook,
		rollback,
		get confirming() {
			return confirming;
		},
		runConfirmAction
	};
}
