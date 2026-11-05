<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { createQuery } from '@tanstack/svelte-query';
	import { queries } from '$lib/query/options';
	import * as Button from '$lib/components/ui/button';
	import { ArrowLeft, RotateCcw, Save } from '@lucide/svelte';
	import RequestLoading from '$lib/components/request-loading.svelte';
	import RequestError from '$lib/components/request-error.svelte';
	import ErrorScreen from '$lib/components/errors/error-screen.svelte';
	import UnsavedChanges from '$lib/components/console/unsaved-changes.svelte';
	import RollbackDialog from '$lib/features/watchers/components/rollback-dialog.svelte';
	import ReleaseNotes from '$lib/features/watchers/releases/release-notes.svelte';
	import VersionConfigServices from '$lib/features/watchers/version-config/version-config-services.svelte';
	import { createVersionConfig } from '$lib/features/watchers/version-config/version-config-controller.svelte';

	const watcherId = $derived(Number(page.params.id));
	const version = $derived(page.params.version || '');
	const watcherQuery = createQuery(() => queries.watcher(watcherId));
	const model = createVersionConfig(() => ({ watcherId, version }));
	const config = $derived(model.configQuery.data ?? null);

	function sourceLabel(source: string) {
		if (source === 'deployment') return 'Deployment';
		if (source === 'rollback_edit') return 'Rollback with edited configuration';
		if (source === 'active_config_update') return 'Active configuration change';
		if (source === 'current_version_migration') return 'Current version migration';
		return 'Historical snapshot';
	}
</script>

<UnsavedChanges dirty={model.dirty} label="version configuration" />
<div class="space-y-6">
	<div class="flex items-start gap-4">
		<Button.Root
			href={resolve('/watchers/[id]', { id: String(watcherId) }) + '?tab=versions'}
			variant="ghost"
			size="icon"
			aria-label="Back to versions"
		>
			<ArrowLeft class="h-4 w-4" />
		</Button.Root>
		<div class="min-w-0 flex-1">
			<h1 class="text-2xl font-bold tracking-tight break-all">Configuration: {version}</h1>
			<p class="text-sm text-muted-foreground">
				{#if model.current}
					This version is running. Change its configuration from the watcher's services.
				{:else if model.blocked}
					This version is blocked, so it cannot be rolled back to.
				{:else}
					Edit the configuration to roll back with. The configuration this version last ran with
					stays unchanged until a rollback with your edits succeeds.
				{/if}
			</p>
		</div>
		{#if !model.readonly && config}
			<div class="page-actions">
				<Button.Root
					variant="outline"
					size="sm"
					disabled={model.saving || model.rollingBack}
					onclick={model.resetToSnapshot}>Reset to saved configuration</Button.Root
				>
				<Button.Root
					size="sm"
					loading={model.saving}
					disabled={!model.dirty || model.saving || model.problems.length > 0}
					onclick={model.saveDrafts}><Save />Save edits</Button.Root
				>
				<Button.Root
					size="sm"
					class="bg-amber-600 text-white hover:bg-amber-700"
					disabled={model.dirty || model.saving || model.rollingBack}
					title={model.dirty ? 'Save your edits before rolling back' : undefined}
					onclick={() => {
						model.ui.useDraft = true;
						model.ui.rollbackOpen = true;
					}}><RotateCcw />Roll back to {version}…</Button.Root
				>
			</div>
		{/if}
	</div>

	<ReleaseNotes mark={model.mark} />
	{#if !model.ui.rollbackOpen}<RequestError message={model.ui.error} />{/if}
	{#if model.ui.notice}<p role="status" class="text-sm text-emerald-500">
			{model.ui.notice}
		</p>{/if}
	{#each model.problems as problem (problem)}<p class="text-sm text-amber-400">{problem}</p>{/each}

	{#if model.configQuery.isPending}
		<RequestLoading label="Loading version configuration..." />
	{:else if config}
		<p class="text-xs text-muted-foreground">
			Last ran with this configuration: captured {new Date(config.captured_at).toLocaleString()} · {sourceLabel(
				config.source
			)}
		</p>
		<VersionConfigServices {model} {version} installDir={watcherQuery.data?.install_dir ?? ''} />
	{:else}
		<ErrorScreen error={model.configQuery.error} onRetry={() => model.configQuery.refetch()} />
	{/if}
</div>

<RollbackDialog
	bind:open={model.ui.rollbackOpen}
	pending={model.rollingBack}
	errorMessage={model.ui.rollbackOpen ? model.ui.error : ''}
	rollbackTargetVersion={version}
	bind:rollbackReportGitHub={model.ui.reportGitHub}
	hasDraft={model.hasDraft}
	bind:useDraft={model.ui.useDraft}
	onRollback={model.startRollback}
/>
