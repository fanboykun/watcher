import { api, type Watcher, type CatalogDownload } from '$lib/api';
import type { QueryClient } from '@tanstack/svelte-query';
import { queries, keys } from '$lib/query/options';
import { forget } from '$lib/query/client';
import { resolve } from '$app/paths';
import { goto } from '$app/navigation';
export type ConfirmationOptions = {
	title: string;
	description: string;
	actionLabel: string;
	actionClass?: string;
	action: () => Promise<void> | void;
};
type Context = {
	readonly id: number;
	readonly watcher: Watcher | null;
	client: QueryClient;
	ui: {
		error: string;
		triggerMsg: string;
		activeTab: string;
		rollbackTargetVersion: string;
		rollbackReportGitHub: boolean;
		showRollbackDialog: boolean;
	};
	fetchWatcher: () => Promise<Watcher>;
	openConfirmDialog: (options: ConfirmationOptions) => void;
};
// Every action runs inside a detail command that invalidates the watcher's scope afterwards,
// so actions only refresh explicitly when they navigate based on the new state.
export function createWatcherActions(context: Context) {
	async function configureDownload(download: import('$lib/api').CatalogDownload) {
		try {
			await api.selectCatalogCandidate(context.id, download.id);
			await context.fetchWatcher();
			context.ui.activeTab = 'candidates';
			await goto(resolve('/watchers/[id]?tab=candidates', { id: String(context.id) }), {
				replaceState: true,
				noScroll: true
			});
		} catch (cause) {
			context.ui.error = cause instanceof Error ? cause.message : 'Could not select candidate';
		}
	}
	function deployDownload(download: import('$lib/api').CatalogDownload) {
		context.openConfirmDialog({
			title: `Deploy ${download.version}?`,
			description: `Deploy the downloaded ${download.asset_name} using saved candidate configuration? Services will stop during activation, followed by health checks. Automatic polling keeps its configured release reference.`,
			actionLabel: 'Deploy release',
			action: async () => {
				try {
					await api.deployCatalogRelease(context.id, download.id);
					context.ui.activeTab = 'deploys';
					await goto(resolve('/watchers/[id]?tab=deploys', { id: String(context.id) }), {
						replaceState: true,
						noScroll: true
					});
				} catch (cause) {
					context.ui.error = cause instanceof Error ? cause.message : 'Could not queue deployment';
				}
			}
		});
	}
	function deleteService(svcId: number, name: string) {
		context.openConfirmDialog({
			title: 'Delete Service',
			description: `Delete service "${name}"?`,
			actionLabel: 'Delete',
			actionClass: 'bg-red-600 text-white hover:bg-red-700',
			action: async () => {
				try {
					await api.deleteService(context.id, svcId);
					forget(context.client, keys.service(svcId));
				} catch (e) {
					context.ui.error = e instanceof Error ? e.message : 'Delete failed';
				}
			}
		});
	}
	async function triggerCheck() {
		try {
			const res = await api.triggerCheck(context.id);
			context.ui.triggerMsg = res.message;
			setTimeout(() => (context.ui.triggerMsg = ''), 3000);
		} catch (e) {
			context.ui.error = e instanceof Error ? e.message : 'Poll could not be requested';
		}
	}
	function triggerRedeploy() {
		context.openConfirmDialog({
			title: 'Force Redeploy',
			description: `Force redeployment for "${context.watcher?.name}"? This will restart its services.`,
			actionLabel: 'Redeploy',
			actionClass: 'bg-amber-600 text-white hover:bg-amber-700',
			action: async () => {
				try {
					const res = await api.redeployWatcher(context.id);
					const fallback = resolve(`/watchers/${context.id}/logs/${res.deploy_log_id}`);
					await goto(fallback);
				} catch (e) {
					const msg = e instanceof Error ? e.message : 'Redeploy failed';
					const m = /deploy_log_id["'\s:]+(\d+)/i.exec(msg);
					if (m && m[1]) {
						context.ui.triggerMsg =
							'Deployment is already running. Opening current deployment log...';
						setTimeout(() => (context.ui.triggerMsg = ''), 3500);
						await goto(resolve(`/watchers/${context.id}/logs/${Number(m[1])}`));
						return;
					}
					context.ui.error = msg;
				}
			}
		});
	}
	async function togglePause() {
		if (!context.watcher) return;
		const newPaused = !context.watcher.paused;
		try {
			const updated = await api.updateWatcher(context.id, { paused: newPaused });
			context.client.setQueryData(queries.watcher(updated.id).queryKey, updated);
		} catch (e) {
			context.ui.error = e instanceof Error ? e.message : 'Toggle pause failed';
		}
	}
	function openRollbackDialog(version: string) {
		context.ui.rollbackTargetVersion = version;
		context.ui.rollbackReportGitHub = true;
		context.ui.showRollbackDialog = true;
	}
	async function rollback(version: string, reportGithub = true) {
		try {
			context.ui.triggerMsg = `Starting rollback to ${version}...`;
			const res = await api.rollbackWatcher(context.id, version, reportGithub);
			context.ui.showRollbackDialog = false;
			const fallback = resolve(`/watchers/${context.id}/logs/${res.deploy_log_id}`);
			await goto(fallback);
		} catch (e) {
			context.ui.error = e instanceof Error ? e.message : `Rollback to ${version} failed`;
		}
	}
	function deleteVersion(version: string) {
		context.openConfirmDialog({
			title: 'Delete Version',
			description: `Delete version ${version} from disk? This cannot be undone.`,
			actionLabel: 'Delete',
			actionClass: 'bg-red-600 text-white hover:bg-red-700',
			action: async () => {
				try {
					await api.deleteWatcherVersion(context.id, version);
				} catch (e) {
					context.ui.error = e instanceof Error ? e.message : `Delete ${version} failed`;
				}
			}
		});
	}
	async function resumeAutoDeploy() {
		try {
			await api.resumeWatcherUpdates(context.id);
			context.ui.triggerMsg = `Auto-deploy resumed!`;
		} catch (e) {
			context.ui.error = e instanceof Error ? e.message : `Failed to resume auto deploy`;
		}
	}
	async function sendWebhookTest() {
		try {
			const res = await api.sendWatcherWebhookTest(context.id);
			context.ui.triggerMsg = res.message;
		} catch (e) {
			context.ui.error = e instanceof Error ? e.message : 'Failed to send webhook test';
		}
	}
	async function resumeWebhook(replaySuppressed = false) {
		try {
			const res = await api.resumeWatcherWebhook(context.id, replaySuppressed);
			context.ui.triggerMsg = res.message;
		} catch (e) {
			if (replaySuppressed) throw e;
			context.ui.error = e instanceof Error ? e.message : 'Failed to resume webhook delivery';
		}
	}
	async function toggleIntercept() {
		try {
			if (!context.watcher) return;
			const nextVal = !context.watcher.intercept_next_release;
			await api.interceptWatcher(context.id, nextVal);
			context.ui.triggerMsg = nextVal ? 'Will intercept next release.' : 'Intercept disabled.';
		} catch (e) {
			context.ui.error = e instanceof Error ? e.message : 'Failed to toggle intercept';
		}
	}
	async function approveRelease(version?: string, catalogId = 0) {
		// The candidate editor owns approval errors and keeps its review state.
		const target = version || context.watcher?.pending_version;
		if (!target) return;
		if (catalogId > 0) await api.deployCatalogRelease(context.id, catalogId);
		else await api.approveRelease(context.id, target);
		context.ui.triggerMsg = 'Release approved! Deploying...';
	}
	async function discardRelease() {
		context.openConfirmDialog({
			title: 'Discard Release Candidate',
			description: `Discard release candidate "${context.watcher?.pending_version}"? It will not be deployed, its staged candidate configurations will be removed, and polling will resume without deploying this version.`,
			actionLabel: 'Discard Release',
			actionClass: 'bg-red-600 text-white hover:bg-red-700',
			action: async () => {
				try {
					const res = await api.discardRelease(context.id);
					context.ui.triggerMsg = res.message;
				} catch (e) {
					context.ui.error = e instanceof Error ? e.message : 'Failed to discard release';
				}
			}
		});
	}
	return {
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
	};
}
