<!-- eslint-disable svelte/no-navigation-without-resolve -->
<script lang="ts">
	import RequestError from '$lib/components/request-error.svelte';
	import { afterNavigate } from '$app/navigation';
	import { onMount } from 'svelte';
	import ServiceStatus from '$lib/components/service-status.svelte';
	import { page } from '$app/state';
	import {
		api,
		iisAppKindLabel,
		isIISService,
		serviceTypeLabel,
		type DeployLog,
		type HealthEvent,
		type IISAppKind,
		type Service,
		type ServiceConfigFile,
		type Watcher
	} from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import {
		ArrowLeft,
		Play,
		Square,
		RefreshCw,
		ChevronDown,
		ExternalLink,
		TerminalSquare,
		Pencil,
		Trash2
	} from '@lucide/svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import HealthTab from './components/health-tab.svelte';
	import DeploysTab from './components/deploys-tab.svelte';
	import LogsTab from './components/logs-tab.svelte';
	import EnvTab from './components/env-tab.svelte';
	import CandidatesTab from './components/candidates-tab.svelte';
	import RequestLoading from '$lib/components/request-loading.svelte';

	let service = $state<Service | null>(null);
	let watcher = $state<Watcher | null>(null);
	let healthHistory = $state<HealthEvent[]>([]);
	let deploys = $state<DeployLog[]>([]);
	let deployPage = $state(1);
	let deployPageSize = $state(10);
	let deployTotal = $state(0);
	let logLines = $state<string[]>([]);
	let error = $state('');
	let loading = $state(true);
	let actionMsg = $state('');
	let logError = $state('');
	let statusError = $state('');
	let checkingStatus = $state(false);
	let logsLoading = $state(false);
	let healthLoading = $state(false);
	let healthError = $state('');
	let deploysLoading = $state(false);
	let deploysError = $state('');
	let logType = $state<'out' | 'err'>('out');
	let logCount = $state(100);

	let envContent = $state('');
	let configFiles = $state<ServiceConfigFile[]>([]);
	let savingEnv = $state(false);
	let showDeleteDialog = $state(false);
	let deleting = $state(false);
	let pendingAction = $state('');
	let restartingEnv = $state(false);
	let confirmServiceAction = $state<'stop' | 'restart' | null>(null);
	const actionBusy = $derived(
		Boolean(pendingAction) ||
			savingEnv ||
			restartingEnv ||
			deleting ||
			watcher?.status === 'deploying'
	);

	let activeTab = $state(page.url.searchParams.get('tab') || 'health');

	const id = $derived(Number(page.params.id));
	let loadedID: number | null = null;
	let mounted = false;
	let detailGeneration = 0;
	let statusRequest = 0;
	let healthRequest = 0;
	onMount(() => {
		mounted = true;
		loadCurrentService();
		return () => {
			mounted = false;
			detailGeneration++;
		};
	});
	afterNavigate(() => {
		if (mounted) loadCurrentService();
	});

	function loadCurrentService() {
		if (loadedID === id) return;
		loadedID = id;
		void loadDetail();
	}

	async function loadDetail() {
		const generation = ++detailGeneration;
		loading = true;
		checkingStatus = false;
		statusError = '';
		healthHistory = [];
		deploys = [];
		deployTotal = 0;
		logLines = [];
		logsLoading = false;
		error = '';
		service = null;
		watcher = null;
		try {
			await refreshServiceDetail();
		} catch (e) {
			if (generation === detailGeneration)
				error = e instanceof Error ? e.message : 'Failed to load service';
		} finally {
			if (generation === detailGeneration) loading = false;
		}
		if (generation !== detailGeneration || !mounted || !service) return;
		void refreshStatus();
		void loadHealthHistory();
		void loadDeploys();
		void loadLogs();
	}

	async function loadHealthHistory() {
		const generation = detailGeneration;
		const request = ++healthRequest;
		healthLoading = true;
		healthError = '';
		try {
			const history = await api.healthHistory(id, 50);
			if (generation === detailGeneration && request === healthRequest) healthHistory = history;
		} catch (e) {
			if (generation === detailGeneration && request === healthRequest)
				healthError = e instanceof Error ? e.message : 'Failed to load health history';
		} finally {
			if (generation === detailGeneration && request === healthRequest) healthLoading = false;
		}
	}

	async function loadDeploys() {
		const generation = detailGeneration;
		deploysLoading = true;
		deploysError = '';
		try {
			const res = await api.serviceDeploys(id, deployPage, deployPageSize);
			if (generation !== detailGeneration) return;
			deploys = res.data;
			deployTotal = res.total;
		} catch (e) {
			if (generation === detailGeneration)
				deploysError = e instanceof Error ? e.message : 'Failed to load deployments';
		} finally {
			if (generation === detailGeneration) deploysLoading = false;
		}
	}

	async function loadLogs() {
		if (logsLoading) return;
		const generation = detailGeneration;
		logsLoading = true;
		logError = '';
		try {
			const res = await api.serviceLogs(id, logCount, logType);
			if (generation === detailGeneration) logLines = res.lines ?? [];
		} catch (e) {
			if (generation !== detailGeneration) return;
			logError = e instanceof Error ? e.message : 'Failed to load logs';
			logLines = [];
		} finally {
			if (generation === detailGeneration) logsLoading = false;
		}
	}

	async function refreshServiceDetail() {
		const serviceID = id;
		const generation = detailGeneration;
		const detail = await api.getService(serviceID);
		if (id !== serviceID || generation !== detailGeneration) return;
		service = detail.service;
		watcher = detail.watcher;
		envContent = service?.env_content || '';
		configFiles = [
			...(detail.service.config_files || []).map((file) => ({
				...file,
				target: file.target || 'app_dir'
			}))
		];
	}

	async function runAction(name: string, fn: () => Promise<{ message: string }>) {
		if (pendingAction) return;
		pendingAction = name;
		error = '';
		try {
			const res = await fn();
			actionMsg = res.message;
			setTimeout(() => (actionMsg = ''), 4000);
			if (service) {
				await refreshServiceDetail();
				void refreshStatus(true);
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Action failed';
		} finally {
			pendingAction = '';
		}
	}

	async function saveEnv() {
		if (!service || !watcher || savingEnv) return false;
		savingEnv = true;
		error = '';
		try {
			service = await api.updateService(watcher.id, service.id, {
				env_content: envContent,
				config_files: configFiles.filter((file) => file.file_path.trim() !== '')
			});
			envContent = service.env_content || '';
			configFiles = [
				...(service.config_files || []).map((file) => ({
					...file,
					target: file.target || 'app_dir'
				}))
			];
			actionMsg = 'Service files saved';
			setTimeout(() => (actionMsg = ''), 4000);
			return true;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save env';
			return false;
		} finally {
			savingEnv = false;
		}
	}

	async function deleteService() {
		if (!service || !watcher || deleting) return;
		error = '';
		deleting = true;
		try {
			await api.deleteService(watcher.id, service.id);
			showDeleteDialog = false;
			await goto(resolve(`/watchers/${watcher.id}/edit#services`));
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to delete service';
			setTimeout(() => (actionMsg = ''), 5000);
		} finally {
			deleting = false;
		}
	}

	async function refreshStatus(force = false) {
		if ((checkingStatus && !force) || !service) return;
		const request = ++statusRequest;
		const serviceID = id;
		const generation = detailGeneration;
		checkingStatus = true;
		statusError = '';
		try {
			const status = await api.serviceStatus(serviceID);
			if (generation !== detailGeneration || request !== statusRequest || !service) return;
			service = {
				...service,
				last_service_status: status.last_service_status,
				last_service_error: status.last_service_error,
				last_service_checked_at: status.last_service_checked_at,
				last_health_status: status.last_health_status,
				last_health_http_status: status.last_health_http_status,
				last_health_error: status.last_health_error,
				last_health_checked_at: status.last_health_checked_at
			};
			void loadHealthHistory();
		} catch (e) {
			if (generation === detailGeneration && request === statusRequest)
				statusError = e instanceof Error ? e.message : 'Status check failed';
		} finally {
			if (generation === detailGeneration && request === statusRequest) checkingStatus = false;
		}
	}
</script>

<div class="space-y-6">
	<header class="page-header">
		<div class="page-identity">
			<Button.Root
				href={resolve('/services')}
				variant="ghost"
				size="icon"
				aria-label="Back to services"><ArrowLeft /></Button.Root
			>
			<div class="min-w-0">
				<h1 class="text-2xl font-semibold tracking-tight">
					{service?.windows_service_name || service?.binary_name || 'Service'}
				</h1>
				{#if watcher}<a
						href={resolve(`/watchers/${watcher.id}`)}
						class="text-sm text-muted-foreground hover:underline">Watcher · {watcher.name}</a
					>{/if}
			</div>
		</div>
		{#if service}
			<div class="page-actions">
				<Button.Root
					href={resolve(`/services/${id}/edit`)}
					variant="outline"
					size="sm"
					disabled={actionBusy}><Pencil /> Edit</Button.Root
				>
				<DropdownMenu.Root>
					<DropdownMenu.Trigger
						class={Button.buttonVariants({ variant: 'outline', size: 'sm' })}
						disabled={actionBusy}
						aria-busy={Boolean(pendingAction)}
					>
						{#if pendingAction}<RefreshCw class="animate-spin" />Working…{:else}Actions<ChevronDown
							/>{/if}
					</DropdownMenu.Trigger>
					<DropdownMenu.Content
						align="end"
						class="w-44"
						onCloseAutoFocus={(event) => {
							if (confirmServiceAction || showDeleteDialog) event.preventDefault();
						}}
					>
						{#if !isIISService(service.service_type)}
							<DropdownMenu.Item
								onSelect={() => {
									void runAction('start', () => api.startService(id));
								}}><Play />Start service</DropdownMenu.Item
							>
							<DropdownMenu.Item
								onSelect={() => {
									error = '';
									confirmServiceAction = 'restart';
								}}><RefreshCw />Restart service</DropdownMenu.Item
							>
							<DropdownMenu.Item
								onSelect={() => {
									error = '';
									confirmServiceAction = 'stop';
								}}><Square />Stop service</DropdownMenu.Item
							>
						{/if}
						<DropdownMenu.Item
							variant="destructive"
							class="mt-1 border-t border-border pt-2"
							onSelect={() => {
								error = '';
								showDeleteDialog = true;
							}}><Trash2 />Delete service</DropdownMenu.Item
						>
					</DropdownMenu.Content>
				</DropdownMenu.Root>
			</div>
		{/if}
	</header>

	{#if !showDeleteDialog && !confirmServiceAction}<RequestError
			message={error}
			onRetry={!service ? loadDetail : undefined}
		/>{/if}

	{#if actionMsg}
		<div class="rounded-lg border border-blue-500/30 bg-blue-500/10 p-4 text-sm text-blue-400">
			{actionMsg}
		</div>
	{/if}

	{#if loading}
		<RequestLoading label="Loading service details…" />
	{:else if service}
		<ServiceStatus
			{service}
			variant="detail"
			checking={checkingStatus}
			onRefresh={() => refreshStatus()}
			refreshDisabled={Boolean(pendingAction)}
		/>
		<RequestError
			message={statusError}
			title="Status could not be refreshed"
			onRetry={() => refreshStatus()}
		/>
		<!-- Service Info Card -->
		<Card.Root class="border-border bg-card">
			<Card.Content class="grid min-w-0 grid-cols-2 gap-x-6 gap-y-4 px-6 sm:grid-cols-3">
				<div>
					<p class="text-xs text-muted-foreground">Hosting Mode</p>
					<p class="mt-1 text-sm">{serviceTypeLabel(service.service_type)}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">
						{isIISService(service.service_type) ? 'IIS App Kind' : 'Binary'}
					</p>
					<p class="mt-1 font-mono text-sm break-all">
						{isIISService(service.service_type)
							? iisAppKindLabel(service.iis_app_kind || 'static')
							: service.binary_name || '—'}
					</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">
						{isIISService(service.service_type) ? 'IIS App Pool' : 'Env File'}
					</p>
					<p class="mt-1 font-mono text-sm break-all">
						{isIISService(service.service_type)
							? service.iis_app_pool || '—'
							: service.env_file || '—'}
					</p>
				</div>
				{#if isIISService(service.service_type)}
					<div>
						<p class="text-xs text-muted-foreground">IIS Site Name</p>
						<p class="mt-1 font-mono text-sm break-all">{service.iis_site_name || '—'}</p>
					</div>
				{/if}
				<div>
					<p class="text-xs text-muted-foreground">Health URL</p>
					<p class="mt-1 font-mono text-sm break-all">
						{service.health_check_url || watcher?.hc_url || 'Not configured'}
					</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">Install Dir</p>
					<p class="mt-1 font-mono text-sm break-all">{watcher?.install_dir ?? '—'}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">Public URL</p>
					<p class="mt-1 font-mono text-sm break-all">
						{#if service.public_url}
							<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
							<Button.Root
								variant="link"
								size="sm"
								href={service.public_url}
								data-sveltekit-noscroll
								target="_blank"
								rel="noopener noreferrer"
								class="h-auto min-w-0 justify-start gap-1.5 p-0 text-left text-xs break-all whitespace-normal text-blue-400 hover:underline"
							>
								{service.public_url}
								<ExternalLink class="h-3 w-3" />
							</Button.Root>
						{:else}
							—
						{/if}
					</p>
				</div>
			</Card.Content>
		</Card.Root>

		{#if isIISService(service.service_type)}
			<details class="rounded-lg border border-border p-4">
				<summary class="cursor-pointer text-sm font-medium">IIS deployment details</summary>
				<div class="mb-2 flex items-center gap-2 font-medium text-blue-400">
					<TerminalSquare class="h-5 w-5" />
					IIS Bootstrap
				</div>
				<p class="mb-4 text-sm text-foreground/80">
					Watcher can now create the IIS app pool and site automatically on first deploy when
					<code>iis_app_pool</code>, <code>iis_site_name</code>, and <code>public_url</code> are
					set. The root application is kept pointed at <code>{watcher?.install_dir}.urrent</code> on each
					deploy.
				</p>
				<p class="text-sm text-foreground/80">
					This service is configured as <code
						>{iisAppKindLabel(service.iis_app_kind || 'static')}</code
					>. Watcher will choose the IIS managed runtime automatically for that app kind, and if the
					site already exists it will reuse it and refresh the root path and app pool assignment.
				</p>
				<p class="text-sm text-foreground/80">
					Watcher does not install PHP, .NET hosting bundles, or IIS handler mappings. Those
					server-level prerequisites still need to exist before the deployed site can serve traffic
					successfully.
				</p>
			</details>
		{/if}

		<Tabs.Root
			bind:value={activeTab}
			onValueChange={(v) => {
				if (v) {
					goto(resolve(`/services/[id]?tab=${v}`, { id: String(id) }), {
						replaceState: true,
						keepFocus: true,
						noScroll: true
					}).catch(() => {});
				}
			}}
		>
			<Tabs.List>
				<Tabs.Trigger value="health">Health</Tabs.Trigger>
				<Tabs.Trigger value="logs">Logs</Tabs.Trigger>
				<Tabs.Trigger value="env">Files</Tabs.Trigger>
				<Tabs.Trigger value="candidates">Candidates</Tabs.Trigger>
				<Tabs.Trigger value="deploys">Deploys ({deployTotal})</Tabs.Trigger>
			</Tabs.List>

			<!-- Health History -->
			<Tabs.Content value="health" class="mt-4">
				<RequestError message={healthError} onRetry={loadHealthHistory} />
				{#if healthLoading && healthHistory.length === 0}<RequestLoading
						label="Loading health history…"
					/>{:else}<HealthTab {healthHistory} />{/if}
			</Tabs.Content>

			<!-- Logs -->
			<Tabs.Content value="logs" class="mt-4">
				<LogsTab
					loading={logsLoading}
					bind:logLines
					bind:logError
					bind:logType
					bind:logCount
					onLoadLogs={loadLogs}
				/>
			</Tabs.Content>

			<!-- Environment -->
			<Tabs.Content value="env" class="mt-4">
				<EnvTab
					{service}
					bind:envContent
					bind:configFiles
					savingEnv={savingEnv || restartingEnv || actionBusy}
					onSaveEnv={async () => {
						await saveEnv();
					}}
					onSaveAndRestart={async () => {
						restartingEnv = true;
						try {
							if (await saveEnv()) await runAction('restart', () => api.restartService(id));
						} finally {
							restartingEnv = false;
						}
					}}
				/>
			</Tabs.Content>

			<!-- Deploys -->
			<Tabs.Content value="deploys" class="mt-4">
				<RequestError message={deploysError} onRetry={loadDeploys} />
				{#if deploysLoading}<RequestLoading label="Loading deployments…" />{:else}
					<DeploysTab
						bind:deploys
						bind:deployPage
						bind:deployPageSize
						{deployTotal}
						{watcher}
						onLoadDeploys={loadDeploys}
					/>
				{/if}
			</Tabs.Content>

			<Tabs.Content value="candidates" class="mt-4">
				<CandidatesTab
					serviceId={id}
					currentEnv={service?.env_content || ''}
					watcherId={watcher?.id}
					pendingVersion={watcher?.pending_version}
				/>
			</Tabs.Content>
		</Tabs.Root>
	{/if}
</div>

<Dialog.Root
	bind:open={showDeleteDialog}
	onOpenChange={(open) => {
		if (deleting && !open) showDeleteDialog = true;
	}}
>
	<Dialog.Content class="sm:max-w-[420px]" showCloseButton={!deleting}>
		<Dialog.Header>
			<Dialog.Title>Delete Service</Dialog.Title>
			<Dialog.Description>
				Delete <span class="font-medium">{service?.windows_service_name || 'this service'}</span>?
				This removes it from Watcher.
			</Dialog.Description>
		</Dialog.Header>
		<RequestError message={error} />
		<Dialog.Footer>
			<Button.Root
				variant="outline"
				type="button"
				onclick={() => (showDeleteDialog = false)}
				disabled={deleting}
			>
				Cancel
			</Button.Root>
			<Button.Root
				type="button"
				class="bg-red-600 text-white hover:bg-red-700"
				onclick={deleteService}
				disabled={deleting}
			>
				{deleting ? 'Deleting...' : 'Delete Service'}
			</Button.Root>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<Dialog.Root
	open={confirmServiceAction !== null}
	onOpenChange={(open) => {
		if (!open && !pendingAction) confirmServiceAction = null;
	}}
>
	<Dialog.Content showCloseButton={!pendingAction}>
		<Dialog.Header
			><Dialog.Title
				>{confirmServiceAction === 'stop' ? 'Stop service?' : 'Restart service?'}</Dialog.Title
			><Dialog.Description
				>{service?.windows_service_name} will {confirmServiceAction === 'stop'
					? 'stop serving traffic.'
					: 'briefly stop serving traffic while it restarts.'}</Dialog.Description
			></Dialog.Header
		>
		<RequestError message={error} />
		<Dialog.Footer>
			<Button.Root
				variant="outline"
				disabled={Boolean(pendingAction)}
				onclick={() => (confirmServiceAction = null)}>Cancel</Button.Root
			>
			<Button.Root
				variant="destructive"
				disabled={Boolean(pendingAction)}
				loading={Boolean(pendingAction)}
				onclick={async () => {
					const action = confirmServiceAction;
					if (!action) return;
					await runAction(action, () =>
						action === 'stop' ? api.stopService(id) : api.restartService(id)
					);
					if (!error) confirmServiceAction = null;
				}}>{confirmServiceAction === 'stop' ? 'Stop service' : 'Restart service'}</Button.Root
			>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
