<script lang="ts">
	import { onMount } from 'svelte';
	import {
		api,
		type SelfVersionResponse,
		type SelfUpdateCheckResponse,
		type SelfConfigResponse
	} from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import { Download, Info, RotateCcw, AlertTriangle, CheckCircle2, Copy } from '@lucide/svelte';

	let versionInfo = $state<SelfVersionResponse | null>(null);
	let updateInfo = $state<SelfUpdateCheckResponse | null>(null);
	let agentConfig = $state<SelfConfigResponse | null>(null);
	let error = $state('');
	let success = $state('');
	
	let isChecking = $state(false);
	let isUpdating = $state(false);
	let isSavingConfig = $state(false);
	let uninstallScript = $state('');
	let githubTokenInput = $state('');
	let clearGitHubToken = $state(false);

	let cfgEnvironment = $state('');
	let cfgGithubDeployEnabled = $state(true);
	let cfgLogDir = $state('');
	let cfgNssmPath = $state('');
	let cfgDBPath = $state('');
	let cfgAPIPort = $state('');
	let cfgAPIBaseURL = $state('');
	let cfgWatcherRepoURL = $state('');
	let cfgWatcherServiceName = $state('');
	let showRestartDialog = $state(false);
	let showUpdateDialog = $state(false);

	onMount(() => {
		const init = async () => {
			try {
				[versionInfo, agentConfig] = await Promise.all([api.selfVersion(), api.selfConfig()]);
				syncConfigForm();
			} catch (e) {
				error = e instanceof Error ? e.message : 'Failed to load version info';
			}
		};
		init();
	});

	function syncConfigForm() {
		if (!agentConfig) return;
		cfgEnvironment = agentConfig.environment;
		cfgGithubDeployEnabled = agentConfig.github_deploy_enabled;
		cfgLogDir = agentConfig.log_dir;
		cfgNssmPath = agentConfig.nssm_path;
		cfgDBPath = agentConfig.db_path;
		cfgAPIPort = agentConfig.api_port;
		cfgAPIBaseURL = agentConfig.api_base_url;
		cfgWatcherRepoURL = agentConfig.watcher_repo_url;
		cfgWatcherServiceName = agentConfig.watcher_service_name;
	}

	async function saveAgentConfig() {
		isSavingConfig = true;
		error = '';
		success = '';
		try {
			const payload: Record<string, string | boolean> = {
				environment: cfgEnvironment,
				github_deploy_enabled: cfgGithubDeployEnabled,
				log_dir: cfgLogDir,
				nssm_path: cfgNssmPath,
				db_path: cfgDBPath,
				api_port: cfgAPIPort,
				api_base_url: cfgAPIBaseURL,
				watcher_repo_url: cfgWatcherRepoURL,
				watcher_service_name: cfgWatcherServiceName
			};

			if (clearGitHubToken) {
				payload.github_token = '';
			} else if (githubTokenInput.trim()) {
				payload.github_token = githubTokenInput.trim();
			}

			const res = await api.updateSelfConfig(payload);
			agentConfig = res.config;
			syncConfigForm();
			githubTokenInput = '';
			clearGitHubToken = false;
			success = res.message;
			setTimeout(() => (success = ''), 4000);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save config';
		} finally {
			isSavingConfig = false;
		}
	}

	async function restartWatcherService() {
		error = '';
		success = '';
		try {
			const res = await api.selfRestart();
			success = `${res.message} (${res.service_name})`;
			showRestartDialog = false;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to restart watcher service';
		}
	}

	async function checkForUpdates() {
		isChecking = true;
		error = '';
		try {
			updateInfo = await api.selfUpdateCheck();
			if (!updateInfo.update_available) {
				error = ''; // Watcher is up to date
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Check failed';
		} finally {
			isChecking = false;
		}
	}

	async function performUpdate() {
		isUpdating = true;
		error = '';
		try {
			await api.selfUpdate();
			showUpdateDialog = false;
			setTimeout(() => {
				window.location.reload();
			}, 3000);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Update failed';
			isUpdating = false;
		}
	}

	async function generateUninstall() {
		try {
			const res = await api.selfUninstall();
			uninstallScript = res.script;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Uninstall generation failed';
		}
	}

	async function copyUninstallScript() {
		if (!uninstallScript) return;
		try {
			await navigator.clipboard.writeText(uninstallScript);
		} catch (e) {
			error = 'Failed to copy script';
		}
	}
</script>

<svelte:head>
	<title>Settings | Watcher</title>
</svelte:head>

<div class="space-y-6">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">System Settings</h1>
		<p class="text-muted-foreground text-sm flex items-center gap-1.5 mt-1">
			<Info class="w-4 h-4" /> Manage the Watcher agent installation and updates
		</p>
	</div>

	{#if success}
		<div class="rounded-lg border border-green-500/30 bg-green-500/10 p-4 text-sm text-green-400">
			{success}
		</div>
	{/if}

	{#if error}
		<div class="rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400">
			{error}
		</div>
	{/if}

	<Card.Root class="bg-card">
		<Card.Header>
			<Card.Title>Agent Configuration (.env)</Card.Title>
			<Card.Description>Edit runtime configuration values from the dashboard.</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-4">
			{#if agentConfig}
				<div class="grid gap-4 md:grid-cols-2">
					<div class="space-y-2">
						<label class="text-sm text-muted-foreground" for="cfg-environment">Environment</label>
						<Input id="cfg-environment" bind:value={cfgEnvironment} />
					</div>
					<div class="space-y-2">
						<label class="text-sm text-muted-foreground inline-flex items-center gap-2" for="cfg-github-deploy-enabled">
							<input id="cfg-github-deploy-enabled" type="checkbox" bind:checked={cfgGithubDeployEnabled} />
							Enable GitHub Deployment API
						</label>
					</div>
					<div class="space-y-2">
						<label class="text-sm text-muted-foreground" for="cfg-api-port">API Port</label>
						<Input id="cfg-api-port" bind:value={cfgAPIPort} />
					</div>
					<div class="space-y-2 md:col-span-2">
						<label class="text-sm text-muted-foreground" for="cfg-github-token">GitHub Token (leave blank to keep current)</label>
						<Input id="cfg-github-token" type="password" placeholder={agentConfig.github_token_masked || 'not set'} bind:value={githubTokenInput} />
						<label class="text-xs text-muted-foreground inline-flex items-center gap-2">
							<input type="checkbox" bind:checked={clearGitHubToken} />
							Clear existing GitHub token
						</label>
						<div class="rounded-md border border-border bg-muted/30 p-3 text-xs text-muted-foreground space-y-1">
							<p class="font-medium text-foreground/90">GitHub token requirements</p>
							<p>Public repos: token optional. Private repos: token required.</p>
							<p>Fine-grained PAT minimum: <code>Contents: Read</code>.</p>
							<p>If GitHub Deployment API is enabled: also grant <code>Deployments: Read and write</code>.</p>
							<p class="pt-1 font-medium text-foreground/90">Org private repo checklist</p>
							<p>Token must be authorized for org SSO/SAML and allowed by org PAT policy.</p>
							<p>Token owner must already have access to the target private repository.</p>
						</div>
					</div>
					<div class="space-y-2 md:col-span-2">
						<label class="text-sm text-muted-foreground" for="cfg-api-base-url">API Base URL</label>
						<Input id="cfg-api-base-url" bind:value={cfgAPIBaseURL} placeholder="http://192.168.1.100:8080" />
					</div>
					<div class="space-y-2 md:col-span-2">
						<label class="text-sm text-muted-foreground" for="cfg-watcher-repo-url">Watcher Repo URL</label>
						<Input id="cfg-watcher-repo-url" bind:value={cfgWatcherRepoURL} />
					</div>
					<div class="space-y-2 md:col-span-2">
						<label class="text-sm text-muted-foreground" for="cfg-watcher-service-name">Watcher Service Name</label>
						<Input id="cfg-watcher-service-name" bind:value={cfgWatcherServiceName} />
					</div>
					<div class="space-y-2 md:col-span-2">
						<label class="text-sm text-muted-foreground" for="cfg-nssm-path">NSSM Path</label>
						<Input id="cfg-nssm-path" bind:value={cfgNssmPath} />
					</div>
					<div class="space-y-2 md:col-span-2">
						<label class="text-sm text-muted-foreground" for="cfg-log-dir">Log Directory</label>
						<Input id="cfg-log-dir" bind:value={cfgLogDir} />
					</div>
					<div class="space-y-2 md:col-span-2">
						<label class="text-sm text-muted-foreground" for="cfg-db-path">Database Path</label>
						<Input id="cfg-db-path" bind:value={cfgDBPath} />
					</div>
				</div>

				<div class="rounded-md border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
					Changes are written to <code>{agentConfig.env_path}</code>. Watcher loops reload automatically, but changing API port or DB path requires service restart.
				</div>

				<div class="flex gap-2">
					<Button.Root onclick={saveAgentConfig} disabled={isSavingConfig}>
						{isSavingConfig ? 'Saving...' : 'Save Agent Config'}
					</Button.Root>
					<Button.Root variant="outline" onclick={() => (showRestartDialog = true)}>
						Restart Watcher Service
					</Button.Root>
				</div>
			{:else}
				<div class="animate-pulse bg-muted/50 h-20 rounded"></div>
			{/if}
		</Card.Content>
	</Card.Root>

	<Card.Root class="bg-card">
		<Card.Header>
			<Card.Title>Watcher Version</Card.Title>
			<Card.Description>Current version and system info</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-4">
			{#if versionInfo}
				<div class="grid grid-cols-2 lg:grid-cols-4 gap-4">
					<div class="bg-muted/50 p-3 rounded border border-border">
						<div class="text-xs text-muted-foreground mb-1">Version</div>
						<div class="font-mono text-sm">{versionInfo.version}</div>
					</div>
					<div class="bg-muted/50 p-3 rounded border border-border">
						<div class="text-xs text-muted-foreground mb-1">Go Runtime</div>
						<div class="font-mono text-sm">{versionInfo.go_version}</div>
					</div>
					<div class="bg-muted/50 p-3 rounded border border-border">
						<div class="text-xs text-muted-foreground mb-1">Platform</div>
						<div class="font-mono text-sm">{versionInfo.os} / {versionInfo.arch}</div>
					</div>
					<div class="bg-muted/50 p-3 rounded border border-border lg:col-span-4">
						<div class="text-xs text-muted-foreground mb-1">Executable Path</div>
						<div class="font-mono text-xs truncate break-all">{versionInfo.executable}</div>
					</div>
				</div>
			{:else if !error}
				<div class="animate-pulse bg-muted/50 h-24 rounded"></div>
			{/if}
			
			<div class="pt-4 border-t border-border mt-4">
				<Button.Root onclick={checkForUpdates} disabled={isChecking || isUpdating} variant="outline">
					<RotateCcw class={`w-4 h-4 mr-2 ${isChecking ? 'animate-spin' : ''}`} />
					{isChecking ? 'Checking...' : 'Check for Updates'}
				</Button.Root>
			</div>

			{#if updateInfo}
				<div class="mt-4 p-4 rounded border {updateInfo.update_available ? 'border-blue-500/50 bg-blue-500/5 text-blue-50' : 'border-emerald-500/30 bg-emerald-500/5'}">
					{#if updateInfo.update_available}
						<div class="flex items-start justify-between">
							<div>
								<h4 class="font-medium text-blue-400 flex items-center gap-2 mb-1">
									<Download class="w-4 h-4" /> Update Available
								</h4>
								<p class="text-sm">A new version of Watcher <strong>{updateInfo.latest_version}</strong> is available.</p>
								<p class="text-xs text-muted-foreground mt-1">Currently running: {updateInfo.current_version}</p>
							</div>
							<Button.Root onclick={() => (showUpdateDialog = true)} disabled={isUpdating} class="bg-blue-600 hover:bg-blue-700 text-white">
								{isUpdating ? 'Updating...' : 'Update & Restart Watcher'}
							</Button.Root>
						</div>
					{:else}
						<div class="flex items-center gap-2 text-emerald-500 text-sm font-medium">
							<CheckCircle2 class="w-4 h-4" /> Waiter is up to date (running the latest version: {updateInfo.latest_version}).
						</div>
					{/if}
				</div>
			{/if}
		</Card.Content>
	</Card.Root>

	<Card.Root class="bg-card">
		<Card.Header>
			<Card.Title class="text-red-400 flex items-center gap-2">
				<AlertTriangle class="w-4 h-4" /> Uninstall Watcher
			</Card.Title>
			<Card.Description>Generate a PowerShell script to safely remove the Watcher agent, services, and registry keys.</Card.Description>
		</Card.Header>
		<Card.Content>
			<Button.Root variant="destructive" onclick={generateUninstall} class="mb-4">
				Generate Uninstall Script
			</Button.Root>

			{#if uninstallScript}
				<div class="relative bg-[#0a0a0a] border border-red-500/30 rounded p-4">
					<Button.Root variant="secondary" size="icon" class="absolute top-2 right-2 h-8 w-8 text-xs bg-muted hover:bg-muted/80" onclick={copyUninstallScript}>
						<Copy class="w-3.5 h-3.5" />
					</Button.Root>
					<pre class="font-mono text-xs text-red-300 overflow-x-auto p-2 leading-relaxed"><code>{uninstallScript}</code></pre>
				</div>
				<p class="text-xs text-muted-foreground mt-2">
					Save this script as <code>uninstall-watcher.ps1</code> and run it from an elevated PowerShell window to completely remove watcher.
				</p>
			{/if}
		</Card.Content>
	</Card.Root>
</div>

<Dialog.Root bind:open={showRestartDialog}>
	<Dialog.Content class="sm:max-w-[460px]">
		<Dialog.Header>
			<Dialog.Title>Restart Watcher Service</Dialog.Title>
			<Dialog.Description>
				Restart watcher service now? This may temporarily disconnect the dashboard.
			</Dialog.Description>
		</Dialog.Header>
		<Dialog.Footer>
			<Button.Root variant="outline" type="button" onclick={() => (showRestartDialog = false)}>
				Cancel
			</Button.Root>
			<Button.Root type="button" onclick={restartWatcherService}>
				Restart
			</Button.Root>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<Dialog.Root bind:open={showUpdateDialog}>
	<Dialog.Content class="sm:max-w-[460px]">
		<Dialog.Header>
			<Dialog.Title>Update Watcher</Dialog.Title>
			<Dialog.Description>
				Update Watcher now? The service will be restarted automatically.
			</Dialog.Description>
		</Dialog.Header>
		<Dialog.Footer>
			<Button.Root variant="outline" type="button" onclick={() => (showUpdateDialog = false)} disabled={isUpdating}>
				Cancel
			</Button.Root>
			<Button.Root type="button" class="bg-blue-600 hover:bg-blue-700 text-white" onclick={performUpdate} disabled={isUpdating}>
				{isUpdating ? 'Updating...' : 'Update & Restart'}
			</Button.Root>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
