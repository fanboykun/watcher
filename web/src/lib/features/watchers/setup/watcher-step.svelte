<script lang="ts">
	import { type Service } from '$lib/api';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Check } from '@lucide/svelte';
	import type { createWatcherSetup } from '$lib/features/watchers/setup/setup-controller.svelte';
	let { model }: { model: ReturnType<typeof createWatcherSetup> } = $props();
</script>

<div class="space-y-4">
	{#if model.inspectServices().length > 0}
		<div class="space-y-3 rounded-lg border border-border/70 bg-muted/20 p-4">
			<div class="flex items-center justify-between gap-3 text-sm">
				<div>
					<p class="font-medium">Detected deploy target</p>
					<p class="text-xs text-muted-foreground">
						Choose the release target that should drive this watcher's `service_name`.
					</p>
				</div>
				<span class="rounded border border-border px-2 py-1 text-xs text-muted-foreground">
					{model.ui.inspectResult?.source === 'manifest' ? 'version.json' : 'repo assets'}
				</span>
			</div>
			<div class="grid gap-2 lg:grid-cols-2">
				{#each model.inspectServices() as target (target.name)}
					<button
						type="button"
						class={`rounded-lg border p-3 text-left text-sm transition ${
							model.ui.selectedInspectService === target.name
								? 'border-primary bg-primary/10'
								: 'border-border/70 bg-background/60 hover:bg-muted/40'
						}`}
						onclick={() => model.selectInspectTarget(target.name)}
					>
						<div class="flex items-start justify-between gap-2">
							<span class="font-medium">{target.name}</span>
							<span class="text-xs text-muted-foreground">{target.version}</span>
						</div>
						<p class="mt-1 truncate font-mono text-xs text-muted-foreground">
							{target.artifact}
						</p>
					</button>
				{/each}
			</div>
		</div>
	{/if}

	<div class="grid gap-4 md:grid-cols-2">
		<div class="space-y-2">
			<Label for="watcherName">Watcher Display Name</Label>
			<Input id="watcherName" bind:value={model.ui.watcherName} placeholder="my-app" />
		</div>
		<div class="space-y-2">
			<Label for="watcherServiceName">App/Service Name ID</Label>
			<Input
				id="watcherServiceName"
				bind:value={model.ui.watcherServiceName}
				placeholder="my-app"
			/>
		</div>
		<div class="space-y-2 md:col-span-2">
			<Label for="metadataURL">Metadata URL</Label>
			<Input id="metadataURL" bind:value={model.ui.metadataURL} />
		</div>
		<div class="space-y-2 md:col-span-2">
			<Label for="installDir">Base Install Directory</Label>
			<Input id="installDir" bind:value={model.ui.installDir} placeholder="C:\apps\my-app" />
		</div>
		<div class="space-y-2">
			<Label for="checkInterval">Check Interval (sec)</Label>
			<Input id="checkInterval" type="number" min="10" bind:value={model.ui.checkInterval} />
		</div>
		<div class="space-y-2">
			<Label for="deploymentEnvironment">Deployment Environment</Label>
			<Input
				id="deploymentEnvironment"
				bind:value={model.ui.deploymentEnvironment}
				placeholder="production"
			/>
		</div>
		<div class="space-y-2 md:col-span-2">
			<Label for="watcherGitHubToken">Watcher GitHub Token Override</Label>
			<Input
				id="watcherGitHubToken"
				type="password"
				bind:value={model.ui.watcherGitHubToken}
				disabled={!model.ui.useWatcherGitHubToken}
				placeholder="Optional watcher-specific token"
			/>
			<div class="mt-2 flex items-center gap-2">
				<Checkbox id="useWatcherGitHubToken" bind:checked={model.ui.useWatcherGitHubToken} />
				<Label for="useWatcherGitHubToken">Use watcher-specific GitHub token</Label>
			</div>
		</div>
		<div class="flex items-center gap-2 md:col-span-2">
			<Checkbox id="healthChecksEnabled" bind:checked={model.ui.healthChecksEnabled} />
			<Label for="healthChecksEnabled">Enable watcher-level health checks</Label>
		</div>
		<div class="space-y-2 md:col-span-2">
			<Label for="healthCheckURL">Global Health Check URL</Label>
			<Input
				id="healthCheckURL"
				bind:value={model.ui.healthCheckURL}
				placeholder="http://localhost:3000/health"
			/>
		</div>
		<div class="space-y-2 rounded-lg border border-border/70 bg-muted/20 p-4 md:col-span-2">
			<Label class="text-sm font-medium">Deployment Policy</Label>
			<div class="mt-2 space-y-2">
				<label class="flex cursor-pointer items-start gap-3">
					<input
						type="radio"
						name="autoDeploy"
						class="mt-1"
						checked={model.ui.autoDeploy}
						onchange={() => (model.ui.autoDeploy = true)}
					/>
					<div>
						<p class="text-sm font-medium">Automatic Deployment</p>
						<p class="text-xs text-muted-foreground">
							Automatically pull and deploy new releases as soon as they are detected.
						</p>
					</div>
				</label>
				<label class="flex cursor-pointer items-start gap-3">
					<input
						type="radio"
						name="autoDeploy"
						class="mt-1"
						checked={!model.ui.autoDeploy}
						onchange={() => (model.ui.autoDeploy = false)}
					/>
					<div>
						<p class="text-sm font-medium">
							Manual Approval Required (Release First, Deploy Later)
						</p>
						<p class="text-xs text-muted-foreground">
							Hold new versions as Release Candidates. Prepare environment variables and deploy
							manually during maintenance windows.
						</p>
					</div>
				</label>
			</div>
		</div>
	</div>
</div>
