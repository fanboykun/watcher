<script lang="ts">
	import ServiceDrafts from '$lib/features/watchers/candidates/service-drafts.svelte';
	import ApprovalReview from '$lib/features/watchers/candidates/approval-review.svelte';
	import TargetSelector from '$lib/features/watchers/candidates/target-selector.svelte';
	import { keys } from '$lib/query/options';

	import UnsavedChanges from '$lib/components/console/unsaved-changes.svelte';
	import RequestError from '$lib/components/request-error.svelte';
	import { type Watcher } from '$lib/api';

	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';

	import RequestLoading from '$lib/components/request-loading.svelte';
	import { Check, Save } from '@lucide/svelte';
	import { page } from '$app/state';
	import { createCandidate } from '$lib/features/watchers/candidates/candidate-controller.svelte';
	let {
		watcher,
		busy = false,
		onApprove,
		onDiscard,
		onRefreshWatcher
	}: {
		watcher: Watcher;
		busy?: boolean;
		onApprove?: (version: string, catalogId: number) => Promise<void>;
		onDiscard?: () => Promise<void>;
		onRefreshWatcher?: () => Promise<void>;
	} = $props();
	const model = createCandidate(() => ({ watcher, busy, onApprove, onDiscard, onRefreshWatcher }));
</script>

<UnsavedChanges dirty={model.hasAnyChanges} label="candidate configuration" />
<fieldset
	disabled={model.busy ||
		model.saving ||
		model.approving ||
		model.discarding ||
		model.deletingRevision}
	class="min-w-0 space-y-4"
