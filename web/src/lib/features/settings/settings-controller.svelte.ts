import { draftKey } from './settings-draft';
import { untrack } from 'svelte';
import { createQuery, createMutation, useQueryClient } from '@tanstack/svelte-query';
import { queries, keys } from '$lib/query/options';
import { invalidate, message } from '$lib/query/client';
import { createSelfUpdate } from '$lib/query/self-update.svelte';
import { page } from '$app/state';
import { afterNavigate, goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { api, auth } from '$lib/api';
export function createSettings() {
	const client = useQueryClient();
	const versionQuery = createQuery(() => queries.selfVersion());
	const configQuery = createQuery(() => queries.selfConfig());
	const authQuery = createQuery(() => queries.auth());
	const update = createSelfUpdate();
	const versionInfo = $derived(versionQuery.data ?? null);
	const updateInfo = $derived(update.check.data ?? null);
	const agentConfig = $derived(configQuery.data ?? null);
	const authStatus = $derived(authQuery.data ?? null);
	const loading = $derived(versionQuery.isPending || configQuery.isPending || authQuery.isPending);
	const loadError = $derived(message(versionQuery.error || configQuery.error || authQuery.error));
	const configMutation = createMutation(() => ({
		mutationFn: api.updateSelfConfig,
		onSuccess: (value) => {
			client.setQueryData(queries.selfConfig().queryKey, value.config);
			return invalidate(client, keys.watchers);
		}
	}));
	const passwordMutation = createMutation(() => ({
		mutationFn: (v: { current: string; next: string }) => api.updateAuthPassword(v.current, v.next)
	}));
	const restartMutation = createMutation(() => ({ mutationFn: api.selfRestart }));
	const uninstallMutation = createMutation(() => ({ mutationFn: api.selfUninstall }));

	const ui = $state({
		configError: '',
		configSuccess: '',
		passwordError: '',
		passwordSuccess: '',
		maintenanceError: '',
		maintenanceSuccess: '',
		saveNotes: [] as string[],
		activeSection:
			page.url.searchParams.get('tab') === 'maintenance' ? 'maintenance' : 'deployment',
		savedDraft: '',
		uninstallScript: '',
		githubTokenInput: '',
		clearGitHubToken: false,
		currentPassword: '',
		newPassword: '',
		confirmPassword: '',
		cfgEnvironment: '',
		cfgGithubDeployEnabled: true,
		cfgLogDir: '',
		cfgNssmPath: '',
		cfgDBPath: '',
		cfgAPIPort: '',
		cfgAPIBaseURL: '',
		cfgWebAssetsPath: '',
		cfgWatcherRepoURL: '',
		cfgWatcherServiceName: '',
		cfgWebhookDefaultURL: '',
		cfgWebhookTimeoutSec: 10,
		cfgWebhookRetryScheduleSec: '0,10,60,300',
		cfgWebhookAutoPauseEnabled: true,
		cfgWebhookAutoPauseAfterFailures: 5,
		cfgWebhookEventRetentionDays: 90,
		cfgWebhookDeliveryRetentionDays: 30,
		webhookDefaultSigningSecretInput: '',
		clearWebhookDefaultSigningSecret: false,
		showRestartDialog: false,
		showUpdateDialog: false
	});

	const updateState = $derived(update.snapshot);

	const configDirty = $derived(Boolean(agentConfig) && draftKey(ui) !== ui.savedDraft);

	function resetConfig() {
		syncConfigForm();
		ui.githubTokenInput = '';
		ui.clearGitHubToken = false;
		ui.webhookDefaultSigningSecretInput = '';
		ui.clearWebhookDefaultSigningSecret = false;
		ui.savedDraft = draftKey(ui);
		ui.configError = '';
		ui.configSuccess = '';
		ui.saveNotes = [];
	}

	const isChecking = $derived(update.check.isFetching);
	const isUpdating = $derived(
		updateState.updateStatus === 'updating' || updateState.updateStatus === 'applied'
	);
	const isSavingConfig = $derived(configMutation.isPending);
	const isSavingPassword = $derived(passwordMutation.isPending);
	const isRestarting = $derived(restartMutation.isPending);
	const agentBusy = $derived(isSavingConfig || isRestarting || isUpdating);

	let initialized = false;
	$effect(() => {
		if (configQuery.data && !initialized)
			untrack(() => {
				initialized = true;
				resetConfig();
			});
	});
	$effect(() => {
		if (!loading && !loadError && initialized && page.url.searchParams.get('action') === 'update')
			void untrack(updateFromBanner);
	});

	afterNavigate(() => {
		if (page.url.searchParams.get('tab') === 'maintenance') ui.activeSection = 'maintenance';
		if (!loading && !loadError && page.url.searchParams.get('action') === 'update') {
			void updateFromBanner();
		}
	});

	async function updateFromBanner() {
		ui.activeSection = 'maintenance';
		// Consume the action before issuing a mutation, including on refresh or retry.
		await goto(resolve('/settings?tab=maintenance'), { replaceState: true, noScroll: true });
		if (configDirty || isSavingPassword || agentBusy) {
			if (!isUpdating)
				ui.maintenanceError = 'Save or discard your pending changes before updating Watcher.';
			return;
		}
		await performUpdate();
	}

	async function loadSettings() {
		await Promise.all([versionQuery.refetch(), configQuery.refetch(), authQuery.refetch()]);
	}

	function syncConfigForm() {
		if (!agentConfig) return;
		ui.cfgEnvironment = agentConfig.environment;
		ui.cfgGithubDeployEnabled = agentConfig.github_deploy_enabled;
		ui.cfgLogDir = agentConfig.log_dir;
		ui.cfgNssmPath = agentConfig.nssm_path;
		ui.cfgDBPath = agentConfig.db_path;
		ui.cfgAPIPort = agentConfig.api_port;
		ui.cfgAPIBaseURL = agentConfig.api_base_url;
		ui.cfgWebAssetsPath = agentConfig.web_assets_path ?? '';
		ui.cfgWatcherRepoURL = agentConfig.watcher_repo_url;
		ui.cfgWatcherServiceName = agentConfig.watcher_service_name;
		ui.cfgWebhookDefaultURL = agentConfig.webhook_default_url;
		ui.cfgWebhookTimeoutSec = agentConfig.webhook_timeout_sec;
		ui.cfgWebhookRetryScheduleSec = agentConfig.webhook_retry_schedule_sec;
		ui.cfgWebhookAutoPauseEnabled = agentConfig.webhook_auto_pause_enabled;
		ui.cfgWebhookAutoPauseAfterFailures = agentConfig.webhook_auto_pause_after_failures;
		ui.cfgWebhookEventRetentionDays = agentConfig.webhook_event_retention_days;
		ui.cfgWebhookDeliveryRetentionDays = agentConfig.webhook_delivery_retention_days;
	}

	async function saveAgentConfig() {
		if (agentBusy || isSavingPassword || !agentConfig || !configDirty) return;
		ui.configError = '';
		ui.configSuccess = '';
		try {
			const payload: Record<string, string | boolean | number> = {
				environment: ui.cfgEnvironment,
				github_deploy_enabled: ui.cfgGithubDeployEnabled,
				log_dir: ui.cfgLogDir,
				nssm_path: ui.cfgNssmPath,
				db_path: ui.cfgDBPath,
				api_port: ui.cfgAPIPort,
				api_base_url: ui.cfgAPIBaseURL,
				web_assets_path: ui.cfgWebAssetsPath.trim(),
				watcher_repo_url: ui.cfgWatcherRepoURL,
				watcher_service_name: ui.cfgWatcherServiceName,
				webhook_default_url: ui.cfgWebhookDefaultURL,
				webhook_timeout_sec: ui.cfgWebhookTimeoutSec,
				webhook_retry_schedule_sec: ui.cfgWebhookRetryScheduleSec,
				webhook_auto_pause_enabled: ui.cfgWebhookAutoPauseEnabled,
				webhook_auto_pause_after_failures: ui.cfgWebhookAutoPauseAfterFailures,
				webhook_event_retention_days: ui.cfgWebhookEventRetentionDays,
				webhook_delivery_retention_days: ui.cfgWebhookDeliveryRetentionDays
			};

			if (ui.clearGitHubToken) {
				payload.github_token = '';
			} else if (ui.githubTokenInput.trim()) {
				payload.github_token = ui.githubTokenInput.trim();
			}
			if (ui.clearWebhookDefaultSigningSecret) {
				payload.webhook_default_signing_secret = '';
			} else if (ui.webhookDefaultSigningSecretInput.trim()) {
				payload.webhook_default_signing_secret = ui.webhookDefaultSigningSecretInput.trim();
			}

			const res = await configMutation.mutateAsync(payload);
			syncConfigForm();
			ui.githubTokenInput = '';
			ui.clearGitHubToken = false;
			ui.webhookDefaultSigningSecretInput = '';
			ui.clearWebhookDefaultSigningSecret = false;
			ui.savedDraft = draftKey(ui);
			ui.configSuccess = res.message;
			ui.saveNotes = res.notes || [];
		} catch (e) {
			ui.configError = e instanceof Error ? e.message : 'Failed to save config';
		}
	}

	async function savePassword() {
		if (isSavingPassword || agentBusy) return;
		ui.passwordError = '';
		ui.passwordSuccess = '';
		try {
			if (!ui.newPassword.trim()) {
				throw new Error('New password is required');
			}
			if (ui.newPassword !== ui.confirmPassword) {
				throw new Error('New password confirmation does not match');
			}
			const res = await passwordMutation.mutateAsync({
				current: ui.currentPassword,
				next: ui.newPassword
			});
			auth.setPassword(ui.newPassword);
			client.setQueryData(queries.auth().queryKey, {
				authenticated: true,
				using_default_password: res.using_default_password
			});
			ui.currentPassword = '';
			ui.newPassword = '';
			ui.confirmPassword = '';
			ui.passwordSuccess = res.message;
		} catch (e) {
			ui.passwordError = e instanceof Error ? e.message : 'Failed to update password';
		}
	}

	async function restartWatcherService() {
		if (agentBusy || isSavingPassword || configDirty) return;
		ui.maintenanceError = '';
		ui.maintenanceSuccess = '';
		try {
			const res = await restartMutation.mutateAsync();
			ui.maintenanceSuccess = `${res.message} (${res.service_name})`;
			ui.showRestartDialog = false;
		} catch (e) {
			ui.maintenanceError = e instanceof Error ? e.message : 'Failed to restart watcher service';
		}
	}

	async function checkForUpdates() {
		ui.maintenanceError = '';
		try {
			const result = await update.check.refetch({ throwOnError: true });
			const info = result.data;
			if (info && !info.update_available) {
				update.clearDismissal();
			}
		} catch (e) {
			ui.maintenanceError = e instanceof Error ? e.message : 'Check failed';
		}
	}

	async function performUpdate() {
		if (agentBusy || isSavingPassword || configDirty) return;
		ui.maintenanceError = '';
		ui.showUpdateDialog = false;
		await update.perform();
	}

	async function generateUninstall() {
		try {
			const res = await uninstallMutation.mutateAsync();
			ui.uninstallScript = res.script;
		} catch (e) {
			ui.maintenanceError = e instanceof Error ? e.message : 'Uninstall generation failed';
		}
	}

	async function copyUninstallScript() {
		if (!ui.uninstallScript) return;
		try {
			await navigator.clipboard.writeText(ui.uninstallScript);
		} catch (e) {
			ui.maintenanceError = 'Failed to copy script';
		}
	}
	return {
		ui,
		get loading() {
			return loading;
		},
		get loadError() {
			return loadError;
		},
		loadSettings,
		get agentBusy() {
			return agentBusy;
		},
		get isSavingPassword() {
			return isSavingPassword;
		},
		get agentConfig() {
			return agentConfig;
		},
		savePassword,
		get authStatus() {
			return authStatus;
		},
		get configDirty() {
			return configDirty;
		},
		get updateState() {
			return updateState;
		},
		performUpdate,
		get versionInfo() {
			return versionInfo;
		},
		checkForUpdates,
		get isChecking() {
			return isChecking;
		},
		get updateInfo() {
			return updateInfo;
		},
		get isUpdating() {
			return isUpdating;
		},
		generateUninstall,
		copyUninstallScript,
		resetConfig,
		get isSavingConfig() {
			return isSavingConfig;
		},
		saveAgentConfig,
		get isRestarting() {
			return isRestarting;
		},
		restartWatcherService
	};
}
