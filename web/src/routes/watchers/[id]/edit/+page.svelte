<script lang="ts">
	import WebhooksSection from '$lib/features/watchers/settings/webhooks-section.svelte';
	import ServicesSection from '$lib/features/watchers/settings/services-section.svelte';
	import DeploymentSection from '$lib/features/watchers/settings/deployment-section.svelte';
	import SourceSection from '$lib/features/watchers/settings/source-section.svelte';

	import * as Tabs from '$lib/components/ui/tabs';
	import RequestLoading from '$lib/components/request-loading.svelte';
	import RequestError from '$lib/components/request-error.svelte';
	import { message } from '$lib/query/client';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { type Service, type Watcher } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { ArrowLeft, CheckCircle2, Save, Link as LinkIcon } from '@lucide/svelte';

	import { createWatcherSettings } from '$lib/features/watchers/settings/settings-controller.svelte';
	const model = createWatcherSettings();
</script>

<div class="space-y-6">
	<header class="page-header">
		<div class="page-identity">
			<Button.Root
				href={resolve(`/watchers/${model.id}`)}
				variant="ghost"
				size="icon"
				aria-label="Back to watcher"><ArrowLeft /></Button.Root
			>
			<div class="flex-1">
				<h1 class="text-2xl font-bold tracking-tight">
					{model.watcher ? `Edit ${model.watcher.name}` : 'Edit Watcher'}
				</h1>
				{#if model.watcher && model.watcher.service_name !== model.watcher.name}
					<p class="font-mono text-sm text-muted-foreground">{model.watcher.service_name}</p>
				{/if}
			</div>
		</div>
		<Button.Root href={resolve(`/watchers/${model.id}?tab=webhooks`)} variant="outline" size="sm"
			><LinkIcon />Webhook history</Button.Root
		>
	</header>

	{#if !model.ui.showConfirmDialog}<RequestError
			message={model.ui.error || message(model.detail.error)}
		/>{/if}

	{#if model.ui.success}
		<div
			class="flex items-center rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-4 text-sm text-emerald-400"
		>
			<CheckCircle2 class="mr-2 h-4 w-4 shrink-0" />
			<span>{model.ui.success}</span>
		</div>
	{/if}

	{#if model.loading}<RequestLoading label="Loading watcher settings…" />{:else if model.watcher}
		<Tabs.Root bind:value={model.ui.activeSection}>
			<Tabs.List
				><Tabs.Trigger value="source">Source</Tabs.Trigger><Tabs.Trigger value="deployment"
					>Deployment</Tabs.Trigger
				><Tabs.Trigger value="health">Health checks</Tabs.Trigger><Tabs.Trigger value="webhooks"
					>Webhooks</Tabs.Trigger
				><Tabs.Trigger value="services">Services</Tabs.Trigger></Tabs.List
			>
			<form
				id="watcher-settings"
				onsubmit={(event) => {
					event.preventDefault();
					void model.saveEdit();
				}}
			>
				<fieldset disabled={model.saving || model.sendingTest || model.confirming}>
					<SourceSection {model} />
					<DeploymentSection {model} />
					<Tabs.Content value="health"
						><Card.Root
							><Card.Header
								><Card.Title>Health checks</Card.Title><Card.Description
									>Services can override this watcher-level health URL.</Card.Description
								></Card.Header
							><Card.Content class="space-y-4"
								><div class="flex flex-wrap items-center gap-2 py-1">
									<Checkbox
										id="editHcEnabled"
										bind:checked={model.ui.editHcEnabled}
										aria-describedby="editHcEnabled-help"
									/>

									<Label for="editHcEnabled">Enable health checks</Label>
									<p id="editHcEnabled-help" class="w-full text-xs text-muted-foreground">
										Check service health after deployment. A failed check triggers rollback.
									</p>
								</div>
								<div class="space-y-2">
									<Label for="editHcURL">Health Check URL</Label>
									<Input
										id="editHcURL"
										bind:value={model.ui.editHcURL}
										aria-describedby="editHcURL-help"
									/>
									<p id="editHcURL-help" class="text-xs text-muted-foreground">
										Default health endpoint for services in this watcher. A service-specific URL
										takes precedence.
									</p>
								</div></Card.Content
							></Card.Root
						></Tabs.Content
					>
					<WebhooksSection {model} />
				</fieldset>
			</form>
			<ServicesSection {model} />
		</Tabs.Root>
		<div
			class="section-toolbar sticky bottom-3 rounded-lg border border-border bg-background p-4 shadow-lg"
		>
			<p class="text-sm text-muted-foreground">
				Saves source, deployment, health and webhook settings together. Service changes are saved
				separately.
			</p>
			<Button.Root
				type="submit"
				form="watcher-settings"
				loading={model.saving}
				disabled={model.saving || model.sendingTest || model.confirming}
				><Save />Save changes</Button.Root
			>
		</div>
	{/if}
</div>

<Dialog.Root
	bind:open={model.ui.showConfirmDialog}
	onOpenChange={(open) => {
		if (model.confirming && !open) model.ui.showConfirmDialog = true;
	}}
>
	<Dialog.Content class="sm:max-w-115" showCloseButton={!model.confirming}>
		<Dialog.Header>
			<Dialog.Title>{model.ui.confirmTitle}</Dialog.Title>
			<Dialog.Description>{model.ui.confirmDescription}</Dialog.Description>
		</Dialog.Header>
		<RequestError message={model.ui.error || message(model.detail.error)} />
		<Dialog.Footer>
			<Button.Root
				variant="outline"
				type="button"
				disabled={model.confirming}
				onclick={() => (model.ui.showConfirmDialog = false)}
			>
				Cancel
			</Button.Root>
			<Button.Root
				type="button"
				class={model.ui.confirmActionClass}
				onclick={model.runConfirmAction}
				disabled={model.confirming}
			>
				{model.confirming ? 'Working...' : model.ui.confirmActionLabel}
			</Button.Root>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
