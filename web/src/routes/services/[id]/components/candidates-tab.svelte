<script lang="ts">
	import RequestError from '$lib/components/request-error.svelte';
	import type { ServiceConfigRevision } from '$lib/api';
	import { api } from '$lib/api';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Textarea } from '$lib/components/ui/textarea';
	import {
		Trash2,
		Save,
		RefreshCw,
		Plus,
		Check,
		Clock,
		Archive,
		Sparkles,
		Copy,
		AlertCircle
	} from '@lucide/svelte';
	import { onMount } from 'svelte';

	let {
		serviceId,
		currentEnv,
		watcherId,
		pendingVersion
	}: {
		serviceId: number;
		currentEnv: string;
		watcherId?: number;
		pendingVersion?: string;
	} = $props();

	let revisions = $state<ServiceConfigRevision[]>([]);
	let availableVersions = $state<string[]>([]);
	let loading = $state(true);
	let error = $state('');

	// Active selection
	let selectedTarget = $state('next');
	let isCustom = $state(false);
	let customTarget = $state('');

	let editorContent = $state('');
	let contentSource = $state<'active' | 'snapshot' | 'candidate'>('active');
	let saving = $state(false);
	let saved = $state(false);
	let deletingTarget = $state('');
	let confirmingDelete = $state('');
	let loadingContent = $state(false);
	let loadedTarget = $state('');
	let loadGeneration = 0;

	const effectiveTarget = $derived(isCustom ? customTarget.trim() : selectedTarget);

	async function loadRevisions() {
		try {
			const res = await api.getServiceConfigRevisions(serviceId);
			revisions = res.data || [];
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load config candidates';
		}
	}

	async function loadVersions() {
		if (!watcherId) return;
		try {
			const vers = await api.watcherVersions(watcherId);
			availableVersions = (vers || []).map((v) => v.version);
		} catch {
			availableVersions = [];
		}
	}

	async function selectTarget(target: string, custom = false) {
		if (saving || deletingTarget) return;
		isCustom = custom;
		selectedTarget = target;
		if (custom) {
			customTarget = target === 'custom' ? '' : target;
		}

		await loadContentForTarget(custom ? customTarget.trim() : target);
	}

	async function loadContentForTarget(target: string) {
		const generation = ++loadGeneration;
		loadedTarget = target;
		loadingContent = false;
		if (!target) {
			editorContent = currentEnv;
			contentSource = 'active';
			return;
		}

		// 1. Check if a candidate revision already exists in DB
		const existingRev =
			revisions.find((r) => r.target_version === target) ??
			revisions.find((r) => r.target_version === 'next');
		if (existingRev) {
			editorContent = existingRev.env_content;
			contentSource = 'candidate';
			return;
		}

		// 2. If it's an available on-disk version, fetch its snapshot
		if (availableVersions.includes(target)) {
			loadingContent = true;
			try {
				const snapshot = await api.getServiceSnapshotEnv(serviceId, target);
				if (generation !== loadGeneration) return;
				editorContent = snapshot.env_content;
				contentSource = 'snapshot';
				return;
			} catch {
				if (generation !== loadGeneration) return;
				// Snapshot missing, fallback to active
			} finally {
				if (generation === loadGeneration) loadingContent = false;
			}
		}

		// 3. Fallback to active current env
		editorContent = currentEnv;
		contentSource = 'active';
	}

	async function saveRevision() {
		const target = effectiveTarget;
		if (!target || target !== loadedTarget || loadingContent) {
			error = 'Load the selected target before saving';
			return;
		}
		saving = true;
		error = '';
		saved = false;
		try {
			await api.updateServiceConfigRevision(serviceId, target, editorContent);

			await loadRevisions();
			contentSource = 'candidate';
			saved = true;
			setTimeout(() => {
				saved = false;
			}, 3000);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save config candidate';
		} finally {
			saving = false;
		}
	}

	async function deleteRevision(target: string) {
		if (deletingTarget) return;
		deletingTarget = target;
		error = '';
		try {
			await api.deleteServiceConfigRevision(serviceId, target);
			await loadRevisions();
			if (effectiveTarget === target) {
				await loadContentForTarget(effectiveTarget);
			}
			confirmingDelete = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to delete config candidate';
		} finally {
			deletingTarget = '';
		}
	}

	onMount(async () => {
		try {
			await Promise.all([loadRevisions(), loadVersions()]);
			await loadContentForTarget('next');
		} finally {
			loading = false;
		}
	});
