<script lang="ts">
	import { onMount } from 'svelte';
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
	import {
		ArrowLeft,
		Play,
		Square,
		RefreshCw,
		Heart,
		AlertCircle,
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

	let service = $state<Service | null>(null);
	let watcher = $state<Watcher | null>(null);
	let healthHistory = $state<HealthEvent[]>([]);
	let deploys = $state<DeployLog[]>([]);
	let deployPage = $state(1);
	let deployPageSize = $state(10);
	let deployTotal = $state(0);
	let logLines = $state<string[]>([]);
	let error = $state('');
	let actionMsg = $state('');
	let logError = $state('');
	let logType = $state<'out' | 'err'>('out');
	let logCount = $state(100);

	let envContent = $state('');
	let configFiles = $state<ServiceConfigFile[]>([]);
	let savingEnv = $state(false);
	let showDeleteDialog = $state(false);
	let deleting = $state(false);

	let activeTab = $state(page.url.searchParams.get('tab') || 'health');

	const id = Number(page.params.id);

	onMount(async () => {
		try {
			await refreshServiceDetail();
			healthHistory = await api.healthHistory(id, 50);
			await loadDeploys();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load service';
		}
		loadLogs();
	});

	async function loadDeploys() {
		const res = await api.serviceDeploys(id, deployPage, deployPageSize);
		deploys = res.data;
		deployTotal = res.total;
	}

	async function loadLogs() {
		logError = '';
		try {
			const res = await api.serviceLogs(id, logCount, logType);
			logLines = res.lines ?? [];
		} catch (e) {
			logError = e instanceof Error ? e.message : 'Failed to load logs';
			logLines = [];
		}
	}

	async function refreshServiceDetail() {
		const detail = await api.getService(id);
		service = detail.service;
		watcher = detail.watcher;
		envContent = detail.service.env_content || '';
		configFiles = [...(detail.service.config_files || []).map((file) => ({ ...file, target: file.target || 'app_dir' }))];
	}

	async function runAction(fn: () => Promise<{ message: string }>) {
		try {
			const res = await fn();
			actionMsg = res.message;
			setTimeout(() => (actionMsg = ''), 4000);
			if (service) await refreshServiceDetail();
		} catch (e) {
			actionMsg = e instanceof Error ? e.message : 'Action failed';
			setTimeout(() => (actionMsg = ''), 5000);
		}
	}

	async function saveEnv() {
		if (!service || !watcher) return;
		savingEnv = true;
		try {
			service = await api.updateService(watcher.id, service.id, {
				env_content: envContent,
				config_files: configFiles.filter((file) => file.file_path.trim() !== '')
			});
			envContent = service.env_content || '';
			configFiles = [...(service.config_files || []).map((file) => ({ ...file, target: file.target || 'app_dir' }))];
			actionMsg = 'Service files saved';
			setTimeout(() => (actionMsg = ''), 4000);
		} catch (e) {
			actionMsg = e instanceof Error ? e.message : 'Failed to save env';
		} finally {
			savingEnv = false;
		}
	}

	async function deleteService() {
		if (!service || !watcher) return;
		deleting = true;
		try {
			await api.deleteService(watcher.id, service.id);
			showDeleteDialog = false;
			await goto(resolve(`/watchers/${watcher.id}/edit#services`));
		} catch (e) {
			actionMsg = e instanceof Error ? e.message : 'Failed to delete service';
			setTimeout(() => (actionMsg = ''), 5000);
		} finally {
			deleting = false;
		}
	}

	async function checkHealth() {
		try {
			const h = await api.serviceHealth(id);
			actionMsg = `Health: ${h.status} (HTTP ${h.http_status})${h.error ? ' — ' + h.error : ''}`;
			healthHistory = await api.healthHistory(id, 50);
			setTimeout(() => (actionMsg = ''), 5000);
		} catch (e) {
			actionMsg = e instanceof Error ? e.message : 'Health check failed';
		}
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center gap-4">
		<a href={resolve('/services')}>
			<Button.Root variant="ghost" size="icon" class="h-8 w-8">
				<ArrowLeft class="h-4 w-4" />
			</Button.Root>
		</a>
		<div class="flex-1">
			<h1 class="text-2xl font-bold tracking-tight">
				{service?.windows_service_name ?? 'Loading...'}
			</h1>
			{#if watcher}
				<p class="text-sm text-muted-foreground">
					Watcher: <a href={resolve(`/watchers/${watcher.id}`)} class="hover:underline">{watcher.name}</a>
				</p>
			{/if}
		</div>
		{#if service}
			<div class="flex items-center gap-2">
				<a href={resolve(`/services/${id}/edit`)}>
					<Button.Root variant="outline" size="sm">
						<Pencil class="mr-1.5 h-4 w-4" /> Edit
					</Button.Root>
				</a>
				<Button.Root variant="outline" size="sm" class="text-red-400" onclick={() => (showDeleteDialog = true)}>
					<Trash2 class="mr-1.5 h-4 w-4" /> Delete
				</Button.Root>
				{#if !isIISService(service.service_type)}
					<Button.Root
						variant="outline"
						size="sm"
						class="text-emerald-400"
						onclick={() => runAction(() => api.startService(id))}
					>
						<Play class="mr-1.5 h-4 w-4" /> Start
					</Button.Root>
					<Button.Root
						variant="outline"
						size="sm"
						class="text-red-400"
						onclick={() => runAction(() => api.stopService(id))}
					>
						<Square class="mr-1.5 h-4 w-4" /> Stop
					</Button.Root>
					<Button.Root
						variant="outline"
						size="sm"
						class="text-amber-400"
						onclick={() => runAction(() => api.restartService(id))}
					>
						<RefreshCw class="mr-1.5 h-4 w-4" /> Restart
					</Button.Root>
				{/if}
				<Button.Root variant="outline" size="sm" class="text-blue-400" onclick={checkHealth}>
					<Heart class="mr-1.5 h-4 w-4" /> Health
				</Button.Root>
			</div>
		{/if}
	</div>

	{#if error}
		<div class="rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400">
			<AlertCircle class="mr-2 inline h-4 w-4" />
			{error}
		</div>
	{/if}

	{#if actionMsg}
		<div class="rounded-lg border border-blue-500/30 bg-blue-500/10 p-4 text-sm text-blue-400">
			{actionMsg}
		</div>
	{/if}

	{#if service}
		<!-- Service Info Card -->
		<Card.Root class="border-border bg-card">
			<Card.Content class="grid gap-4 p-6 sm:grid-cols-4">
				<div>
					<p class="text-xs text-muted-foreground">Hosting Mode</p>
					<p class="mt-1 text-sm">{serviceTypeLabel(service.service_type)}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">{isIISService(service.service_type) ? 'IIS App Kind' : 'Binary'}</p>
					<p class="mt-1 font-mono text-sm">{isIISService(service.service_type) ? iisAppKindLabel(service.iis_app_kind || 'static') : (service.binary_name || '—')}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">{isIISService(service.service_type) ? 'IIS App Pool' : 'Env File'}</p>
					<p class="mt-1 font-mono text-sm">{isIISService(service.service_type) ? (service.iis_app_pool || '—') : (service.env_file || '—')}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">{isIISService(service.service_type) ? 'IIS Site Name' : 'Health URL'}</p>
					<p class="mt-1 font-mono text-sm">{isIISService(service.service_type) ? (service.iis_site_name || '—') : (service.health_check_url || '—')}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">Health URL</p>
					<p class="mt-1 font-mono text-sm">{service.health_check_url || '—'}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">Install Dir</p>
					<p class="mt-1 font-mono text-sm">{watcher?.install_dir ?? '—'}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground">Public URL</p>
					<p class="mt-1 font-mono text-sm">
						{#if service.public_url}
							<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
							<a
								href={service.public_url}
								target="_blank"
								rel="noopener noreferrer"
								class="inline-flex items-center gap-1.5 text-blue-400 hover:underline"
							>
								{service.public_url}
								<ExternalLink class="h-3 w-3" />
							</a>
						{:else}
							—
						{/if}
					</p>
				</div>
			</Card.Content>
		</Card.Root>

		{#if isIISService(service.service_type)}
			<div class="rounded-lg border border-blue-500/30 bg-blue-500/5 p-4">
				<div class="mb-2 flex items-center gap-2 font-medium text-blue-400">
					<TerminalSquare class="h-5 w-5" />
					IIS Bootstrap
				</div>
				<p class="mb-4 text-sm text-foreground/80">
					Watcher can now create the IIS app pool and site automatically on first deploy when
					<code>iis_app_pool</code>, <code>iis_site_name</code>, and <code>public_url</code> are set. The
					root application is kept pointed at <code>{watcher?.install_dir}\current</code> on each deploy.
				</p>
				<p class="text-sm text-foreground/80">
					This service is configured as <code>{iisAppKindLabel(service.iis_app_kind || 'static')}</code>.
					Watcher will choose the IIS managed runtime automatically for that app kind, and if the site already
					exists it will reuse it and refresh the root path and app pool assignment.
				</p>
				<p class="text-sm text-foreground/80">
					Watcher does not install PHP, .NET hosting bundles, or IIS handler mappings. Those server-level
					prerequisites still need to exist before the deployed site can serve traffic successfully.
				</p>
			</div>
		{/if}

		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
		<Tabs.Root
			bind:value={activeTab}
			onValueChange={(v) => {
				if (v) goto(`?tab=${v}`, { replaceState: true, keepFocus: true, noScroll: true });
			}}
		>
			<Tabs.List>
				<Tabs.Trigger value="health">Health History ({healthHistory.length})</Tabs.Trigger>
				<Tabs.Trigger value="logs">Logs</Tabs.Trigger>
				<Tabs.Trigger value="env">Environment (.env)</Tabs.Trigger>
				<Tabs.Trigger value="deploys">Deploys ({deployTotal})</Tabs.Trigger>
			</Tabs.List>

			<!-- Health History -->
			<Tabs.Content value="health" class="mt-4">
				<HealthTab {healthHistory} />
			</Tabs.Content>

			<!-- Logs -->
			<Tabs.Content value="logs" class="mt-4">
				<LogsTab bind:logLines bind:logError bind:logType bind:logCount onLoadLogs={loadLogs} />
			</Tabs.Content>

			<!-- Environment -->
			<Tabs.Content value="env" class="mt-4">
				<EnvTab
					{service}
					bind:envContent
					bind:configFiles
					bind:savingEnv
					onSaveEnv={saveEnv}
					onSaveAndRestart={async () => {
						await saveEnv();
						await runAction(() => api.restartService(id));
					}}
				/>
			</Tabs.Content>

			<!-- Deploys -->
			<Tabs.Content value="deploys" class="mt-4">
				<DeploysTab
					bind:deploys
					bind:deployPage
					bind:deployPageSize
					{deployTotal}
					{watcher}
					onLoadDeploys={loadDeploys}
				/>
			</Tabs.Content>
		</Tabs.Root>
	{/if}
</div>

<Dialog.Root bind:open={showDeleteDialog}>
	<Dialog.Content class="sm:max-w-[420px]">
		<Dialog.Header>
			<Dialog.Title>Delete Service</Dialog.Title>
			<Dialog.Description>
				Delete <span class="font-medium">{service?.windows_service_name || 'this service'}</span>? This removes it from Watcher.
			</Dialog.Description>
		</Dialog.Header>
		<Dialog.Footer>
			<Button.Root variant="outline" type="button" onclick={() => (showDeleteDialog = false)} disabled={deleting}>
				Cancel
			</Button.Root>
			<Button.Root type="button" class="bg-red-600 text-white hover:bg-red-700" onclick={deleteService} disabled={deleting}>
				{deleting ? 'Deleting...' : 'Delete Service'}
			</Button.Root>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
