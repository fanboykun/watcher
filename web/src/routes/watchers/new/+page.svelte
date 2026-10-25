<script lang="ts">
	import WebhooksStep from '$lib/features/watchers/setup/webhooks-step.svelte';
	import ServicesStep from '$lib/features/watchers/setup/services-step.svelte';
	import WatcherStep from '$lib/features/watchers/setup/watcher-step.svelte';
	import { message } from '$lib/query/client';

	import PageHeading from '$lib/components/console/page-heading.svelte';
	import WizardProgress from '$lib/components/console/wizard-progress.svelte';
	import RequestError from '$lib/components/request-error.svelte';
	import { resolve } from '$app/paths';

	import * as Button from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';

	import { ArrowLeft, ArrowRight, Check } from '@lucide/svelte';
	import { createWatcherSetup } from '$lib/features/watchers/setup/setup-controller.svelte';
	const model = createWatcherSetup();
</script>

<svelte:head>
	<title>Add Watcher | Watcher</title>
</svelte:head>

<div class="space-y-6">
	<PageHeading
		eyebrow="Workspace / New watcher"
		title="Connect. Configure. Deploy."
		description="Bring a repository into your workspace. We'll take it one step at a time."
	>
		{#snippet actions()}<Button.Root href={resolve('/watchers')} variant="outline" size="sm"
				><ArrowLeft class="size-3.5" />Back to watchers</Button.Root
			>{/snippet}
	</PageHeading>
	<WizardProgress
		steps={model.steps}
		current={model.ui.currentStep}
		onselect={(step) => model.goToStep(step as typeof model.ui.currentStep)}
		disabled={model.inspecting || model.creating}
	/>

	<RequestError message={model.ui.error} />

	<Card.Root class="border-border bg-card">
		<Card.Content class="space-y-5 p-6">
			{#if model.ui.currentStep === 1}
				<div class="space-y-4">
					<div class="grid gap-4 md:grid-cols-2">
						<div class="space-y-2 md:col-span-2">
							<Label for="repoURL">GitHub Repository or metadata URL</Label>
							<Input
								id="repoURL"
								bind:value={model.ui.repoURL}
								placeholder="https://github.com/org/repo"
							/>
						</div>
						<div class="space-y-2">
							<Label for="releaseRef">Release Ref</Label>
							<Input
								id="releaseRef"
								bind:value={model.ui.releaseRef}
								placeholder="latest or v1.2.3"
							/>
						</div>
						<div class="space-y-2">
							<Label for="inspectGitHubToken">Inspect Token Override</Label>
							<Input
								id="inspectGitHubToken"
								type="password"
								bind:value={model.ui.inspectGitHubToken}
								disabled={!model.ui.useInspectGitHubToken}
								placeholder="Use only for inspect"
							/>
							<div class="mt-2 flex items-center gap-2">
								<Checkbox
									id="useInspectGitHubToken"
									bind:checked={model.ui.useInspectGitHubToken}
								/>
								<Label for="useInspectGitHubToken">Use custom token for inspect</Label>
							</div>
						</div>
					</div>

					<div
						class="rounded-lg border border-border/70 bg-muted/20 p-4 text-sm text-muted-foreground"
					>
						Watcher inspects `version.json` first for the chosen ref and falls back to legacy repo
						asset discovery when needed.
					</div>

					{#if model.ui.inspectResult}
						<div class="rounded-lg border border-border/70 bg-muted/20 p-4">
							<div class="grid gap-3 md:grid-cols-3">
								<div>
									<p class="text-xs text-muted-foreground">Detected version</p>
									<p class="mt-1 font-medium">
										{model.ui.inspectResult.latest_version || 'Unknown'}
									</p>
								</div>
								<div>
									<p class="text-xs text-muted-foreground">Source</p>
									<p class="mt-1 font-medium">{model.ui.inspectResult.source}</p>
								</div>
								<div>
									<p class="text-xs text-muted-foreground">Assets</p>
									<p class="mt-1 font-medium">{model.ui.inspectResult.assets.length}</p>
								</div>
							</div>
						</div>
					{/if}
				</div>
			{:else if model.ui.currentStep === 2}
				<WatcherStep {model} />
			{:else if model.ui.currentStep === 3}
				<ServicesStep {model} />
			{:else}
				<WebhooksStep {model} />
			{/if}
		</Card.Content>
	</Card.Root>

	<div class="flex items-center justify-between gap-3">
		{#if model.ui.currentStep === 1}
			<Button.Root href={resolve('/watchers')} variant="outline">Cancel</Button.Root>
		{:else}
			<Button.Root
				variant="outline"
				type="button"
				onclick={model.goBack}
				disabled={model.creating || model.inspecting}>Back</Button.Root
			>
		{/if}

		{#if model.ui.currentStep < 4}
			<Button.Root type="button" onclick={model.goNext} disabled={model.inspecting}>
				{#if model.ui.currentStep === 1 && model.inspecting}
					Inspecting...
				{:else}
					Next <ArrowRight class="ml-2 h-4 w-4" />
				{/if}
			</Button.Root>
		{:else}
			<Button.Root type="button" onclick={model.createWatcher} disabled={model.creating}>
				{#if model.creating}
					Creating...
				{:else}
					<Check class="mr-2 h-4 w-4" /> Save Watcher & Services
				{/if}
			</Button.Root>
		{/if}
	</div>
</div>