>
	<!-- Top Alert / Callouts -->
	{#if !model.ui.approvalOpen}<RequestError message={model.ui.error} />{/if}

	{#if model.ui.saveSuccess}
		<div
			class="flex items-center rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-4 text-sm text-emerald-400"
		>
			<Check class="mr-2 h-4 w-4 shrink-0" />
			<span>{model.ui.saveSuccess}</span>
		</div>
	{/if}

	{#if model.watcher.status === 'pending_approval' || model.retainedCatalog}
		<div class="section-toolbar rounded-lg border border-amber-500/30 bg-amber-500/10 p-4">
			<div class="min-w-0">
				<p class="text-sm font-medium">
					Review <span class="font-mono">{model.watcher.pending_version}</span>
				</p>
				<p class="mt-1 text-xs text-muted-foreground">
					Approval saves your edits and restarts the services.
				</p>
				{#if !model.loading && model.ui.loadedSelection !== model.pendingSelection}
					<p class="mt-2 text-sm text-amber-400">
						Load the pending candidate using its version below before approving.
						{#if model.hasAnyChanges}Save your edits first to keep them for the displayed target.{/if}
					</p>
				{/if}
				{#if model.retainedCatalog && model.watcher.last_error}
					<p class="mt-2 text-sm text-destructive">{model.watcher.last_error}</p>
					<p class="mt-1 text-xs text-muted-foreground">
						Retry this download, or discard the candidate to select another release. The download
						stays in Versions.
					</p>
				{/if}
			</div>
			<div class="page-actions">
				<Button.Root
					variant="outline"
					size="sm"
					disabled={model.busy || model.discarding || model.approving || model.saving}
					onclick={model.handleDiscard}>Discard</Button.Root
				>
			</div>
		</div>
	{:else}
		<p class="text-sm text-muted-foreground">
			Stage configuration for the next deployment.{#if model.watcher.auto_deploy && !model.watcher.intercept_next_release}
				Enable Intercept next to review before deploying.{/if}
		</p>
	{/if}

	<!-- Version Selector and Actions Bar -->
	<TargetSelector {model} />

	<div class="mt-6 mb-3">
		<p class="eyebrow mb-2">02 / Configuration</p>
		<h2 class="text-sm font-semibold">Review service files</h2>
		<p class="mt-1 text-xs text-muted-foreground">
			Edit a file or compare its exact contents with the active version. Comments, duplicate keys,
			quoting, and line endings stay intact.
		</p>
	</div>
	{#if model.loading}<RequestLoading label="Loading candidate configuration…" />
	{:else if model.ui.services.length === 0}<p class="console-empty text-sm text-muted-foreground">
			No services configured for this watcher.
		</p>
	{:else}
		<ServiceDrafts {model} />
	{/if}
	{#if model.watcher.status === 'pending_approval' || model.retainedCatalog}
		<section class="review-step" aria-label="Deployment review">
			<p class="eyebrow mb-2">03 / Deploy</p>
			<div class="section-toolbar">
				<div>
					<h3>Ready for a final review?</h3>
					<p class="mt-1 text-xs leading-relaxed text-muted-foreground">
						Confirm the release, affected services, and configuration before anything is restarted.
					</p>
				</div>
				<Button.Root
					size="sm"
					onclick={model.openApproval}
					disabled={model.busy ||
						model.saving ||
						model.approving ||
						model.loading ||
						model.problems.length > 0 ||
						model.ui.loadedSelection !== model.pendingSelection ||
						model.ui.loadedTarget !== model.watcher.pending_version ||
						model.effectiveTarget !== model.watcher.pending_version}>Review deployment</Button.Root
				>
			</div>
		</section>
	{/if}
</fieldset>

<Dialog.Root
	open={model.ui.requestedTarget !== null}
	onOpenChange={(open) => {
		if (!open) model.ui.requestedTarget = null;
	}}
	><Dialog.Content
		><Dialog.Header
			><Dialog.Title>Switch configuration target?</Dialog.Title><Dialog.Description
				>Your edits belong to {model.ui.loadedTarget}. Save them before switching, or discard the
				unsaved edits to load {model.ui.requestedTarget?.target}.</Dialog.Description
			></Dialog.Header
		><Dialog.Footer
			><Button.Root variant="outline" onclick={() => (model.ui.requestedTarget = null)}
				>Keep editing</Button.Root
			><Button.Root
				variant="destructive"
				onclick={async () => {
					const next = model.ui.requestedTarget;
					model.ui.requestedTarget = null;
					if (next) await model.selectTarget(next.target, next.custom, true);
				}}>Discard and switch</Button.Root
			></Dialog.Footer
		></Dialog.Content
	></Dialog.Root
>
<Dialog.Root
	open={model.ui.resetSelection !== null}
	onOpenChange={(open) => {
		if (!open) model.ui.resetSelection = null;
	}}
	><Dialog.Content
		><Dialog.Header
			><Dialog.Title>Reset this service draft?</Dialog.Title><Dialog.Description
				>Unsaved environment and managed-file edits for {model.ui.resetSelection?.serviceName} will be
				replaced with the last loaded draft.</Dialog.Description
			></Dialog.Header
		><Dialog.Footer
			><Button.Root variant="outline" onclick={() => (model.ui.resetSelection = null)}
				>Keep editing</Button.Root
			><Button.Root
				variant="destructive"
				onclick={() => {
					if (model.ui.resetSelection) model.resetCandidate(model.ui.resetSelection);
					model.ui.resetSelection = null;
				}}>Reset draft</Button.Root
			></Dialog.Footer
		></Dialog.Content
	></Dialog.Root
>
<ApprovalReview {model} />
<Dialog.Root
	open={model.ui.confirmDelete !== null}
	onOpenChange={(open) => {
		if (!open && !model.deletingRevision) model.ui.confirmDelete = null;
	}}
>
	<Dialog.Content showCloseButton={!model.deletingRevision}>
		<Dialog.Header>
			<Dialog.Title>Delete saved draft?</Dialog.Title>
			<Dialog.Description
				>Delete the environment and managed file draft for {model.ui.confirmDelete?.serviceName} at {model
					.ui.confirmDelete?.target}? Downloaded releases and historical snapshots stay unchanged.</Dialog.Description
			>
		</Dialog.Header>
		<RequestError message={model.ui.deleteError} />
		<Dialog.Footer>
			<Button.Root
				variant="outline"
				disabled={model.deletingRevision}
				onclick={() => (model.ui.confirmDelete = null)}>Cancel</Button.Root
			>
			<Button.Root
				variant="destructive"
				disabled={model.busy ||
					model.saving ||
					model.approving ||
					model.discarding ||
					model.hasAnyChanges ||
					model.deletingRevision}
				onclick={model.deleteSavedRevision}
				>{model.deletingRevision ? 'Deleting…' : 'Delete draft'}</Button.Root
			>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
