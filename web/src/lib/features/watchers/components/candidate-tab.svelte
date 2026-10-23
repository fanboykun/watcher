<script lang="ts">
	import ConfigWorkspace from '$lib/features/configuration/config-workspace.svelte';
	import UnsavedChanges from '$lib/components/console/unsaved-changes.svelte';
	import { configurationProblems } from '$lib/features/configuration/model';
	import RequestError from '$lib/components/request-error.svelte';
	import { api, type Watcher, type ServiceConfigFile } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import RequestLoading from '$lib/components/request-loading.svelte';
	import { Check, Layers, Pencil, RefreshCw, Save, Zap } from '@lucide/svelte';
	import { onDestroy, onMount, tick } from 'svelte';
	import { afterNavigate } from '$app/navigation';
	import { page } from '$app/state';

	interface EditableServiceCandidate {
		serviceId: number;
		serviceName: string;
		serviceType: string;
		activeEnv: string;
		candidateEnv: string;
		originalCandidateEnv: string;
		hasCandidateEnv: boolean;
		revisionTarget: string;
		activeFiles: ServiceConfigFile[];
		candidateFiles: ServiceConfigFile[];
		originalCandidateFiles: ServiceConfigFile[];
		filesExplicit: boolean;
		originalFilesExplicit: boolean;
	}

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

	let selectedTarget = $state('');
	let isCustom = $state(false);
	let customTarget = $state('');

	let services = $state<EditableServiceCandidate[]>([]);
	let loading = $state(true);
	let loadedTarget = $state('');
	let loadedSelection = $state('');
	let loadGeneration = 0;
	let saving = $state(false);
	let saveSuccess = $state('');
	let error = $state('');
	let approving = $state(false);
	let discarding = $state(false);
	let savedTargets = $state<string[]>([]);
	let requestedTarget = $state<{ target: string; custom: boolean } | null>(null);
	let resetSelection = $state<EditableServiceCandidate | null>(null);
	let approvalOpen = $state(false);
	let approvalSelection = $state('');
	let approvalVersion = $state('');
	let approvalOpener: HTMLElement | null = null;
	const problems = $derived(
		services.flatMap((svc) =>
			configurationProblems(
				watcher.services.find((service) => service.id === svc.serviceId)?.env_file || '',
				svc.candidateFiles
			)
		)
	);
	let deletingRevision = $state(false);
	let confirmDelete = $state<{ serviceId: number; serviceName: string; target: string } | null>(
		null
	);
	let deleteError = $state('');

	const pendingSelection = $derived(
		`${watcher.id}:${watcher.pending_catalog_id}:${watcher.pending_version}`
	);
	const activeTargetChoice = $derived(selectedTarget || watcher.pending_version || 'next');
	// Typing a proposed version must not retarget an already loaded draft.
	const effectiveTarget = $derived(
		isCustom && selectedTarget === 'custom' ? '' : activeTargetChoice
	);
	const retainedCatalog = $derived(
		watcher.pending_catalog_id > 0 &&
			watcher.status !== 'approved' &&
			watcher.status !== 'deploying'
	);

	function copyFiles(files: ServiceConfigFile[] = []) {
		return files.map(({ file_path, target, content }) => ({ file_path, target, content }));
	}

	function filesEqual(a: ServiceConfigFile[], b: ServiceConfigFile[]) {
		return JSON.stringify(copyFiles(a)) === JSON.stringify(copyFiles(b));
	}

	function serviceChanged(service: EditableServiceCandidate) {
		return (
			service.candidateEnv !== service.originalCandidateEnv ||
			service.filesExplicit !== service.originalFilesExplicit ||
			!filesEqual(service.candidateFiles, service.originalCandidateFiles)
		);
	}

	function candidatePayload() {
		return services.map((service) => ({
			service_id: service.serviceId,
			env_content: service.candidateEnv,
			...(service.filesExplicit ? { config_files: copyFiles(service.candidateFiles) } : {})
		}));
	}

	const hasAnyChanges = $derived(services.some(serviceChanged));

	async function focusRequestedService() {
		const serviceID = Number(page.url.searchParams.get('service'));
		if (!Number.isSafeInteger(serviceID) || serviceID <= 0) return;
		await tick();
		document.getElementById(`candidate-service-${serviceID}`)?.scrollIntoView({ block: 'start' });
	}

	async function loadCandidateData(target: string) {
		if (!target) return;
		const generation = ++loadGeneration;
		const selection = pendingSelection;
		loadedTarget = '';
		loadedSelection = '';
		loading = true;
		error = '';
		try {
			const res = await api.getWatcherCandidate(watcher.id, target);
			if (generation !== loadGeneration || selection !== pendingSelection) return;
			loadedTarget = target;
			loadedSelection = selection;
			savedTargets = res.saved_targets ?? [];
			services = res.services.map((s) => ({
				serviceId: s.service_id,
				serviceName: s.service_name,
				serviceType: s.service_type,
				activeEnv: s.active_env,
				candidateEnv: s.candidate_env,
				originalCandidateEnv: s.candidate_env,
				hasCandidateEnv: s.has_candidate_env,
				revisionTarget: s.candidate_target ?? (s.has_candidate_env ? target : ''),
				activeFiles: copyFiles(s.active_config_files),
				candidateFiles: copyFiles(s.candidate_config_files ?? s.active_config_files),
				originalCandidateFiles: copyFiles(s.candidate_config_files ?? s.active_config_files),
				filesExplicit: s.has_candidate_config_files ?? false,
				originalFilesExplicit: s.has_candidate_config_files ?? false
			}));
			void focusRequestedService();
		} catch (e) {
			if (generation === loadGeneration)
				error = e instanceof Error ? e.message : 'Failed to load candidate configuration';
		} finally {
			if (generation === loadGeneration) loading = false;
		}
	}

	// Tabs keep this editor mounted. Catalog selection refreshes the watcher before
	// navigating here; routine watcher refreshes must preserve in-progress edits.
	let navigatedSelection = '';
	function syncPendingSelection() {
		if (navigatedSelection === pendingSelection) return;
		navigatedSelection = pendingSelection;
		++loadGeneration;
		loadedSelection = '';
		if (hasAnyChanges) {
			loading = false;
			error =
				'The pending release changed. Your edits are preserved; save them before loading the new candidate.';
			return;
		}
		isCustom = false;
		selectedTarget = watcher.pending_version || 'next';
		void loadCandidateData(selectedTarget);
	}
	onMount(syncPendingSelection);
	afterNavigate(() => {
		syncPendingSelection();
		void focusRequestedService();
	});
	onDestroy(() => {
		++loadGeneration;
	});

	async function selectTarget(target: string, custom = false, discard = false) {
		if (hasAnyChanges && !discard) {
			requestedTarget = { target, custom };
			return;
		}
		if (saving || approving || discarding || deletingRevision) return;
		isCustom = custom;
		selectedTarget = target;
		if (custom) {
			customTarget = target === 'custom' ? '' : target;
		}
		const toLoad = custom ? customTarget.trim() : target;
		if (toLoad) {
			await loadCandidateData(toLoad);
		}
	}

	async function handleCustomTargetSubmit() {
		if (customTarget.trim()) {
			await selectTarget(customTarget.trim(), true);
		}
	}

	async function saveAllCandidates() {
		if (busy || saving || approving || discarding || deletingRevision) return;
		if (loading || effectiveTarget !== loadedTarget) {
			error = 'Load the selected target before saving';
			return;
		}
		if (!effectiveTarget) {
			error = 'Please specify a target version';
			return;
		}
		if (problems.length) {
			error = problems.join(' ');
			return;
		}
		saving = true;
		error = '';
		saveSuccess = '';
		try {
			await api.updateWatcherCandidate(watcher.id, effectiveTarget, candidatePayload());
			for (const s of services) {
				s.originalCandidateEnv = s.candidateEnv;
				s.hasCandidateEnv = true;
				s.revisionTarget = effectiveTarget;
				s.originalCandidateFiles = copyFiles(s.candidateFiles);
				s.originalFilesExplicit = s.filesExplicit;
			}
			if (!savedTargets.includes(effectiveTarget))
				savedTargets = [...savedTargets, effectiveTarget].sort();
			saveSuccess = `Candidate configuration saved for version "${effectiveTarget}"!`;
			setTimeout(() => {
				saveSuccess = '';
			}, 4000);
			if (onRefreshWatcher) {
				await onRefreshWatcher();
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save candidate configuration';
		} finally {
			saving = false;
		}
	}

	function resetCandidate(service: EditableServiceCandidate) {
		service.candidateEnv = service.originalCandidateEnv;
		service.candidateFiles = copyFiles(service.originalCandidateFiles);
		service.filesExplicit = service.originalFilesExplicit;
	}

	async function deleteSavedRevision() {
		const selected = confirmDelete;
		if (!selected || busy || saving || approving || discarding || deletingRevision || hasAnyChanges)
			return;
		deletingRevision = true;
		deleteError = '';
		try {
			await api.deleteServiceConfigRevision(selected.serviceId, selected.target);
			confirmDelete = null;
			await loadCandidateData(effectiveTarget);
		} catch (cause) {
			deleteError = cause instanceof Error ? cause.message : 'Could not delete the saved draft';
		} finally {
			deletingRevision = false;
		}
	}

	async function handleApprove() {
		if (!onApprove || deletingRevision || busy || saving || approving || problems.length) return;
		if (approvalSelection !== pendingSelection) {
			error = 'The pending release changed. Close this review and load the new candidate.';
			return;
		}
		if (
			loading ||
			loadedTarget !== watcher.pending_version ||
			effectiveTarget !== watcher.pending_version ||
			loadedSelection !== pendingSelection
		) {
			error = 'Select and load the pending release before approving';
			return;
		}
		const version = watcher.pending_version;
		const selection = pendingSelection;
		const catalogId = watcher.pending_catalog_id;
		approving = true;
		error = '';
		try {
			// Save any unpersisted edits first
			if (hasAnyChanges) {
				await api.updateWatcherCandidate(watcher.id, effectiveTarget, candidatePayload());
				for (const service of services) {
					service.originalCandidateEnv = service.candidateEnv;
					service.originalCandidateFiles = copyFiles(service.candidateFiles);
					service.originalFilesExplicit = service.filesExplicit;
					service.hasCandidateEnv = true;
					service.revisionTarget = effectiveTarget;
				}
			}
			if (selection !== pendingSelection || loadedSelection !== selection) {
				error = 'The pending release changed. Load and review it before approving.';
				return;
			}
			await onApprove(version, catalogId);
			approvalOpen = false;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Approval failed';
		} finally {
			approving = false;
		}
	}

	async function handleDiscard() {
		if (!onDiscard || deletingRevision) return;
		discarding = true;
		error = '';
		try {
			await onDiscard();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Discard failed';
		} finally {
			discarding = false;
		}
	}

	function openApproval(event: MouseEvent) {
		approvalOpener = event.currentTarget as HTMLElement;
		approvalSelection = pendingSelection;
		approvalVersion = loadedTarget;
		error = '';
		approvalOpen = true;
	}
</script>

<UnsavedChanges dirty={hasAnyChanges} label="candidate configuration" />
<fieldset
	disabled={busy || saving || approving || discarding || deletingRevision}
	class="min-w-0 space-y-4"
>
	<!-- Top Alert / Callouts -->
	{#if !approvalOpen}<RequestError message={error} />{/if}

	{#if saveSuccess}
		<div
			class="flex items-center rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-4 text-sm text-emerald-400"
		>
			<Check class="mr-2 h-4 w-4 shrink-0" />
			<span>{saveSuccess}</span>
		</div>
	{/if}

	{#if watcher.status === 'pending_approval' || retainedCatalog}
		<div class="section-toolbar rounded-lg border border-amber-500/30 bg-amber-500/10 p-4">
			<div class="min-w-0">
				<p class="text-sm font-medium">
					Review <span class="font-mono">{watcher.pending_version}</span>
				</p>
				<p class="mt-1 text-xs text-muted-foreground">
					Approval saves your edits and restarts the services.
				</p>
				{#if !loading && loadedSelection !== pendingSelection}
					<p class="mt-2 text-sm text-amber-400">
						Load the pending candidate using its version below before approving.
						{#if hasAnyChanges}Save your edits first to keep them for the displayed target.{/if}
					</p>
				{/if}
				{#if retainedCatalog && watcher.last_error}
					<p class="mt-2 text-sm text-destructive">{watcher.last_error}</p>
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
					disabled={busy || discarding || approving || saving}
					onclick={handleDiscard}>Discard</Button.Root
				>
			</div>
		</div>
	{:else}
		<p class="text-sm text-muted-foreground">
			Stage configuration for the next deployment.{#if watcher.auto_deploy && !watcher.intercept_next_release}
				Enable Intercept next to review before deploying.{/if}
		</p>
	{/if}

	<!-- Version Selector and Actions Bar -->
	<Card.Root class="border-border bg-card">
		<Card.Header class="flex flex-col items-stretch pb-3">
			<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
				<div>
					<p class="eyebrow mb-2">01 / Release target</p>
					<Card.Title class="text-base font-semibold">Candidate configuration</Card.Title>
					<Card.Description class="text-xs">
						Select the pending release, the next release, or a saved version draft. Changes remain
						isolated from active settings.
					</Card.Description>
				</div>

				<div class="flex flex-wrap items-center gap-2">
					{#if hasAnyChanges}
						<span class="text-xs font-medium text-amber-400">Unsaved edits</span>
					{/if}
					<Button.Root
						variant="default"
						size="sm"
						class="h-8"
						onclick={saveAllCandidates}
						disabled={busy ||
							saving ||
							approving ||
							discarding ||
							loading ||
							!effectiveTarget ||
							loadedTarget !== effectiveTarget}
					>
						{#if saving}
							<RefreshCw class="mr-1.5 h-3.5 w-3.5 animate-spin" /> Saving...
						{:else}
							<Save class="mr-1.5 h-3.5 w-3.5" /> Save candidate
						{/if}
					</Button.Root>
				</div>
			</div>

			<!-- Quick Target Selector Pills -->
			<div class="mt-3 flex flex-wrap items-center gap-2">
				{#if watcher.pending_version}
					<Button.Root
						variant="ghost"
						size="sm"
						type="button"
						class={`inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-semibold transition ${
							!isCustom && activeTargetChoice === watcher.pending_version
								? 'bg-primary text-white shadow-sm'
								: 'border border-primary/40 bg-primary/10 text-primary hover:bg-primary/20'
						}`}
						onclick={() => selectTarget(watcher.pending_version)}
					>
						<Zap class="h-3 w-3" />
						{watcher.pending_version}
						<span class="py-0.2 rounded bg-black/20 px-1 text-[10px] tracking-wider uppercase">
							Candidate
						</span>
					</Button.Root>
				{/if}

				<Button.Root
					variant="ghost"
					size="sm"
					type="button"
					class={`inline-flex items-center gap-1 rounded-full px-3 py-1 text-xs font-medium transition ${
						!isCustom && activeTargetChoice === 'next'
							? 'bg-primary text-primary-foreground shadow-sm hover:bg-primary/90'
							: 'border border-border bg-muted/40 text-muted-foreground hover:bg-muted'
					}`}
					onclick={() => selectTarget('next')}
				>
					<Layers class="h-3 w-3" />
					Next release
				</Button.Root>

				<Button.Root
					variant="ghost"
					size="sm"
					type="button"
					class={`inline-flex items-center gap-1 rounded-full px-3 py-1 text-xs font-medium transition ${
						isCustom
							? 'bg-primary text-primary-foreground shadow-sm hover:bg-primary/90'
							: 'border border-border bg-muted/40 text-muted-foreground hover:bg-muted'
					}`}
					onclick={() => selectTarget(customTarget || 'custom', true)}
				>
					<Pencil class="h-3 w-3" />
					Specific version
				</Button.Root>

				{#if isCustom}
					<div class="flex items-center gap-1.5 pl-1">
						<Input
							bind:value={customTarget}
							aria-label="Candidate target version"
							placeholder="e.g. v2.0.0"
							class="h-7 w-36 font-mono text-xs"
							onkeydown={(e) => {
								if (e.key === 'Enter') handleCustomTargetSubmit();
							}}
						/>
						<Button.Root
							variant="outline"
							size="sm"
							class="h-7 px-2 text-xs"
							onclick={handleCustomTargetSubmit}
						>
							Load
						</Button.Root>
					</div>
				{/if}
			</div>
			{#if isCustom && customTarget.trim() !== effectiveTarget}<p
					class="mt-3 text-xs text-muted-foreground"
				>
					Load this version to switch drafts.{#if loadedTarget}
						Current draft: <span class="font-mono">{loadedTarget}</span>.{/if}
				</p>{/if}
			{#if savedTargets.some((target) => target !== 'next' && target !== watcher.pending_version)}
				<div class="mt-3 flex flex-wrap items-center gap-2" aria-label="Saved candidate targets">
					<span class="text-xs text-muted-foreground">Saved drafts:</span>
					{#each savedTargets.filter((target) => target !== 'next' && target !== watcher.pending_version) as target (target)}
						<Button.Root
							variant="outline"
							size="sm"
							type="button"
							onclick={() => selectTarget(target)}>{target}</Button.Root
						>
					{/each}
				</div>
			{/if}
		</Card.Header>
	</Card.Root>

	<div class="mt-6 mb-3">
		<p class="eyebrow mb-2">02 / Configuration</p>
		<h2 class="text-sm font-semibold">Review service files</h2>
		<p class="mt-1 text-xs text-muted-foreground">
			Edit a file or compare its exact contents with the active version. Comments, duplicate keys,
			quoting, and line endings stay intact.
		</p>
	</div>
	{#if loading}<RequestLoading label="Loading candidate configuration…" />
	{:else if services.length === 0}<p class="console-empty text-sm text-muted-foreground">
			No services configured for this watcher.
		</p>
	{:else}
		{#each services as svc (svc.serviceId)}
			<section
				id={`candidate-service-${svc.serviceId}`}
				aria-label={`${svc.serviceName} candidate`}
				class="scroll-mt-6 space-y-2"
			>
				<div class="flex flex-wrap items-center justify-between gap-3">
					<span class="text-xs text-muted-foreground"
						>{svc.hasCandidateEnv
							? `Saved draft: ${svc.revisionTarget}`
							: 'Using active configuration as the starting point'}</span
					>
					<div class="flex gap-2">
						<Button.Root
							variant="ghost"
							size="xs"
							disabled={busy || loading || !serviceChanged(svc)}
							onclick={() => (resetSelection = svc)}>Reset to saved</Button.Root
						>{#if svc.hasCandidateEnv}<Button.Root
								variant="ghost"
								size="xs"
								disabled={busy || loading || hasAnyChanges}
								onclick={() => {
									deleteError = '';
									confirmDelete = {
										serviceId: svc.serviceId,
										serviceName: svc.serviceName,
										target: svc.revisionTarget || effectiveTarget
									};
								}}>Delete saved draft</Button.Root
							>{/if}
					</div>
				</div>
				<ConfigWorkspace
					serviceName={svc.serviceName}
					context={loadedTarget || effectiveTarget}
					mode="candidate"
					directory={watcher.install_dir}
					envPath={watcher.services.find((service) => service.id === svc.serviceId)?.env_file || ''}
					bind:envContent={svc.candidateEnv}
					bind:files={svc.candidateFiles}
					baselineEnv={svc.activeEnv}
					baselineFiles={svc.activeFiles}
					savedEnv={svc.originalCandidateEnv}
					savedFiles={svc.originalCandidateFiles}
					explicitFiles={svc.filesExplicit}
					savedExplicitFiles={svc.originalFilesExplicit}
					disabled={busy ||
						saving ||
						approving ||
						discarding ||
						loading ||
						effectiveTarget !== loadedTarget}
					onfileschange={() => (svc.filesExplicit = true)}
				/>
			</section>
		{/each}
	{/if}
	{#if watcher.status === 'pending_approval' || retainedCatalog}
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
					onclick={openApproval}
					disabled={busy ||
						saving ||
						approving ||
						loading ||
						problems.length > 0 ||
						loadedSelection !== pendingSelection ||
						loadedTarget !== watcher.pending_version ||
						effectiveTarget !== watcher.pending_version}>Review deployment</Button.Root
				>
			</div>
		</section>
	{/if}
</fieldset>

<Dialog.Root
	open={requestedTarget !== null}
	onOpenChange={(open) => {
		if (!open) requestedTarget = null;
	}}
	><Dialog.Content
		><Dialog.Header
			><Dialog.Title>Switch configuration target?</Dialog.Title><Dialog.Description
				>Your edits belong to {loadedTarget}. Save them before switching, or discard the unsaved
				edits to load {requestedTarget?.target}.</Dialog.Description
			></Dialog.Header
		><Dialog.Footer
			><Button.Root variant="outline" onclick={() => (requestedTarget = null)}
				>Keep editing</Button.Root
			><Button.Root
				variant="destructive"
				onclick={async () => {
					const next = requestedTarget;
					requestedTarget = null;
					if (next) await selectTarget(next.target, next.custom, true);
				}}>Discard and switch</Button.Root
			></Dialog.Footer
		></Dialog.Content
	></Dialog.Root
>
<Dialog.Root
	open={resetSelection !== null}
	onOpenChange={(open) => {
		if (!open) resetSelection = null;
	}}
	><Dialog.Content
		><Dialog.Header
			><Dialog.Title>Reset this service draft?</Dialog.Title><Dialog.Description
				>Unsaved environment and managed-file edits for {resetSelection?.serviceName} will be replaced
				with the last loaded draft.</Dialog.Description
			></Dialog.Header
		><Dialog.Footer
			><Button.Root variant="outline" onclick={() => (resetSelection = null)}
				>Keep editing</Button.Root
			><Button.Root
				variant="destructive"
				onclick={() => {
					if (resetSelection) resetCandidate(resetSelection);
					resetSelection = null;
				}}>Reset draft</Button.Root
			></Dialog.Footer
		></Dialog.Content
	></Dialog.Root
>
<Dialog.Root
	bind:open={approvalOpen}
	onOpenChange={(open) => {
		if (approving && !open) approvalOpen = true;
	}}
	><Dialog.Content
		class="sm:max-w-xl"
		showCloseButton={!approving}
		onCloseAutoFocus={(event) => {
			if (approvalOpener?.isConnected) {
				event.preventDefault();
				approvalOpener.focus({ preventScroll: true });
			}
		}}
		><Dialog.Header
			><Dialog.Title>Deploy reviewed release {approvalVersion}?</Dialog.Title><Dialog.Description
				>Unsaved edits are saved to this candidate first. Watcher then deploys this exact release
				and restarts its services. Active configuration changes during activation.</Dialog.Description
			></Dialog.Header
		>
		<dl class="grid grid-cols-2 gap-4 rounded-md border border-border p-4 text-xs">
			<div>
				<dt class="text-muted-foreground">Release</dt>
				<dd class="mt-1 font-mono">{approvalVersion}</dd>
			</div>
			<div>
				<dt class="text-muted-foreground">Artifact identity</dt>
				<dd class="mt-1 font-mono">{approvalSelection}</dd>
			</div>
		</dl>
		<ul class="divide-y divide-border">
			{#each services as svc (svc.serviceId)}<li class="py-3 text-xs">
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
		{#if approvalSelection !== pendingSelection}<p role="alert" class="text-sm text-amber-300">
				The selected release changed. Close this review and load the current candidate.
			</p>{/if}<RequestError message={error} />
		<Dialog.Footer
			><Button.Root variant="outline" disabled={approving} onclick={() => (approvalOpen = false)}
				>Back to configuration</Button.Root
			><Button.Root
				disabled={busy || approving || approvalSelection !== pendingSelection}
				loading={approving}
				onclick={handleApprove}>Deploy reviewed release</Button.Root
			></Dialog.Footer
		>
	</Dialog.Content></Dialog.Root
>
<Dialog.Root
	open={confirmDelete !== null}
	onOpenChange={(open) => {
		if (!open && !deletingRevision) confirmDelete = null;
	}}
>
	<Dialog.Content showCloseButton={!deletingRevision}>
		<Dialog.Header>
			<Dialog.Title>Delete saved draft?</Dialog.Title>
			<Dialog.Description
				>Delete the environment and managed file draft for {confirmDelete?.serviceName} at {confirmDelete?.target}?
				Downloaded releases and historical snapshots stay unchanged.</Dialog.Description
			>
		</Dialog.Header>
		<RequestError message={deleteError} />
		<Dialog.Footer>
			<Button.Root
				variant="outline"
				disabled={deletingRevision}
				onclick={() => (confirmDelete = null)}>Cancel</Button.Root
			>
			<Button.Root
				variant="destructive"
				disabled={busy || saving || approving || discarding || hasAnyChanges || deletingRevision}
				onclick={deleteSavedRevision}>{deletingRevision ? 'Deleting…' : 'Delete draft'}</Button.Root
			>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
