<script lang="ts">
	import RequestError from '$lib/components/request-error.svelte';
	import { keys } from '$lib/query/options';
	import { message } from '$lib/query/client';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import { Download, RotateCcw, AlertTriangle, CheckCircle2, Copy } from '@lucide/svelte';
	import * as Tabs from '$lib/components/ui/tabs';
	import type { createSettings } from '$lib/features/settings/settings-controller.svelte';
	let { model }: { model: ReturnType<typeof createSettings> } = $props();
</script>

<Tabs.Content value="maintenance"
	><div class="space-y-6">
		{#if model.configDirty}<div
				class="section-toolbar rounded-lg border border-amber-500/30 bg-amber-500/5 p-3"
			>
				<p class="text-sm text-amber-300">
					Save or discard your configuration changes before restarting or updating.
				</p>
				<Button.Root
					size="sm"
					variant="outline"
					onclick={() => {
						model.ui.activeSection = 'deployment';
					}}>Review changes</Button.Root
				>
			</div>{/if}
		{#if !model.ui.showRestartDialog && !model.ui.showUpdateDialog}<RequestError
				message={model.ui.maintenanceError}
			/>{/if}
		{#if model.ui.maintenanceSuccess}<p role="status" class="text-sm text-green-400">
				{model.ui.maintenanceSuccess}
			</p>{/if}
		{#if model.updateState.updateStatus === 'failed'}
			<RequestError message={model.updateState.updateMessage} onRetry={model.performUpdate} />
		{:else if model.updateState.updateStatus !== 'idle'}
			<p
				role="status"
				class="rounded-lg border border-blue-500/30 bg-blue-500/10 p-3 text-sm text-blue-200"
			>
				{model.updateState.updateMessage}
			</p>
			{#if model.updateState.updateStatus === 'applied'}
				<Button.Root variant="outline" onclick={() => window.location.reload()}
					>Reload dashboard</Button.Root
				>
			{/if}
		{/if}
		<Card.Root class="bg-card">
			<Card.Header>
				<Card.Title>Watcher Version</Card.Title>
				<Card.Description>Current version and system info</Card.Description>
			</Card.Header>
			<Card.Content class="space-y-4">
				{#if model.versionInfo}
					<div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
						<div class="rounded border border-border bg-muted/50 p-3">
							<div class="mb-1 text-xs text-muted-foreground">Version</div>
							<div class="font-mono text-sm">{model.versionInfo.version}</div>
						</div>
						<div class="rounded border border-border bg-muted/50 p-3">
							<div class="mb-1 text-xs text-muted-foreground">Go Runtime</div>
							<div class="font-mono text-sm">{model.versionInfo.go_version}</div>
						</div>
						<div class="rounded border border-border bg-muted/50 p-3">
							<div class="mb-1 text-xs text-muted-foreground">Platform</div>
							<div class="font-mono text-sm">{model.versionInfo.os} / {model.versionInfo.arch}</div>
						</div>
						<div class="rounded border border-border bg-muted/50 p-3 lg:col-span-4">
							<div class="mb-1 text-xs text-muted-foreground">Executable Path</div>
							<div class="font-mono text-xs break-all">{model.versionInfo.executable}</div>
						</div>
					</div>
				{:else if !model.loadError}
					<div class="h-24 animate-pulse rounded bg-muted/50"></div>
				{/if}

				<div class="mt-4 border-t border-border pt-4">
					<Button.Root
						onclick={model.checkForUpdates}
						disabled={model.isChecking || model.agentBusy}
						variant="outline"
					>
						<RotateCcw class={`mr-2 h-4 w-4 ${model.isChecking ? 'animate-spin' : ''}`} />
						{model.isChecking ? 'Checking...' : 'Check for Updates'}
					</Button.Root>
				</div>

				{#if model.updateInfo}
					<div
						class="mt-4 rounded border p-4 {model.updateInfo.update_available
							? 'border-blue-500/50 bg-blue-500/5 text-blue-50'
							: 'border-emerald-500/30 bg-emerald-500/5'}"
					>
						{#if model.updateInfo.update_available}
							<div class="section-toolbar">
								<div>
									<h4 class="mb-1 flex items-center gap-2 font-medium text-blue-400">
										<Download class="h-4 w-4" /> Update Available
									</h4>
									<p class="text-sm">
										A new version of Watcher <strong>{model.updateInfo.latest_version}</strong> is available.
									</p>
									<p class="mt-1 text-xs text-muted-foreground">
										Currently running: {model.updateInfo.current_version}
									</p>
									{#if model.updateInfo.published_at}
										<p class="mt-1 text-xs text-muted-foreground">
											Published: {new Date(model.updateInfo.published_at).toLocaleString()}
										</p>
									{/if}
								</div>
								<Button.Root
									onclick={() => {
										model.ui.maintenanceError = '';
										model.ui.showUpdateDialog = true;
									}}
									disabled={model.agentBusy || model.configDirty || model.isSavingPassword}
									class="bg-blue-600 text-white hover:bg-blue-700"
								>
									{model.isUpdating ? 'Updating...' : 'Update & Restart Watcher'}
								</Button.Root>
							</div>
						{:else}
							<div class="flex items-center gap-2 text-sm font-medium text-emerald-500">
								<CheckCircle2 class="h-4 w-4" /> Watcher is up to date (running the latest version:
								{model.updateInfo.latest_version}).
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
					disabled={model.agentBusy || model.isSavingPassword || model.configDirty}
					onclick={() => {
						model.ui.maintenanceError = '';
						model.ui.showRestartDialog = true;
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
					>Generate a PowerShell script to safely remove the Watcher agent, services, and registry
					keys.</Card.Description
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
							onclick={model.generateUninstall}
							disabled={model.agentBusy}
							class="mb-4"
						>
							Generate Uninstall Script
						</Button.Root>

						{#if model.ui.uninstallScript}
							<div class="relative rounded border border-red-500/30 bg-[#0a0a0a] p-4">
								<Button.Root
									variant="secondary"
									size="icon"
									class="absolute top-2 right-2 h-8 w-8 bg-muted text-xs hover:bg-muted/80"
									onclick={model.copyUninstallScript}
									aria-label="Copy uninstall script"
								>
									<Copy class="h-3.5 w-3.5" />
								</Button.Root>
								<pre
									class="overflow-x-auto p-2 font-mono text-xs leading-relaxed text-red-300"><code
										>{model.ui.uninstallScript}</code
									></pre>
							</div>
							<p class="mt-2 text-xs text-muted-foreground">
								Save this script as <code>uninstall-watcher.ps1</code> and run it from an elevated PowerShell
								window to completely remove watcher.
							</p>
						{/if}
					</div>
				</details></Card.Content
			>
		</Card.Root>
	</div></Tabs.Content
>
