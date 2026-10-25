<script lang="ts">
	import MaintenanceSection from '$lib/features/settings/maintenance-section.svelte';
	import AccessSection from '$lib/features/settings/access-section.svelte';
	import InstallationSection from '$lib/features/settings/installation-section.svelte';
	import WebhooksSection from '$lib/features/settings/webhooks-section.svelte';
	import DeploymentSection from '$lib/features/settings/deployment-section.svelte';
	import PageHeading from '$lib/components/console/page-heading.svelte';
	import RequestError from '$lib/components/request-error.svelte';

	import { message } from '$lib/query/client';

	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';

	import * as Tabs from '$lib/components/ui/tabs';
	import RequestLoading from '$lib/components/request-loading.svelte';
	import { createSettings } from '$lib/features/settings/settings-controller.svelte';
	const model = createSettings();
</script>

<svelte:head>
	<title>Settings | Watcher</title>
</svelte:head>

<div class="space-y-6">
	<PageHeading
		eyebrow="Administration / Agent"
		title="Settings"
		description="Configure your environment, manage access, and keep the agent up to date."
	/>

	{#if model.loading}
		<RequestLoading label="Loading global settings…" />
	{:else if model.loadError}
		<RequestError message={model.loadError} onRetry={model.loadSettings} />
	{:else}
		<Tabs.Root bind:value={model.ui.activeSection}>
			<Tabs.List>
				<Tabs.Trigger value="deployment">Deployment</Tabs.Trigger>
				<Tabs.Trigger value="webhooks">Webhooks</Tabs.Trigger>
				<Tabs.Trigger value="installation">Installation</Tabs.Trigger>
				<Tabs.Trigger value="access">Access</Tabs.Trigger>
				<Tabs.Trigger value="maintenance">Maintenance</Tabs.Trigger>
			</Tabs.List>
			<DeploymentSection {model} />
			<WebhooksSection {model} />
			<InstallationSection {model} />
			<AccessSection {model} />
			<MaintenanceSection {model} />
		</Tabs.Root>
		{#if ['deployment', 'webhooks', 'installation'].includes(model.ui.activeSection)}
			<div
				class="sticky bottom-3 z-10 space-y-3 rounded-lg border border-border bg-background p-4 shadow-lg"
			>
				<RequestError message={model.ui.configError} />
				{#if model.ui.configSuccess && !model.configDirty}<p
						role="status"
						class="text-sm text-green-400"
					>
						{model.ui.configSuccess}
					</p>{/if}
				{#if model.ui.saveNotes.length && !model.configDirty}<ul
						class="list-inside list-disc text-xs text-muted-foreground"
					>
						{#each model.ui.saveNotes as note (note)}<li>{note}</li>{/each}
					</ul>{/if}
				<div class="section-toolbar">
					<div class="min-w-0">
						<p class="text-sm font-medium">
							{model.configDirty ? 'Unsaved changes' : 'All changes saved'}
						</p>
						<p class="text-xs text-muted-foreground">
							Saves deployment, webhook and installation settings together.
						</p>
					</div>
					<div class="flex flex-wrap gap-2">
						<Button.Root
							variant="outline"
							disabled={!model.configDirty || model.agentBusy || model.isSavingPassword}
							onclick={model.resetConfig}>Discard changes</Button.Root
						>
						<Button.Root
							loading={model.isSavingConfig}
							disabled={!model.configDirty || model.agentBusy || model.isSavingPassword}
							onclick={model.saveAgentConfig}>Save changes</Button.Root
						>
					</div>
				</div>
				<details class="text-xs text-muted-foreground">
					<summary class="cursor-pointer">Where settings are saved</summary>
					<p class="mt-2 break-all">{model.agentConfig?.env_path}</p>
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
	bind:open={model.ui.showRestartDialog}
	onOpenChange={(open) => {
		if (model.isRestarting && !open) model.ui.showRestartDialog = true;
	}}
>
	<Dialog.Content class="sm:max-w-115" showCloseButton={!model.isRestarting}>
		<Dialog.Header>
			<Dialog.Title>Restart Watcher Service</Dialog.Title>
			<Dialog.Description>
				Restart watcher service now? This may temporarily disconnect the dashboard.
			</Dialog.Description>
		</Dialog.Header>
		<RequestError message={model.ui.maintenanceError} />
		<Dialog.Footer>
			<Button.Root
				variant="outline"
				type="button"
				disabled={model.isRestarting}
				onclick={() => (model.ui.showRestartDialog = false)}
			>
				Cancel
			</Button.Root>
			<Button.Root
				type="button"
				disabled={model.isRestarting}
				loading={model.isRestarting}
				onclick={model.restartWatcherService}>Restart</Button.Root
			>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<Dialog.Root
	bind:open={model.ui.showUpdateDialog}
	onOpenChange={(open) => {
		if (model.isUpdating && !open) model.ui.showUpdateDialog = true;
	}}
>
	<Dialog.Content class="sm:max-w-115" showCloseButton={!model.isUpdating}>
		<Dialog.Header>
			<Dialog.Title>Update Watcher</Dialog.Title>
			<Dialog.Description>
				Update Watcher now? The service will be restarted automatically.
			</Dialog.Description>
		</Dialog.Header>
		<RequestError message={model.ui.maintenanceError} />
		<Dialog.Footer>
			<Button.Root
				variant="outline"
				type="button"
				onclick={() => (model.ui.showUpdateDialog = false)}
				disabled={model.isUpdating}
			>
				Cancel
			</Button.Root>
			<Button.Root
				type="button"
				class="bg-blue-600 text-white hover:bg-blue-700"
				onclick={model.performUpdate}
				disabled={model.isUpdating}
			>
				{model.isUpdating ? 'Updating...' : 'Update & Restart'}
			</Button.Root>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
