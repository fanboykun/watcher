import { createQuery, createMutation, useQueryClient } from '@tanstack/svelte-query';
import { queries, keys } from '$lib/query/options';
import { invalidate, message } from '$lib/query/client';
import { untrack } from 'svelte';
import { createCommand } from '$lib/query/command.svelte';
import { page } from '$app/state';
import { api, type Service, type Watcher } from '$lib/api';
import type { WebhookSelectionState } from '$lib/webhooks';
import { Save } from '@lucide/svelte';
export function createWatcherSettings() {
	const id = $derived(Number(page.params.id));

	const client = useQueryClient();
	const detail = createQuery(() => queries.watcher(id));
	const watcher = $derived(detail.data ?? null);
	const saveMutation = createMutation(() => ({
		mutationFn: (payload: import('$lib/api').WatcherWritePayload) => api.updateWatcher(id, payload),
		onSuccess: (value) => {
			client.setQueryData(queries.watcher(value.id).queryKey, value);
			return invalidate(client, keys.watchers);
		}
	}));
	const testMutation = createMutation(() => ({ mutationFn: () => api.sendWatcherWebhookTest(id) }));
	const confirmMutation = createCommand(() => [keys.watcher(id), keys.services, keys.watchers]);
	const ui = $state({
		error: '',
		success: '',
		activeSection:
			page.url.hash === '#services'
				? 'services'
				: page.url.hash === '#webhooks'
					? 'webhooks'
					: 'source',
		showConfirmDialog: false,
		confirmTitle: '',
		confirmDescription: '',
		confirmActionLabel: 'Confirm',
		confirmActionClass: '',
		editInterval: 60,
		editMetadataURL: '',
		editReleaseRef: 'latest',
		editAutoDeploy: true,
		editHcEnabled: false,
		editHcURL: '',
		editMaxKeptVersions: 3,
		editDeploymentEnvironment: '',
		editGitHubToken: '',
		editUseGlobalToken: false,
		editWebhookEnabled: false,
		editWebhookURL: '',
		editWebhookSigningSecret: '',
		editUseGlobalWebhookToken: false,
		webhookSelections: {
			notify_version_found: false,
			notify_deployment_succeeded: false,
			notify_deployment_failed: false,
			notify_rollback_succeeded: false,
			notify_rollback_failed: false,
			notify_service_health_changed: false
		} as WebhookSelectionState
	});

	const saving = $derived(saveMutation.isPending);
	const loading = $derived(detail.isPending);

	const sendingTest = $derived(testMutation.isPending);

	const confirming = $derived(confirmMutation.isPending);

	let confirmAction: (() => Promise<void> | void) | null = null;

	let initializedID: number | undefined;
	$effect(() => {
		const value = detail.data;
		if (value && value.id !== initializedID)
			untrack(() => {
				initializedID = value.id;
				syncEditForm();
			});
	});
	async function loadWatcher(syncForm = true) {
		const result = await detail.refetch();
		if (syncForm && result.data) syncEditForm();
	}

	function syncEditForm() {
		if (!watcher) return;
		ui.editInterval = watcher.check_interval_sec;
		ui.editAutoDeploy = watcher.auto_deploy ?? true;
		ui.editMetadataURL = watcher.metadata_url;
		ui.editReleaseRef = watcher.release_ref || 'latest';
		ui.editHcEnabled = watcher.hc_enabled;
		ui.editHcURL = watcher.hc_url;
		ui.editMaxKeptVersions = watcher.max_kept_versions;
		ui.editDeploymentEnvironment = watcher.deployment_environment || '';
		ui.editGitHubToken = '';
		ui.editUseGlobalToken = !watcher.has_github_token;
		ui.editWebhookEnabled = watcher.webhook_enabled;
		ui.editWebhookURL = watcher.webhook_url || '';
		ui.editWebhookSigningSecret = '';
		ui.editUseGlobalWebhookToken = !watcher.has_webhook_signing_secret;
		ui.webhookSelections = {
			notify_version_found: watcher.notify_version_found,
			notify_deployment_succeeded: watcher.notify_deployment_succeeded,
			notify_deployment_failed: watcher.notify_deployment_failed,
			notify_rollback_succeeded: watcher.notify_rollback_succeeded,
			notify_rollback_failed: watcher.notify_rollback_failed,
			notify_service_health_changed: watcher.notify_service_health_changed
		};
	}

	async function saveEdit() {
		if (saving || sendingTest || confirming || !watcher) return;
		ui.error = '';
		ui.success = '';
		try {
			const saved = await saveMutation.mutateAsync({
				check_interval_sec: ui.editInterval,
				auto_deploy: ui.editAutoDeploy,
				metadata_url: ui.editMetadataURL,
				release_ref: ui.editReleaseRef.trim() || 'latest',
				deployment_environment: ui.editDeploymentEnvironment,
				github_token: ui.editUseGlobalToken
					? ''
					: ui.editGitHubToken.trim() !== ''
						? ui.editGitHubToken
						: undefined,
				webhook_enabled: ui.editWebhookEnabled,
				webhook_url: ui.editWebhookURL,
				webhook_signing_secret: ui.editUseGlobalWebhookToken
					? ''
					: ui.editWebhookSigningSecret.trim() !== ''
						? ui.editWebhookSigningSecret
						: undefined,
				notify_version_found: ui.webhookSelections.notify_version_found,
				notify_deployment_succeeded: ui.webhookSelections.notify_deployment_succeeded,
				notify_deployment_failed: ui.webhookSelections.notify_deployment_failed,
				notify_rollback_succeeded: ui.webhookSelections.notify_rollback_succeeded,
				notify_rollback_failed: ui.webhookSelections.notify_rollback_failed,
				notify_service_health_changed: ui.webhookSelections.notify_service_health_changed,
				hc_enabled: ui.editHcEnabled,
				hc_url: ui.editHcURL,
				max_kept_versions: ui.editMaxKeptVersions
			});
			if (saved.id !== id) return;
			ui.success = 'Watcher settings saved.';
			syncEditForm();
		} catch (err) {
			ui.error = err instanceof Error ? err.message : 'Save failed';
		}
	}

	async function sendWebhookTest() {
		if (sendingTest || saving || confirming) return;
		ui.error = '';
		ui.success = '';
		try {
			const res = await testMutation.mutateAsync();
			ui.success = res.message;
			await detail.refetch();
		} catch (err) {
			ui.error = err instanceof Error ? err.message : 'Failed to send webhook test';
		}
	}

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
			await confirmMutation.mutateAsync({
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

	function deleteService(svcId: number, name: string) {
		openConfirmDialog({
			title: 'Delete Service',
			description: `Delete service "${name}"?`,
			actionLabel: 'Delete',
			actionClass: 'bg-red-600 text-white hover:bg-red-700',
			action: async () => {
				try {
					await api.deleteService(id, svcId);
					await loadWatcher(false);
					ui.success = 'Service deleted.';
				} catch (err) {
					ui.error = err instanceof Error ? err.message : 'Delete failed';
				}
			}
		});
	}
	return {
		ui,
		get id() {
			return id;
		},
		get watcher() {
			return watcher;
		},
		get detail() {
			return detail;
		},
		get loading() {
			return loading;
		},
		saveEdit,
		get saving() {
			return saving;
		},
		get sendingTest() {
			return sendingTest;
		},
		get confirming() {
			return confirming;
		},
		sendWebhookTest,
		deleteService,
		runConfirmAction
	};
}
