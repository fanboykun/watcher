<script lang="ts">
	import RequestError from '$lib/components/request-error.svelte';
	import { type Watcher } from '$lib/api';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import type { createCandidate } from '$lib/features/watchers/candidates/candidate-controller.svelte';
	let { model }: { model: ReturnType<typeof createCandidate> } = $props();
</script>

<Dialog.Root
	bind:open={model.ui.approvalOpen}
	onOpenChange={(open) => {
		if (model.approving && !open) model.ui.approvalOpen = true;
	}}
	><Dialog.Content
		class="sm:max-w-xl"
		showCloseButton={!model.approving}
		onCloseAutoFocus={(event) => {
			if (model.approvalOpener?.isConnected) {
				event.preventDefault();
				model.approvalOpener.focus({ preventScroll: true });
			}
		}}
		><Dialog.Header
			><Dialog.Title>Deploy reviewed release {model.ui.approvalVersion}?</Dialog.Title
			><Dialog.Description
				>Unsaved edits are saved to this candidate first. Watcher then deploys this exact release
				and restarts its services. Active configuration changes during activation.</Dialog.Description
			></Dialog.Header
		>
		<dl class="grid grid-cols-2 gap-4 rounded-md border border-border p-4 text-xs">
			<div>
				<dt class="text-muted-foreground">Release</dt>
				<dd class="mt-1 font-mono">{model.ui.approvalVersion}</dd>
			</div>
			<div>
				<dt class="text-muted-foreground">Artifact identity</dt>
				<dd class="mt-1 font-mono">{model.ui.approvalSelection}</dd>
			</div>
		</dl>
		<ul class="divide-y divide-border">
			{#each model.ui.services as svc (svc.serviceId)}<li class="py-3 text-xs">
					<p class="font-semibold">{svc.serviceName}</p>
					<p class="mt-1 text-muted-foreground">
						{svc.candidateEnv === svc.activeEnv
							? 'Environment matches active'
							: 'Environment changed'} · {svc.filesExplicit
							? `${svc.candidateFiles.length} managed files staged`
							: 'Managed files inherit active settings'}
					</p>
					{#if svc.filesExplicit}<p
							class="mt-1 font-mono text-[10px] break-all text-muted-foreground"
						>
							{svc.candidateFiles.map((file) => `${file.target}/${file.file_path}`).join(' · ') ||
								'Explicitly empty managed file set'}
						</p>{/if}
				</li>{/each}
		</ul>
		{#if model.ui.approvalSelection !== model.pendingSelection}<p
				role="alert"
				class="text-sm text-amber-300"
			>
				The selected release changed. Close this review and load the current candidate.
			</p>{/if}<RequestError message={model.ui.error} />
		<Dialog.Footer
			><Button.Root
				variant="outline"
				disabled={model.approving}
				onclick={() => (model.ui.approvalOpen = false)}>Back to configuration</Button.Root
			><Button.Root
				disabled={model.busy ||
					model.approving ||
					model.ui.approvalSelection !== model.pendingSelection}
				loading={model.approving}
				onclick={model.handleApprove}>Deploy reviewed release</Button.Root
			></Dialog.Footer
		>
	</Dialog.Content></Dialog.Root
>