</script>

<fieldset disabled={saving || Boolean(deletingTarget)} class="min-w-0 space-y-4">
	<!-- Editor Card -->
	<Card.Root class="border-border bg-card">
		<Card.Header class="pb-3">
			<div class="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
				<div>
					<Card.Title class="text-lg">Candidate configuration</Card.Title>
					<Card.Description
						>Changes apply at deployment; saved snapshots stay unchanged.</Card.Description
					>
				</div>
				{#if revisions.length > 0}
					<span
						class="self-start rounded-full bg-primary/10 px-2.5 py-0.5 text-xs font-medium text-primary sm:self-auto"
					>
						{revisions.length} candidate{revisions.length === 1 ? '' : 's'} staged
					</span>
				{/if}
			</div>
		</Card.Header>
		<Card.Content class="space-y-4">
			<!-- Version Selector Buttons -->
			<div class="space-y-2">
				<p class="text-xs font-medium text-muted-foreground">Select Target Version:</p>
				<div class="flex flex-wrap items-center gap-2">
					<!-- Next release pill -->
					<button
						type="button"
						class={`inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors ${
							!isCustom && selectedTarget === 'next'
								? 'border-primary bg-primary text-primary-foreground'
								: 'border-border bg-muted/30 text-foreground hover:bg-muted/70'
						}`}
						onclick={() => selectTarget('next')}
					>
						<Sparkles class="h-3.5 w-3.5" />
						Next deployment
						{#if revisions.some((r) => r.target_version === 'next')}
							<span class="ml-1 h-1.5 w-1.5 rounded-full bg-emerald-400"></span>
						{/if}
					</button>

					<!-- Pending intercepted version if any -->
					{#if pendingVersion}
						<button
							type="button"
							class={`inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors ${
								!isCustom && selectedTarget === pendingVersion
									? 'border-amber-500 bg-amber-500 text-black'
									: 'border-amber-500/40 bg-amber-500/10 text-amber-300 hover:bg-amber-500/20'
							}`}
							onclick={() => selectTarget(pendingVersion)}
						>
							<Clock class="h-3.5 w-3.5" />
							Pending Approval ({pendingVersion})
							{#if revisions.some((r) => r.target_version === pendingVersion)}
								<span class="ml-1 h-1.5 w-1.5 rounded-full bg-emerald-400"></span>
							{/if}
						</button>
					{/if}

					<!-- Historical / Available Versions -->
					{#each availableVersions as ver (ver)}
						<button
							type="button"
							class={`inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 font-mono text-xs font-medium transition-colors ${
								!isCustom && selectedTarget === ver
									? 'border-primary bg-primary text-primary-foreground'
									: 'border-border bg-muted/30 text-foreground hover:bg-muted/70'
							}`}
							onclick={() => selectTarget(ver)}
						>
							<Archive class="h-3.5 w-3.5 text-blue-400" />
							{ver}
							{#if revisions.some((r) => r.target_version === ver)}
								<span class="ml-1 h-1.5 w-1.5 rounded-full bg-emerald-400"></span>
							{/if}
						</button>
					{/each}

					<!-- Custom tag button -->
					<button
						type="button"
						class={`inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors ${
							isCustom
								? 'border-primary bg-primary text-primary-foreground'
								: 'border-dashed border-border bg-muted/20 text-muted-foreground hover:bg-muted/50 hover:text-foreground'
						}`}
						onclick={() => selectTarget('custom', true)}
					>
						<Plus class="h-3.5 w-3.5" />
						Custom Version Tag...
					</button>
				</div>

				<!-- Custom version input field if custom selected -->
				{#if isCustom}
					<div class="mt-2 flex max-w-sm items-center gap-2">
						<Input
							bind:value={customTarget}
							placeholder="e.g. v2.0.0-rc1"
							class="h-8 font-mono text-xs"
							oninput={() => loadContentForTarget(customTarget.trim())}
						/>
						{#if customTarget.trim()}
							<span class="font-mono text-xs text-muted-foreground">({customTarget.trim()})</span>
						{/if}
					</div>
				{/if}
			</div>

			<!-- Environment Editor -->
			<div class="space-y-2">
				<div class="flex flex-wrap items-center justify-between gap-2">
					<div class="flex flex-wrap items-center gap-2">
						<p class="text-sm font-medium">
							Environment Variables for <code class="font-mono font-bold text-primary"
								>{effectiveTarget || 'unspecified'}</code
							>
						</p>
						{#if contentSource === 'candidate'}
							<span
								class="rounded bg-emerald-500/10 px-2 py-0.5 text-[10px] font-medium text-emerald-400"
							>
								Staged Candidate
							</span>
						{:else if contentSource === 'snapshot'}
							<span
								class="rounded bg-blue-500/10 px-2 py-0.5 text-[10px] font-medium text-blue-400"
							>
								Loaded from Snapshot
							</span>
						{:else}
							<span
								class="rounded bg-muted px-2 py-0.5 text-[10px] font-medium text-muted-foreground"
							>
								Seeded from Active Service
							</span>
						{/if}
					</div>

					<div class="flex flex-wrap items-center gap-2">
						{#if editorContent !== currentEnv}
							<Button.Root
								variant="ghost"
								size="sm"
								class="h-7 text-xs text-muted-foreground"
								onclick={() => (editorContent = currentEnv)}
								title="Reset editor to active environment content"
							>
								<Copy class="mr-1 h-3 w-3" /> Copy Active
							</Button.Root>
						{/if}
						<Button.Root
							variant="ghost"
							size="sm"
							class="h-7 text-xs text-muted-foreground"
							onclick={() => loadContentForTarget(effectiveTarget)}
							title="Re-load saved configuration for this target"
						>
							<RefreshCw class="mr-1 h-3 w-3" /> Reset
						</Button.Root>
					</div>
				</div>

				{#if loadingContent}
					<div class="p-8 text-center text-xs text-muted-foreground">
						<RefreshCw class="mx-auto mb-2 h-4 w-4 animate-spin" />
						Loading snapshot for {effectiveTarget}...
					</div>
				{:else}
					<Textarea
						bind:value={editorContent}
						class="min-h-[180px] font-mono text-sm text-blue-300"
						placeholder="KEY=VALUE"
					/>
				{/if}
			</div>

			<!-- Action buttons -->
			<div class="flex flex-wrap items-center gap-2">
				<Button.Root
					variant="default"
					size="sm"
					onclick={saveRevision}
					disabled={saving || !effectiveTarget}
				>
					{#if saving}
						<RefreshCw class="mr-1.5 h-3.5 w-3.5 animate-spin" /> Saving...
					{:else if saved}
						<Check class="mr-1.5 h-3.5 w-3.5 text-emerald-400" /> Saved Candidate!
					{:else}
						<Save class="mr-1.5 h-3.5 w-3.5" /> Save Candidate for {effectiveTarget || 'Version'}
					{/if}
				</Button.Root>

				{#if revisions.some((r) => r.target_version === effectiveTarget)}
					<Button.Root
						variant="outline"
						size="sm"
						class="text-red-400 hover:text-red-300"
						onclick={() => deleteRevision(effectiveTarget)}
					>
						<Trash2 class="mr-1.5 h-3.5 w-3.5" /> Discard Candidate
					</Button.Root>
				{/if}
			</div>
		</Card.Content>
	</Card.Root>

	<RequestError message={error} />

	<!-- Staged Candidates List -->
	<div class="space-y-3">
		<h3 class="text-sm font-semibold tracking-tight text-foreground">
			Currently Staged Candidate Revisions
		</h3>

		{#if loading}
			<div class="p-4 text-sm text-muted-foreground">Loading candidates...</div>
		{:else if revisions.length === 0}
			<div
				class="rounded-md border border-dashed border-border bg-muted/20 p-8 text-center text-sm text-muted-foreground"
			>
				No candidate revisions staged for this service. Select a version above to stage one.
			</div>
		{:else}
			<div class="grid gap-3">
				{#each revisions as rev (rev.target_version)}
					<Card.Root class="border-border bg-card">
						<Card.Header class="pb-2">
							<div class="section-toolbar">
								<div class="flex flex-wrap items-center gap-2">
									<Card.Title class="font-mono text-base font-bold text-primary">
										{rev.target_version}
									</Card.Title>
									{#if rev.target_version === 'next'}
										<span
											class="rounded bg-primary/10 px-2 py-0.5 text-[10px] font-medium text-primary"
										>
											Next Release
										</span>
									{:else if rev.target_version === pendingVersion}
										<span
											class="rounded bg-amber-500/10 px-2 py-0.5 text-[10px] font-medium text-amber-300"
										>
											Pending Approval
										</span>
									{:else if availableVersions.includes(rev.target_version)}
										<span
											class="rounded bg-blue-500/10 px-2 py-0.5 text-[10px] font-medium text-blue-400"
										>
											On-Disk Version
										</span>
									{/if}
								</div>

								<div class="flex flex-wrap items-center gap-2">
									<Button.Root
										variant="outline"
										size="sm"
										class="h-7 text-xs"
										onclick={() => {
											if (
												rev.target_version === 'next' ||
												rev.target_version === pendingVersion ||
												availableVersions.includes(rev.target_version)
											) {
												selectTarget(rev.target_version, false);
											} else {
												selectTarget(rev.target_version, true);
											}
										}}
									>
										Edit candidate
									</Button.Root>
									<Button.Root
										variant="ghost"
										size="icon"
										class="h-7 w-7 text-red-400 hover:text-red-300"
										onclick={() => {
											error = '';
											confirmingDelete = rev.target_version;
										}}
										title="Delete this candidate revision"
									>
										<Trash2 class="h-3.5 w-3.5" />
									</Button.Root>
								</div>
							</div>
							<Card.Description class="text-xs">
								Last updated: {new Date(rev.updated_at).toLocaleString()}
							</Card.Description>
						</Card.Header>
						<Card.Content>
							<Textarea
								value={rev.env_content}
								readonly
								class="max-h-[140px] min-h-[80px] bg-muted/20 font-mono text-xs text-blue-300"
							/>
						</Card.Content>
					</Card.Root>
				{/each}
			</div>
		{/if}
	</div>
</fieldset>

<Dialog.Root
	open={Boolean(confirmingDelete)}
	onOpenChange={(open) => {
		if (!open && !deletingTarget) confirmingDelete = '';
	}}
>
	<Dialog.Content showCloseButton={!deletingTarget}>
		<Dialog.Header
			><Dialog.Title>Delete candidate?</Dialog.Title><Dialog.Description
				>The staged configuration for {confirmingDelete} will be removed. Active configuration and release
				snapshots stay unchanged.</Dialog.Description
			></Dialog.Header
		>
		{#if error}<p role="alert" class="text-sm text-red-400">{error}</p>{/if}
		<Dialog.Footer
			><Button.Root
				variant="outline"
				disabled={Boolean(deletingTarget)}
				onclick={() => (confirmingDelete = '')}>Cancel</Button.Root
			><Button.Root
				variant="destructive"
				disabled={Boolean(deletingTarget)}
				loading={Boolean(deletingTarget)}
				onclick={() => deleteRevision(confirmingDelete)}>Delete candidate</Button.Root
			></Dialog.Footer
		>
	</Dialog.Content>
</Dialog.Root>
