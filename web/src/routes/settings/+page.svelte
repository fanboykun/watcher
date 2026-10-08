<script lang="ts">
	import RequestError from '$lib/components/request-error.svelte';
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import {
		api,
		auth,
		type SelfVersionResponse,
		type SelfUpdateCheckResponse,
		type SelfConfigResponse
	} from '$lib/api';
	import {
		clearSelfUpdateCache,
		clearSelfUpdateDismissal,
		getSelfUpdateSnapshot,
		lookupSelfUpdate
	} from '$lib/self-update';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import {
		Download,
		Info,
		RotateCcw,
		AlertTriangle,
		CheckCircle2,
		Copy,
		LockKeyhole
	} from '@lucide/svelte';
	import * as Tabs from '$lib/components/ui/tabs';
	import RequestLoading from '$lib/components/request-loading.svelte';

	let versionInfo = $state<SelfVersionResponse | null>(null);
	let updateInfo = $state<SelfUpdateCheckResponse | null>(null);
	let agentConfig = $state<SelfConfigResponse | null>(null);
	let authStatus = $state<{ authenticated: boolean; using_default_password: boolean } | null>(null);
	let loading = $state(true);
	let loadError = $state('');
	let configError = $state('');
	let configSuccess = $state('');
	let passwordError = $state('');
	let passwordSuccess = $state('');
	let maintenanceError = $state('');
	let maintenanceSuccess = $state('');
	let saveNotes = $state<string[]>([]);
	let activeSection = $state('deployment');
	let savedDraft = $state('');
	const configDirty = $derived(Boolean(agentConfig) && draftKey() !== savedDraft);

	function draftKey() {
		return JSON.stringify([
			cfgEnvironment,
			cfgGithubDeployEnabled,
			cfgLogDir,
			cfgNssmPath,
			cfgDBPath,
			cfgAPIPort,
			cfgAPIBaseURL,
			cfgWebAssetsPath,
			cfgWatcherRepoURL,
			cfgWatcherServiceName,
			cfgWebhookDefaultURL,
			cfgWebhookTimeoutSec,
			cfgWebhookRetryScheduleSec,
			cfgWebhookAutoPauseEnabled,
			cfgWebhookAutoPauseAfterFailures,
			cfgWebhookEventRetentionDays,
			cfgWebhookDeliveryRetentionDays,
			githubTokenInput,
			clearGitHubToken,
			webhookDefaultSigningSecretInput,
			clearWebhookDefaultSigningSecret
		]);
	}

	function resetConfig() {
		syncConfigForm();
		githubTokenInput = '';
		clearGitHubToken = false;
		webhookDefaultSigningSecretInput = '';
		clearWebhookDefaultSigningSecret = false;
		savedDraft = draftKey();
		configError = '';
		configSuccess = '';
		saveNotes = [];
	}

	let isChecking = $state(false);
	let isUpdating = $state(false);
	let isSavingConfig = $state(false);
	let isSavingPassword = $state(false);
	let isRestarting = $state(false);
	const agentBusy = $derived(isSavingConfig || isRestarting || isUpdating);
	let uninstallScript = $state('');
	let githubTokenInput = $state('');
	let clearGitHubToken = $state(false);
	let currentPassword = $state('');
	let newPassword = $state('');
	let confirmPassword = $state('');

	let cfgEnvironment = $state('');
	let cfgGithubDeployEnabled = $state(true);
	let cfgLogDir = $state('');
	let cfgNssmPath = $state('');
	let cfgDBPath = $state('');
	let cfgAPIPort = $state('');
	let cfgAPIBaseURL = $state('');
	let cfgWebAssetsPath = $state('');
	let cfgWatcherRepoURL = $state('');
	let cfgWatcherServiceName = $state('');
	let cfgWebhookDefaultURL = $state('');
	let cfgWebhookTimeoutSec = $state(10);
	let cfgWebhookRetryScheduleSec = $state('0,10,60,300');
	let cfgWebhookAutoPauseEnabled = $state(true);
	let cfgWebhookAutoPauseAfterFailures = $state(5);
	let cfgWebhookEventRetentionDays = $state(90);
	let cfgWebhookDeliveryRetentionDays = $state(30);
	let webhookDefaultSigningSecretInput = $state('');
	let clearWebhookDefaultSigningSecret = $state(false);
	let showRestartDialog = $state(false);
	let showUpdateDialog = $state(false);

	onMount(() => {
		void loadSettings();
	});

	async function loadSettings() {
		loading = true;
		loadError = '';
		try {
			updateInfo = getSelfUpdateSnapshot().info;
			[versionInfo, agentConfig, authStatus] = await Promise.all([
				api.selfVersion(),
				api.selfConfig(),
				api.authStatus()
			]);
			resetConfig();
			void lookupSelfUpdate({ silent: true }).then((info) => {
				if (info) updateInfo = info;
			});
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Global settings could not be loaded';
		} finally {
			loading = false;
		}
	}

	function syncConfigForm() {
		if (!agentConfig) return;
		cfgEnvironment = agentConfig.environment;
		cfgGithubDeployEnabled = agentConfig.github_deploy_enabled;
		cfgLogDir = agentConfig.log_dir;
		cfgNssmPath = agentConfig.nssm_path;
		cfgDBPath = agentConfig.db_path;
		cfgAPIPort = agentConfig.api_port;
		cfgAPIBaseURL = agentConfig.api_base_url;
		cfgWebAssetsPath = agentConfig.web_assets_path ?? '';
		cfgWatcherRepoURL = agentConfig.watcher_repo_url;
		cfgWatcherServiceName = agentConfig.watcher_service_name;
		cfgWebhookDefaultURL = agentConfig.webhook_default_url;
		cfgWebhookTimeoutSec = agentConfig.webhook_timeout_sec;
		cfgWebhookRetryScheduleSec = agentConfig.webhook_retry_schedule_sec;
		cfgWebhookAutoPauseEnabled = agentConfig.webhook_auto_pause_enabled;
		cfgWebhookAutoPauseAfterFailures = agentConfig.webhook_auto_pause_after_failures;
		cfgWebhookEventRetentionDays = agentConfig.webhook_event_retention_days;
		cfgWebhookDeliveryRetentionDays = agentConfig.webhook_delivery_retention_days;
	}

	async function saveAgentConfig() {
		if (agentBusy || isSavingPassword || !agentConfig || !configDirty) return;
		isSavingConfig = true;
		configError = '';
		configSuccess = '';
		try {
			const payload: Record<string, string | boolean | number> = {
				environment: cfgEnvironment,
				github_deploy_enabled: cfgGithubDeployEnabled,
				log_dir: cfgLogDir,
				nssm_path: cfgNssmPath,
				db_path: cfgDBPath,
				api_port: cfgAPIPort,
				api_base_url: cfgAPIBaseURL,
				web_assets_path: cfgWebAssetsPath.trim(),
				watcher_repo_url: cfgWatcherRepoURL,
				watcher_service_name: cfgWatcherServiceName,
				webhook_default_url: cfgWebhookDefaultURL,
				webhook_timeout_sec: cfgWebhookTimeoutSec,
				webhook_retry_schedule_sec: cfgWebhookRetryScheduleSec,
				webhook_auto_pause_enabled: cfgWebhookAutoPauseEnabled,
				webhook_auto_pause_after_failures: cfgWebhookAutoPauseAfterFailures,
				webhook_event_retention_days: cfgWebhookEventRetentionDays,
				webhook_delivery_retention_days: cfgWebhookDeliveryRetentionDays
			};

			if (clearGitHubToken) {
				payload.github_token = '';
			} else if (githubTokenInput.trim()) {
				payload.github_token = githubTokenInput.trim();
			}
			if (clearWebhookDefaultSigningSecret) {
				payload.webhook_default_signing_secret = '';
			} else if (webhookDefaultSigningSecretInput.trim()) {
				payload.webhook_default_signing_secret = webhookDefaultSigningSecretInput.trim();
			}

			const res = await api.updateSelfConfig(payload);
			agentConfig = res.config;
			syncConfigForm();
			githubTokenInput = '';
			clearGitHubToken = false;
			webhookDefaultSigningSecretInput = '';
			clearWebhookDefaultSigningSecret = false;
			savedDraft = draftKey();
			configSuccess = res.message;
			saveNotes = res.notes || [];
		} catch (e) {
			configError = e instanceof Error ? e.message : 'Failed to save config';
		} finally {
			isSavingConfig = false;
		}
	}

	async function savePassword() {
		if (isSavingPassword || agentBusy) return;
		isSavingPassword = true;
		passwordError = '';
		passwordSuccess = '';
		try {
			if (!newPassword.trim()) {
				throw new Error('New password is required');
			}
			if (newPassword !== confirmPassword) {
				throw new Error('New password confirmation does not match');
			}
			const res = await api.updateAuthPassword(currentPassword, newPassword);
			auth.setPassword(newPassword);
			authStatus = { authenticated: true, using_default_password: res.using_default_password };
			currentPassword = '';
			newPassword = '';
			confirmPassword = '';
			passwordSuccess = res.message;
		} catch (e) {
			passwordError = e instanceof Error ? e.message : 'Failed to update password';
		} finally {
			isSavingPassword = false;
		}
	}

	async function restartWatcherService() {
		if (agentBusy || isSavingPassword || configDirty) return;
		isRestarting = true;
		maintenanceError = '';
		maintenanceSuccess = '';
		try {
			const res = await api.selfRestart();
			maintenanceSuccess = `${res.message} (${res.service_name})`;
			showRestartDialog = false;
		} catch (e) {
			maintenanceError = e instanceof Error ? e.message : 'Failed to restart watcher service';
		} finally {
			isRestarting = false;
		}
	}

	async function checkForUpdates() {
		isChecking = true;
		maintenanceError = '';
		try {
			const info = await lookupSelfUpdate({ force: true });
			updateInfo = info;
			if (info && !info.update_available) {
				clearSelfUpdateDismissal();
			}
		} catch (e) {
			maintenanceError = e instanceof Error ? e.message : 'Check failed';
		} finally {
			isChecking = false;
		}
	}

	async function performUpdate() {
		if (agentBusy || isSavingPassword || configDirty) return;
		isUpdating = true;
		maintenanceError = '';
		try {
			await api.selfUpdate();
			clearSelfUpdateCache();
			clearSelfUpdateDismissal();
			showUpdateDialog = false;
			setTimeout(() => {
				window.location.reload();
			}, 3000);
		} catch (e) {
			maintenanceError = e instanceof Error ? e.message : 'Update failed';
			isUpdating = false;
		}
	}

	async function generateUninstall() {
		try {
			const res = await api.selfUninstall();
			uninstallScript = res.script;
		} catch (e) {
			maintenanceError = e instanceof Error ? e.message : 'Uninstall generation failed';
		}
	}

	async function copyUninstallScript() {
		if (!uninstallScript) return;
		try {
			await navigator.clipboard.writeText(uninstallScript);
		} catch (e) {
			maintenanceError = 'Failed to copy script';
		}
	}
