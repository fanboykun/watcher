<script lang="ts">
	import DetailTabs from '$lib/features/watchers/detail/detail-tabs.svelte';
	import DetailHeader from '$lib/features/watchers/detail/detail-header.svelte';

	import RequestError from '$lib/components/request-error.svelte';
	import ErrorScreen from '$lib/components/errors/error-screen.svelte';
	import { message } from '$lib/query/client';

	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Zap, AlertCircle } from '@lucide/svelte';
	import { resolve } from '$app/paths';
	import { goto } from '$app/navigation';

	import WatcherPollingStatus from '$lib/components/watcher-polling-status.svelte';

	import RollbackDialog from '$lib/features/watchers/components/rollback-dialog.svelte';
	import ConfirmationDialog from '$lib/features/watchers/components/confirmation-dialog.svelte';
	import RequestLoading from '$lib/components/request-loading.svelte';
	import { createWatcherDetail } from '$lib/features/watchers/detail/detail-controller.svelte';
	import { isGitHubSource } from '$lib/features/watchers/presentation';
	const model = createWatcherDetail();
</script>

{#if !model.loading && !model.watcher}
	<ErrorScreen error={model.detail.error} onRetry={model.loadWatcher} />
{:else}
	<div class="space-y-6">
		<DetailHeader {model} />

		{#if !model.ui.showConfirmDialog && !model.ui.showRollbackDialog}<RequestError
				message={model.ui.error || message(model.detail.error)}
			/>{/if}
		<RequestError
			message={model.dataError}
			title="Details could not be refreshed"
			onRetry={model.refreshDetails}
		/>

		{#if model.ui.triggerMsg}
			<div
				class="flex items-center rounded-lg border border-blue-500/30 bg-blue-500/10 p-4 text-sm text-blue-400"
			>
				<Zap class="mr-2 h-4 w-4 shrink-0" />
				<span>{model.ui.triggerMsg}</span>
			</div>
		{/if}

		{#if model.loading}
			<RequestLoading label="Loading watcher details…" />
		{:else if model.watcher}
			<div class="rounded-xl border border-border bg-card p-4">
				<WatcherPollingStatus watcher={model.watcher} />
			</div>
			{#if model.reviewableCandidate && model.ui.activeTab !== 'candidates'}
				<div class="section-toolbar rounded-lg border border-amber-500/30 bg-amber-500/10 p-4">
					<div class="min-w-0">
						<p class="text-sm font-medium">
							Release <span class="font-mono">{model.watcher.pending_version}</span> awaits review
						</p>
						<p class="mt-1 text-xs text-muted-foreground">
							Review its configuration to deploy or discard this candidate.
						</p>
					</div>
					<Button.Root
						size="sm"
						disabled={model.actionBusy}
						onclick={() => {
							model.ui.activeTab = 'candidates';
							return goto(resolve(`/watchers/[id]?tab=candidates`, { id: String(model.id) }), {
								replaceState: true,
								noScroll: true
							});
						}}>Review candidate</Button.Root
					>
				</div>
			{/if}

			{#if model.hasActiveRollbackPin(model.watcher)}
				<div
					class="section-toolbar mb-4 rounded-lg border border-amber-500/30 bg-amber-500/10 p-4 text-sm text-amber-500"
				>
					<div class="flex items-center gap-2">
						<AlertCircle class="h-4 w-4" />
						<span>
							<strong>Manual rollback pin is active.</strong>
							Current is <code>{model.watcher.current_version || 'unknown'}</code>; auto-update
							ignores versions
							<code>&lt;= {model.watcher.max_ignored_version}</code>.
						</span>
					</div>
					<Button.Root
						variant="outline"
						size="sm"
						class="border-amber-500/30 hover:bg-amber-500/20"
						disabled={model.actionBusy}
						loading={model.pendingAction === 'resume'}
						onclick={() => model.runPending('resume', model.resumeAutoDeploy)}
					>
						Resume Updates
					</Button.Root>
				</div>
			{/if}

			<DetailTabs {model} />
		{/if}
	</div>
{/if}

<!-- Rollback Dialog -->
<RollbackDialog
	errorMessage={model.ui.error}
	onRollback={(version, report) =>
		model.runPending('rollback', () => model.rollback(version, report))}
	pending={model.pendingAction === 'rollback'}
	bind:open={model.ui.showRollbackDialog}
	rollbackTargetVersion={model.ui.rollbackTargetVersion}
	bind:rollbackReportGitHub={model.ui.rollbackReportGitHub}
	canReportGitHub={isGitHubSource(model.watcher?.source_type)}
/>

<!-- Confirm Action Dialog -->
<ConfirmationDialog
	errorMessage={model.ui.error}
	bind:open={model.ui.showConfirmDialog}
	bind:confirmTitle={model.ui.confirmTitle}
	bind:confirmDescription={model.ui.confirmDescription}
	confirming={model.confirming}
	confirmActionClass={model.ui.confirmActionClass}
	confirmActionLabel={model.ui.confirmActionLabel}
	onConfirm={model.runConfirmAction}
/>
