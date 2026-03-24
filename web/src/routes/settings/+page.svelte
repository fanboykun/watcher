<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type SelfVersionResponse, type SelfUpdateCheckResponse } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import { Download, Info, RotateCcw, AlertTriangle, CheckCircle2, Copy } from '@lucide/svelte';

	let versionInfo = $state<SelfVersionResponse | null>(null);
	let updateInfo = $state<SelfUpdateCheckResponse | null>(null);
	let error = $state('');
	
	let isChecking = $state(false);
	let isUpdating = $state(false);
	let uninstallScript = $state('');

	onMount(() => {
		const init = async () => {
			try {
				versionInfo = await api.selfVersion();
			} catch (e) {
				error = e instanceof Error ? e.message : 'Failed to load version info';
			}
		};
		init();
	});

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
		if (!confirm('Are you sure you want to update Watcher? Notice: Watcher will be restarted.')) return;
		isUpdating = true;
		error = '';
		try {
			const res = await api.selfUpdate();
			// Using native alert since toaster isn't available
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

	{#if error}
		<div class="rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400">
			{error}
		</div>
	{/if}

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
							<Button.Root onclick={performUpdate} disabled={isUpdating} class="bg-blue-600 hover:bg-blue-700 text-white">
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