</script>

<svelte:head>
	<title>Settings | Watcher</title>
</svelte:head>

<div class="space-y-6">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">Global settings</h1>
		<p class="mt-1 flex items-center gap-1.5 text-sm text-muted-foreground">
			<Info class="h-4 w-4" /> Defaults, access and maintenance for this agent
		</p>
	</div>

	{#if loading}
		<RequestLoading label="Loading global settings…" />
	{:else if loadError}
		<RequestError message={loadError} onRetry={loadSettings} />
	{:else}
		<Tabs.Root bind:value={activeSection}>
			<Tabs.List>
				<Tabs.Trigger value="deployment">Deployment</Tabs.Trigger>
				<Tabs.Trigger value="webhooks">Webhooks</Tabs.Trigger>
				<Tabs.Trigger value="installation">Installation</Tabs.Trigger>
				<Tabs.Trigger value="access">Access</Tabs.Trigger>
				<Tabs.Trigger value="maintenance">Maintenance</Tabs.Trigger>
			</Tabs.List>
			<Tabs.Content value="deployment"
				><fieldset disabled={agentBusy || isSavingPassword}>
					<Card.Root>
						<Card.Header
							><Card.Title>Deployment defaults</Card.Title><Card.Description
								>Inherited by watchers unless overridden in their settings.</Card.Description
							></Card.Header
						>
						<Card.Content
							><div class="grid gap-4 md:grid-cols-2">
								<div class="space-y-2">
									<label class="text-sm text-muted-foreground" for="cfg-environment"
										>Environment</label
									>
									<Input
										id="cfg-environment"
										bind:value={cfgEnvironment}
										aria-describedby="cfg-environment-help"
									/>
									<p id="cfg-environment-help" class="text-xs text-muted-foreground">
										Default environment name reported to GitHub. Individual watchers can override
										it.
									</p>
								</div>
								<div class="flex flex-wrap items-center gap-2 py-2">
									<Checkbox
										id="cfg-github-deploy-enabled"
										bind:checked={cfgGithubDeployEnabled}
										aria-describedby="cfg-github-deploy-enabled-help"
									/>

									<label
										class="text-sm text-muted-foreground select-none"
										for="cfg-github-deploy-enabled"
									>
										Enable GitHub Deployment API
									</label>
									<p
										id="cfg-github-deploy-enabled-help"
										class="w-full text-xs text-muted-foreground"
									>
										Report deployment progress to GitHub. Watcher can still deploy when reporting is
										disabled.
									</p>
								</div>
								<div class="space-y-2 md:col-span-2">
									<label class="text-sm text-muted-foreground" for="cfg-github-token"
										>GitHub token</label
									>
									<Input
										id="cfg-github-token"
										type="password"
										placeholder={agentConfig?.github_token_masked || 'not set'}
										bind:value={githubTokenInput}
										disabled={clearGitHubToken}
										autocomplete="off"
									/>
									<div class="mt-2 flex items-center gap-2">
										<Checkbox id="clear-github-token" bind:checked={clearGitHubToken} />
										<label
											class="text-xs text-muted-foreground select-none"
											for="clear-github-token"
										>
											Clear existing GitHub token
										</label>
									</div>
									<p class="text-xs text-muted-foreground">
										Leave blank to keep the current token.
									</p>
									<details class="text-xs text-muted-foreground">
										<summary class="cursor-pointer">Token permissions</summary>
										<p class="mt-2">
											Private releases need Contents: Read. Deployment reporting also needs
											Deployments: Read and write. Include the target repositories and any required
											organization authorization.
										</p>
									</details>
								</div>
								<div class="space-y-2 md:col-span-2">
									<label class="text-sm text-muted-foreground" for="cfg-api-base-url"
										>API Base URL</label
									>
									<Input
										id="cfg-api-base-url"
										bind:value={cfgAPIBaseURL}
										placeholder="http://192.168.1.100:8080"
										aria-describedby="cfg-api-base-url-help"
									/>
									<p id="cfg-api-base-url-help" class="text-xs text-muted-foreground">
										Dashboard URL reachable by your team. Used for deployment log links in GitHub.
									</p>
								</div>
							</div></Card.Content
						>
					</Card.Root>
				</fieldset></Tabs.Content
			>
			<Tabs.Content value="webhooks"
				><fieldset disabled={agentBusy || isSavingPassword}>
					<Card.Root class="bg-card">
						<Card.Header>
							<Card.Title>Webhook Defaults</Card.Title>
							<Card.Description>
								Watchers inherit these values unless they supply their own URL or signing secret.
							</Card.Description><Button.Root
								size="sm"
								variant="outline"
								href={resolve('/docs/webhooks')}>Integration guide</Button.Root
							>
						</Card.Header>
						<Card.Content class="space-y-4">
							{#if agentConfig}
								<div class="grid gap-4 md:grid-cols-2">
									<div class="space-y-2 md:col-span-2">
										<label class="text-sm text-muted-foreground" for="cfg-webhook-default-url"
											>Default Webhook URL</label
										>
										<Input
											id="cfg-webhook-default-url"
											bind:value={cfgWebhookDefaultURL}
											placeholder="https://example.com/hooks/watcher"
										/>
										<p class="text-xs text-muted-foreground">
											Watchers can override this, but leaving watcher URL blank will inherit this
											default.
										</p>
									</div>
									<div class="space-y-2 md:col-span-2">
										<label
											class="text-sm text-muted-foreground"
											for="cfg-webhook-default-signing-secret"
										>
											Signing secret
										</label>
										<Input
											id="cfg-webhook-default-signing-secret"
											type="password"
											placeholder={agentConfig.webhook_default_signing_secret_masked || 'not set'}
											bind:value={webhookDefaultSigningSecretInput}
											disabled={clearWebhookDefaultSigningSecret}
											autocomplete="off"
										/>
										<div class="mt-2 flex items-center gap-2">
											<Checkbox
												id="clear-webhook-default-signing-secret"
												bind:checked={clearWebhookDefaultSigningSecret}
											/>
											<label
												class="text-xs text-muted-foreground select-none"
												for="clear-webhook-default-signing-secret"
											>
												Clear existing default webhook signing secret
											</label>
										</div>
										<p class="text-xs text-muted-foreground">
											Use a Standard Webhooks HMAC signing secret. Raw base64 secret material or the
											conventional <code>whsec_...</code> form both work.
										</p>
									</div>
									<details class="md:col-span-2">
										<summary class="cursor-pointer text-sm font-medium"
											>Delivery, retries and retention</summary
										>
										<div class="mt-4 grid gap-4 md:grid-cols-2">
											<div class="space-y-2">
												<label class="text-sm text-muted-foreground" for="cfg-webhook-timeout-sec"
													>Webhook Timeout (s)</label
												>
												<Input
													id="cfg-webhook-timeout-sec"
													type="number"
													min="1"
													bind:value={cfgWebhookTimeoutSec}
													aria-describedby="cfg-webhook-timeout-sec-help"
												/>
												<p id="cfg-webhook-timeout-sec-help" class="text-xs text-muted-foreground">
													Maximum seconds to wait for the receiver to respond to each delivery.
												</p>
											</div>
											<div class="space-y-2">
												<label
													class="text-sm text-muted-foreground"
													for="cfg-webhook-retry-schedule-sec"
												>
													Webhook Retry Schedule (seconds CSV)
												</label>
												<Input
													id="cfg-webhook-retry-schedule-sec"
													bind:value={cfgWebhookRetryScheduleSec}
													placeholder="0,10,60,300"
													aria-describedby="cfg-webhook-retry-schedule-sec-help"
												/>
												<p
													id="cfg-webhook-retry-schedule-sec-help"
													class="text-xs text-muted-foreground"
												>
													Comma-separated retry delays in seconds, for example 0,10,60,300.
												</p>
											</div>
											<div class="flex items-center gap-2 py-2">
												<Checkbox
													id="cfg-webhook-auto-pause-enabled"
													bind:checked={cfgWebhookAutoPauseEnabled}
												/>
												<label
													class="text-sm text-muted-foreground select-none"
													for="cfg-webhook-auto-pause-enabled"
												>
													Enable webhook auto-pause
												</label>
											</div>
											<div class="space-y-2">
												<label
													class="text-sm text-muted-foreground"
													for="cfg-webhook-auto-pause-after-failures"
												>
													Auto-pause after failures
												</label>
												<Input
													id="cfg-webhook-auto-pause-after-failures"
													type="number"
													min="1"
													bind:value={cfgWebhookAutoPauseAfterFailures}
													aria-describedby="cfg-webhook-auto-pause-after-failures-help"
												/>
												<p
													id="cfg-webhook-auto-pause-after-failures-help"
													class="text-xs text-muted-foreground"
												>
													Failed deliveries allowed before automatic delivery pauses. Used when
													auto-pause is enabled.
												</p>
											</div>
											<div class="space-y-2">
												<label
													class="text-sm text-muted-foreground"
													for="cfg-webhook-event-retention-days"
												>
													Webhook Event Retention (days)
												</label>
												<Input
													id="cfg-webhook-event-retention-days"
													type="number"
													min="1"
													bind:value={cfgWebhookEventRetentionDays}
													aria-describedby="cfg-webhook-event-retention-days-help"
												/>
												<p
													id="cfg-webhook-event-retention-days-help"
													class="text-xs text-muted-foreground"
												>
													Days to keep recorded webhook events before cleanup.
												</p>
											</div>
											<div class="space-y-2">
												<label
													class="text-sm text-muted-foreground"
													for="cfg-webhook-delivery-retention-days"
												>
													Webhook Delivery Retention (days)
												</label>
												<Input
													id="cfg-webhook-delivery-retention-days"
													type="number"
													min="1"
													bind:value={cfgWebhookDeliveryRetentionDays}
													aria-describedby="cfg-webhook-delivery-retention-days-help"
												/>
												<p
													id="cfg-webhook-delivery-retention-days-help"
													class="text-xs text-muted-foreground"
												>
													Days to keep delivery attempts and their responses before cleanup.
												</p>
											</div>
										</div>
									</details>
								</div>
							{:else}
								<div class="h-20 animate-pulse rounded bg-muted/50"></div>
							{/if}
						</Card.Content>
					</Card.Root>
				</fieldset></Tabs.Content
			>
			<Tabs.Content value="installation"
				><fieldset disabled={agentBusy || isSavingPassword}>
					<Card.Root>
						<Card.Header
							><Card.Title>Agent installation</Card.Title><Card.Description
								>Network, storage and Windows service settings.</Card.Description
							></Card.Header
						>
						<Card.Content
							><div class="grid gap-4 md:grid-cols-2">
								<div class="space-y-2">
									<label class="text-sm text-muted-foreground" for="cfg-api-port">API Port</label>
									<Input
										id="cfg-api-port"
										bind:value={cfgAPIPort}
										aria-describedby="cfg-api-port-help"
									/>
									<p id="cfg-api-port-help" class="text-xs text-muted-foreground">
										Port used by the dashboard and API. Requires an agent restart; reconnect using
										the new port.
									</p>
								</div>
								<div class="space-y-2 md:col-span-2">
									<label class="text-sm text-muted-foreground" for="cfg-web-assets-path"
										>Web Assets / Base Path</label
									>
									<Input
										id="cfg-web-assets-path"
										bind:value={cfgWebAssetsPath}
										placeholder="/watcher"
									/>
									<p class="text-xs text-muted-foreground">
										Subpath prefix when hosting behind a reverse proxy (e.g. <code>/watcher</code>
										for
										<code>https://domain.co.id/watcher</code>). Leave empty if serving from root.
									</p>
								</div>
								<div class="space-y-2 md:col-span-2">
									<label class="text-sm text-muted-foreground" for="cfg-watcher-repo-url"
										>Watcher Repo URL</label
									>
									<Input
										id="cfg-watcher-repo-url"
										bind:value={cfgWatcherRepoURL}
										aria-describedby="cfg-watcher-repo-url-help"
									/>
									<p id="cfg-watcher-repo-url-help" class="text-xs text-muted-foreground">
										GitHub repository used to check for updates to the Watcher agent itself.
									</p>
								</div>
								<div class="space-y-2 md:col-span-2">
									<label class="text-sm text-muted-foreground" for="cfg-watcher-service-name"
										>Watcher Service Name</label
									>
									<Input
										id="cfg-watcher-service-name"
										bind:value={cfgWatcherServiceName}
										aria-describedby="cfg-watcher-service-name-help"
									/>
									<p id="cfg-watcher-service-name-help" class="text-xs text-muted-foreground">
										Installed Windows service name used when restarting or updating this agent.
									</p>
								</div>
								<div class="space-y-2 md:col-span-2">
									<label class="text-sm text-muted-foreground" for="cfg-nssm-path">NSSM Path</label>
									<Input
										id="cfg-nssm-path"
										bind:value={cfgNssmPath}
										aria-describedby="cfg-nssm-path-help"
									/>
									<p id="cfg-nssm-path-help" class="text-xs text-muted-foreground">
										Full path to nssm.exe on the machine running Watcher.
									</p>
								</div>
								<div class="space-y-2 md:col-span-2">
									<label class="text-sm text-muted-foreground" for="cfg-log-dir"
										>Log Directory</label
									>
									<Input
										id="cfg-log-dir"
										bind:value={cfgLogDir}
										aria-describedby="cfg-log-dir-help"
									/>
									<p id="cfg-log-dir-help" class="text-xs text-muted-foreground">
										Directory for agent log files on the Watcher machine. Restart the agent after
										changing it.
									</p>
								</div>
								<div class="space-y-2 md:col-span-2">
									<label class="text-sm text-muted-foreground" for="cfg-db-path"
										>Database Path</label
									>
									<Input
										id="cfg-db-path"
										bind:value={cfgDBPath}
										aria-describedby="cfg-db-path-help"
									/>
									<p id="cfg-db-path-help" class="text-xs text-muted-foreground">
										Path to the SQLite database. Changing it selects a different database; it does
										not move existing data. Requires a restart.
									</p>
								</div>
							</div></Card.Content
						>
					</Card.Root>
				</fieldset></Tabs.Content
			>
			<Tabs.Content value="access">
				<Card.Root class="bg-card">
					<Card.Header>
						<Card.Title class="flex items-center gap-2">
							<LockKeyhole class="h-4 w-4" /> Dashboard Password
						</Card.Title>
						<Card.Description>Change the password used for the dashboard and API.</Card.Description>
					</Card.Header>
					<Card.Content class="space-y-4">
						<RequestError message={passwordError} />
						{#if passwordSuccess}<p role="status" class="text-sm text-green-400">
								{passwordSuccess}
							</p>{/if}
						<form
							onsubmit={(event) => {
								event.preventDefault();
								void savePassword();
							}}
						>
							<fieldset disabled={isSavingPassword || agentBusy} class="space-y-4">
								{#if authStatus?.using_default_password}
									<div
										class="rounded-md border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-300"
									>
										The dashboard is still using the default password. Change it before exposing
										Watcher beyond a trusted machine.
									</div>
								{/if}

								<div class="grid gap-4 md:grid-cols-3">
									<div class="space-y-2">
										<label class="text-sm text-muted-foreground" for="current-password"
											>Current Password</label
										>
										<Input
											id="current-password"
											type="password"
											bind:value={currentPassword}
											autocomplete="current-password"
										/>
									</div>
									<div class="space-y-2">
										<label class="text-sm text-muted-foreground" for="new-password"
											>New Password</label
										>
										<Input
											id="new-password"
											type="password"
											bind:value={newPassword}
											autocomplete="new-password"
										/>
									</div>
									<div class="space-y-2">
										<label class="text-sm text-muted-foreground" for="confirm-password"
											>Confirm Password</label
										>
										<Input
											id="confirm-password"
											type="password"
											bind:value={confirmPassword}
											autocomplete="new-password"
										/>
									</div>
								</div>

								<Button.Root
									type="submit"
									loading={isSavingPassword}
									disabled={isSavingPassword || agentBusy}
								>
									{isSavingPassword ? 'Saving...' : 'Update Password'}
								</Button.Root>
							</fieldset>
						</form></Card.Content
					>
				</Card.Root></Tabs.Content
			>
			<Tabs.Content value="maintenance"
				><div class="space-y-6">
					{#if configDirty}<div
							class="section-toolbar rounded-lg border border-amber-500/30 bg-amber-500/5 p-3"
						>
							<p class="text-sm text-amber-300">
								Save or discard your configuration changes before restarting or updating.
							</p>
							<Button.Root
								size="sm"
								variant="outline"
								onclick={() => {
									activeSection = 'deployment';
								}}>Review changes</Button.Root
							>
						</div>{/if}
					{#if !showRestartDialog && !showUpdateDialog}<RequestError
							message={maintenanceError}
						/>{/if}
					{#if maintenanceSuccess}<p role="status" class="text-sm text-green-400">
							{maintenanceSuccess}
						</p>{/if}
					<Card.Root class="bg-card">
						<Card.Header>
							<Card.Title>Watcher Version</Card.Title>
							<Card.Description>Current version and system info</Card.Description>
						</Card.Header>
						<Card.Content class="space-y-4">
							{#if versionInfo}
								<div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
									<div class="rounded border border-border bg-muted/50 p-3">
										<div class="mb-1 text-xs text-muted-foreground">Version</div>
										<div class="font-mono text-sm">{versionInfo.version}</div>
									</div>
									<div class="rounded border border-border bg-muted/50 p-3">
										<div class="mb-1 text-xs text-muted-foreground">Go Runtime</div>
										<div class="font-mono text-sm">{versionInfo.go_version}</div>
									</div>
									<div class="rounded border border-border bg-muted/50 p-3">
										<div class="mb-1 text-xs text-muted-foreground">Platform</div>
										<div class="font-mono text-sm">{versionInfo.os} / {versionInfo.arch}</div>
									</div>
									<div class="rounded border border-border bg-muted/50 p-3 lg:col-span-4">
										<div class="mb-1 text-xs text-muted-foreground">Executable Path</div>
										<div class="font-mono text-xs break-all">{versionInfo.executable}</div>
									</div>
								</div>
							{:else if !loadError}
								<div class="h-24 animate-pulse rounded bg-muted/50"></div>
							{/if}

							<div class="mt-4 border-t border-border pt-4">
								<Button.Root
									onclick={checkForUpdates}
									disabled={isChecking || agentBusy}
									variant="outline"
								>
									<RotateCcw class={`mr-2 h-4 w-4 ${isChecking ? 'animate-spin' : ''}`} />
									{isChecking ? 'Checking...' : 'Check for Updates'}
								</Button.Root>
							</div>

							{#if updateInfo}
								<div
									class="mt-4 rounded border p-4 {updateInfo.update_available
										? 'border-blue-500/50 bg-blue-500/5 text-blue-50'
										: 'border-emerald-500/30 bg-emerald-500/5'}"
								>
									{#if updateInfo.update_available}
										<div class="section-toolbar">
											<div>
												<h4 class="mb-1 flex items-center gap-2 font-medium text-blue-400">
													<Download class="h-4 w-4" /> Update Available
												</h4>
												<p class="text-sm">
													A new version of Watcher <strong>{updateInfo.latest_version}</strong> is available.
												</p>
												<p class="mt-1 text-xs text-muted-foreground">
													Currently running: {updateInfo.current_version}
												</p>
												{#if updateInfo.published_at}
													<p class="mt-1 text-xs text-muted-foreground">
														Published: {new Date(updateInfo.published_at).toLocaleString()}
													</p>
												{/if}
											</div>
											<Button.Root
												onclick={() => {
													maintenanceError = '';
													showUpdateDialog = true;
												}}
												disabled={agentBusy || configDirty || isSavingPassword}
												class="bg-blue-600 text-white hover:bg-blue-700"
											>
												{isUpdating ? 'Updating...' : 'Update & Restart Watcher'}
											</Button.Root>
										</div>
									{:else}
										<div class="flex items-center gap-2 text-sm font-medium text-emerald-500">
											<CheckCircle2 class="h-4 w-4" /> Watcher is up to date (running the latest version:
											{updateInfo.latest_version}).
										</div>
									{/if}
								</div>
							{/if}
						</Card.Content>
					</Card.Root>
					<Card.Root
						><Card.Header
							><Card.Title>Restart agent</Card.Title><Card.Description
								>Temporarily disconnects the dashboard. Save pending configuration changes first.</Card.Description
							></Card.Header
						><Card.Content
							><Button.Root
								variant="outline"
								disabled={agentBusy || isSavingPassword || configDirty}
								onclick={() => {
									maintenanceError = '';
									showRestartDialog = true;
								}}>Restart Watcher</Button.Root
							></Card.Content
						></Card.Root
					>
					<Card.Root class="bg-card">
						<Card.Header>
							<Card.Title class="flex items-center gap-2 text-red-400">
								<AlertTriangle class="h-4 w-4" /> Uninstall Watcher
							</Card.Title>
							<Card.Description
								>Generate a PowerShell script to safely remove the Watcher agent, services, and
								registry keys.</Card.Description
							>
						</Card.Header>
						<Card.Content>
							<details>
								<summary class="cursor-pointer text-sm font-medium text-red-400"
									>Show uninstall tools</summary
								>
								<div class="mt-4">
									<Button.Root
										variant="destructive"
										onclick={generateUninstall}
										disabled={agentBusy}
										class="mb-4"
									>
										Generate Uninstall Script
									</Button.Root>

									{#if uninstallScript}
										<div class="relative rounded border border-red-500/30 bg-[#0a0a0a] p-4">
											<Button.Root
												variant="secondary"
												size="icon"
												class="absolute top-2 right-2 h-8 w-8 bg-muted text-xs hover:bg-muted/80"
												onclick={copyUninstallScript}
												aria-label="Copy uninstall script"
											>
												<Copy class="h-3.5 w-3.5" />
											</Button.Root>
											<pre
												class="overflow-x-auto p-2 font-mono text-xs leading-relaxed text-red-300"><code
													>{uninstallScript}</code
												></pre>
										</div>
										<p class="mt-2 text-xs text-muted-foreground">
											Save this script as <code>uninstall-watcher.ps1</code> and run it from an elevated
											PowerShell window to completely remove watcher.
										</p>
									{/if}
								</div>
							</details></Card.Content
						>
					</Card.Root>
				</div></Tabs.Content
			>
		</Tabs.Root>
		{#if ['deployment', 'webhooks', 'installation'].includes(activeSection)}
			<div
				class="sticky bottom-3 z-10 space-y-3 rounded-lg border border-border bg-background p-4 shadow-lg"
			>
				<RequestError message={configError} />
				{#if configSuccess && !configDirty}<p role="status" class="text-sm text-green-400">
						{configSuccess}
					</p>{/if}
				{#if saveNotes.length && !configDirty}<ul
						class="list-inside list-disc text-xs text-muted-foreground"
					>
						{#each saveNotes as note (note)}<li>{note}</li>{/each}
					</ul>{/if}
				<div class="section-toolbar">
					<div class="min-w-0">
						<p class="text-sm font-medium">
							{configDirty ? 'Unsaved changes' : 'All changes saved'}
						</p>
						<p class="text-xs text-muted-foreground">
							Saves deployment, webhook and installation settings together.
						</p>
					</div>
					<div class="flex flex-wrap gap-2">
						<Button.Root
							variant="outline"
							disabled={!configDirty || agentBusy || isSavingPassword}
							onclick={resetConfig}>Discard changes</Button.Root
						>
						<Button.Root
							loading={isSavingConfig}
							disabled={!configDirty || agentBusy || isSavingPassword}
							onclick={saveAgentConfig}>Save changes</Button.Root
						>
					</div>
				</div>
				<details class="text-xs text-muted-foreground">
					<summary class="cursor-pointer">Where settings are saved</summary>
					<p class="mt-2 break-all">{agentConfig?.env_path}</p>
					<p class="mt-1">
						Watcher loops reload after saving. Port, database, logging and dashboard path changes
						require an agent restart.
					</p>
				</details>
			</div>
		{/if}
	{/if}
</div>

<Dialog.Root
	bind:open={showRestartDialog}
	onOpenChange={(open) => {
		if (isRestarting && !open) showRestartDialog = true;
	}}
>
	<Dialog.Content class="sm:max-w-115" showCloseButton={!isRestarting}>
		<Dialog.Header>
			<Dialog.Title>Restart Watcher Service</Dialog.Title>
			<Dialog.Description>
				Restart watcher service now? This may temporarily disconnect the dashboard.
			</Dialog.Description>
		</Dialog.Header>
		<RequestError message={maintenanceError} />
		<Dialog.Footer>
			<Button.Root
				variant="outline"
				type="button"
				disabled={isRestarting}
				onclick={() => (showRestartDialog = false)}
			>
				Cancel
			</Button.Root>
			<Button.Root
				type="button"
				disabled={isRestarting}
				loading={isRestarting}
				onclick={restartWatcherService}>Restart</Button.Root
			>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<Dialog.Root
	bind:open={showUpdateDialog}
	onOpenChange={(open) => {
		if (isUpdating && !open) showUpdateDialog = true;
	}}
>
	<Dialog.Content class="sm:max-w-115" showCloseButton={!isUpdating}>
		<Dialog.Header>
			<Dialog.Title>Update Watcher</Dialog.Title>
			<Dialog.Description>
				Update Watcher now? The service will be restarted automatically.
			</Dialog.Description>
		</Dialog.Header>
		<RequestError message={maintenanceError} />
		<Dialog.Footer>
			<Button.Root
				variant="outline"
				type="button"
				onclick={() => (showUpdateDialog = false)}
				disabled={isUpdating}
			>
				Cancel
			</Button.Root>
			<Button.Root
				type="button"
				class="bg-blue-600 text-white hover:bg-blue-700"
				onclick={performUpdate}
				disabled={isUpdating}
			>
				{isUpdating ? 'Updating...' : 'Update & Restart'}
			</Button.Root>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
