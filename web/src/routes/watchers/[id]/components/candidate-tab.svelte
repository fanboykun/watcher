<script lang="ts">
	import {
		api,
		type Watcher,
		type WatcherCandidateResponse,
		type ServiceCandidateInfo
	} from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Textarea } from '$lib/components/ui/textarea';
	import RequestLoading from '$lib/components/request-loading.svelte';
	import {
		AlertCircle,
		Check,
		Copy,
		Eye,
		Layers,
		Pencil,
		RefreshCw,
		Save,
		Sparkles,
		Split,
		Trash2,
		Zap
	} from '@lucide/svelte';
	import { onMount } from 'svelte';

	interface EditableServiceCandidate {
		serviceId: number;
		serviceName: string;
		serviceType: string;
		activeEnv: string;
		candidateEnv: string;
		originalCandidateEnv: string;
		hasCandidateEnv: boolean;
		viewMode: 'edit' | 'diff' | 'side-by-side';
	}

	interface DiffLine {
		key: string;
		status: 'added' | 'removed' | 'modified' | 'unchanged';
		oldValue?: string;
		newValue?: string;
		raw: string;
	}

	let {
		watcher,
		onApprove,
		onDiscard,
		onRefreshWatcher
	}: {
		watcher: Watcher;
		onApprove?: (version: string) => Promise<void>;
		onDiscard?: () => Promise<void>;
		onRefreshWatcher?: () => Promise<void>;
	} = $props();

	let selectedTarget = $state('');
	let isCustom = $state(false);
	let customTarget = $state('');

	let services = $state<EditableServiceCandidate[]>([]);
	let loading = $state(true);
	let loadedTarget = $state('');
	let loadGeneration = 0;
	let saving = $state(false);
	let saveSuccess = $state('');
	let error = $state('');
	let approving = $state(false);
	let discarding = $state(false);

	const activeTargetChoice = $derived(selectedTarget || watcher.pending_version || 'next');
	const effectiveTarget = $derived(isCustom ? customTarget.trim() : activeTargetChoice);

	const hasAnyChanges = $derived(services.some((s) => s.candidateEnv !== s.originalCandidateEnv));

	const hasAnyDifferenceFromActive = $derived(services.some((s) => s.candidateEnv !== s.activeEnv));

	async function loadCandidateData(target: string) {
		if (!target) return;
		const generation = ++loadGeneration;
		loading = true;
		error = '';
		try {
			const res = await api.getWatcherCandidate(watcher.id, target);
			if (generation !== loadGeneration) return;
			loadedTarget = target;
			services = res.services.map((s) => ({
				serviceId: s.service_id,
				serviceName: s.service_name,
				serviceType: s.service_type,
				activeEnv: s.active_env,
				candidateEnv: s.candidate_env,
				originalCandidateEnv: s.candidate_env,
				hasCandidateEnv: s.has_candidate_env,
				viewMode: 'edit'
			}));
		} catch (e) {
			if (generation === loadGeneration) error = e instanceof Error ? e.message : 'Failed to load candidate configuration';
		} finally {
			if (generation === loadGeneration) loading = false;
		}
	}

	onMount(() => {
		selectedTarget = watcher.pending_version || 'next';
		void loadCandidateData(selectedTarget);
	});

	async function selectTarget(target: string, custom = false) {
		if (saving || approving || discarding) return;
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
			await loadCandidateData(customTarget.trim());
		}
	}

	async function saveAllCandidates() {
		if (loading || effectiveTarget !== loadedTarget) {
			error = 'Load the selected target before saving';
			return;
		}
		if (!effectiveTarget) {
			error = 'Please specify a target version';
			return;
		}
		saving = true;
		error = '';
		saveSuccess = '';
		try {
			await api.updateWatcherCandidate(
				watcher.id,
				effectiveTarget,
				services.map((s) => ({
					service_id: s.serviceId,
					env_content: s.candidateEnv
				}))
			);
			for (const s of services) {
				s.originalCandidateEnv = s.candidateEnv;
				s.hasCandidateEnv = true;
			}
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

	function copyActive(service: EditableServiceCandidate) {
		service.candidateEnv = service.activeEnv;
	}

	function resetCandidate(service: EditableServiceCandidate) {
		service.candidateEnv = service.originalCandidateEnv;
	}

	async function handleApprove() {
		if (!onApprove) return;
		if (loading || loadedTarget !== watcher.pending_version || effectiveTarget !== watcher.pending_version) {
			error = 'Select and load the pending release before approving';
			return;
		}
		const version = watcher.pending_version;
		approving = true;
		error = '';
		try {
			// Save any unpersisted edits first
			if (hasAnyChanges) {
				await api.updateWatcherCandidate(
					watcher.id,
					effectiveTarget,
					services.map((s) => ({
						service_id: s.serviceId,
						env_content: s.candidateEnv
					}))
				);
			}
			await onApprove(version);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Approval failed';
		} finally {
			approving = false;
		}
	}

	async function handleDiscard() {
		if (!onDiscard) return;
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

	// Compute key-value differences between active and candidate env
	function computeEnvDiff(active: string, candidate: string): DiffLine[] {
		const parseEnvLines = (text: string) => {
			const map = new Map<string, string>();
			const order: string[] = [];
			const lines = text.split('\n');
			for (const line of lines) {
				const trimmed = line.trim();
				if (!trimmed || trimmed.startsWith('#')) continue;
				const idx = line.indexOf('=');
				if (idx > 0) {
					const key = line.slice(0, idx).trim();
					const val = line.slice(idx + 1);
					map.set(key, val);
					if (!order.includes(key)) order.push(key);
				}
			}
			return { map, order };
		};

		const activeParsed = parseEnvLines(active);
		const candidateParsed = parseEnvLines(candidate);

		const result: DiffLine[] = [];
		const allKeys = new Set([...candidateParsed.order, ...activeParsed.order]);

		for (const key of allKeys) {
			const inActive = activeParsed.map.has(key);
			const inCandidate = candidateParsed.map.has(key);
			const oldVal = activeParsed.map.get(key);
			const newVal = candidateParsed.map.get(key);

			if (inCandidate && !inActive) {
				result.push({
					key,
					status: 'added',
					newValue: newVal,
					raw: `+ ${key}=${newVal}`
				});
			} else if (!inCandidate && inActive) {
				result.push({
					key,
					status: 'removed',
					oldValue: oldVal,
					raw: `- ${key}=${oldVal}`
				});
			} else if (oldVal !== newVal) {
				result.push({
					key,
					status: 'modified',
					oldValue: oldVal,
					newValue: newVal,
					raw: `~ ${key}: ${oldVal} → ${newVal}`
				});
			} else {
				result.push({
					key,
					status: 'unchanged',
					oldValue: oldVal,
					newValue: newVal,
					raw: `  ${key}=${newVal}`
				});
			}
		}

		return result;
	}
</script>

<div class="space-y-6">
	<!-- Top Alert / Callouts -->
	{#if error}
		<div
			class="flex items-center rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400"
		>
			<AlertCircle class="mr-2 h-4 w-4 shrink-0" />
			<span>{error}</span>
		</div>
	{/if}

	{#if saveSuccess}
		<div
			class="flex items-center rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-4 text-sm text-emerald-400"
		>
			<Check class="mr-2 h-4 w-4 shrink-0" />
			<span>{saveSuccess}</span>
		</div>
	{/if}

	<!-- Release Candidate Staging Hub Banner -->
	{#if watcher.status === 'pending_approval'}
		<div
			class="rounded-xl border border-purple-500/40 bg-purple-950/20 p-5 shadow-sm dark:bg-purple-950/30"
		>
			<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
				<div class="space-y-1">
					<div class="flex items-center gap-2">
						<span class="relative flex h-3 w-3">
							<span
								class="absolute inline-flex h-full w-full animate-ping rounded-full bg-purple-400 opacity-75"
							></span>
							<span class="relative inline-flex h-3 w-3 rounded-full bg-purple-500"></span>
						</span>
						<h3 class="text-base font-semibold text-purple-200">
							Release Candidate Pending Approval
						</h3>
						<span
							class="rounded bg-purple-500/20 px-2 py-0.5 font-mono text-xs font-semibold text-purple-300"
						>
							{watcher.pending_version}
						</span>
					</div>
					<p class="text-xs text-purple-300/80">
						This release was held to let you prepare configuration before deploying. Edit your
						service variables below, then click "Approve & Deploy" when ready.
					</p>
				</div>

				<div class="flex flex-wrap items-center gap-2">
					<Button.Root
						variant="outline"
						size="sm"
						class="border-red-500/30 text-red-400 hover:bg-red-500/10 hover:text-red-300"
						onclick={handleDiscard}
						disabled={discarding || approving}
					>
						<Trash2 class="mr-1.5 h-3.5 w-3.5" />
						{discarding ? 'Discarding...' : 'Discard Release'}
					</Button.Root>
					<Button.Root
						variant="default"
						size="sm"
						class="bg-purple-600 font-medium text-white hover:bg-purple-700"
						onclick={handleApprove}
						disabled={approving || discarding || saving || loading || effectiveTarget !== watcher.pending_version || loadedTarget !== watcher.pending_version}
					>
						{#if approving}
							<RefreshCw class="mr-1.5 h-3.5 w-3.5 animate-spin" /> Deploying...
						{:else}
							<Zap class="mr-1.5 h-3.5 w-3.5" /> Approve & Deploy Now
						{/if}
					</Button.Root>
				</div>
			</div>
		</div>
	{:else}
		<div
			class="flex flex-col gap-2 rounded-lg border border-border/60 bg-muted/20 p-4 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between"
		>
			<div class="flex items-center gap-2">
				<Sparkles class="h-4 w-4 shrink-0 text-amber-400" />
				<div>
					<span class="font-medium text-foreground">Release First, Deploy Later:</span>
					Prepare upcoming environment changes across your services without touching active production.
					{#if !watcher.auto_deploy}
						<span class="font-medium text-emerald-400">
							(Auto-Deploy is OFF — all new releases will hold here for review).
						</span>
					{:else}
						<span>
							(Auto-Deploy is ON — enable "Intercept Next" in header if you want the next release to
							hold here).
						</span>
					{/if}
				</div>
			</div>
		</div>
	{/if}

	<!-- Version Selector and Actions Bar -->
	<Card.Root class="border-border bg-card">
		<Card.Header class="pb-3">
			<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
				<div>
					<Card.Title class="text-base font-semibold">Target Version Staging</Card.Title>
					<Card.Description class="text-xs">
						Select the version tag you are staging configurations for.
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
						disabled={saving || loading || !effectiveTarget}
					>
						{#if saving}
							<RefreshCw class="mr-1.5 h-3.5 w-3.5 animate-spin" /> Saving...
						{:else}
							<Save class="mr-1.5 h-3.5 w-3.5" /> Save Candidate Changes
						{/if}
					</Button.Root>
				</div>
			</div>

			<!-- Quick Target Selector Pills -->
			<div class="mt-3 flex flex-wrap items-center gap-2">
				{#if watcher.pending_version}
					<button
						type="button"
						class={`inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-semibold transition ${
							!isCustom && activeTargetChoice === watcher.pending_version
								? 'bg-purple-600 text-white shadow-sm'
								: 'border border-purple-500/40 bg-purple-500/10 text-purple-300 hover:bg-purple-500/20'
						}`}
						onclick={() => selectTarget(watcher.pending_version)}
					>
						<Zap class="h-3 w-3" />
						{watcher.pending_version}
						<span class="py-0.2 rounded bg-black/20 px-1 text-[10px] tracking-wider uppercase">
							Candidate
						</span>
					</button>
				{/if}

				<button
					type="button"
					class={`inline-flex items-center gap-1 rounded-full px-3 py-1 text-xs font-medium transition ${
						!isCustom && activeTargetChoice === 'next'
							? 'bg-primary text-primary-foreground shadow-sm'
							: 'border border-border bg-muted/40 text-muted-foreground hover:bg-muted'
					}`}
					onclick={() => selectTarget('next')}
				>
					<Layers class="h-3 w-3" />
					next
					<span class="text-[10px] text-muted-foreground/80">(Default upcoming)</span>
				</button>

				<button
					type="button"
					class={`inline-flex items-center gap-1 rounded-full px-3 py-1 text-xs font-medium transition ${
						isCustom
							? 'bg-primary text-primary-foreground shadow-sm'
							: 'border border-border bg-muted/40 text-muted-foreground hover:bg-muted'
					}`}
					onclick={() => selectTarget(customTarget || 'custom', true)}
				>
					<Pencil class="h-3 w-3" />
					Custom Tag...
				</button>

				{#if isCustom}
					<div class="flex items-center gap-1.5 pl-1">
						<Input
							bind:value={customTarget}
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
		</Card.Header>
	</Card.Root>

	<!-- Services Staging List -->
	{#if loading}
		<RequestLoading label="Loading candidate configuration..." />
	{:else if services.length === 0}
		<Card.Root class="border-dashed border-border bg-card">
			<Card.Content class="py-12 text-center">
				<p class="text-sm text-muted-foreground">No services configured for this watcher.</p>
			</Card.Content>
		</Card.Root>
	{:else}
		<div class="space-y-4">
			{#each services as svc (svc.serviceId)}
				{@const diffLines = computeEnvDiff(svc.activeEnv, svc.candidateEnv)}
				{@const hasModified = svc.candidateEnv !== svc.activeEnv}

				<Card.Root class="border-border bg-card">
					<Card.Header class="pb-3">
						<div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
							<div class="space-y-0.5">
								<div class="flex items-center gap-2">
									<Card.Title class="text-base font-semibold">{svc.serviceName}</Card.Title>
									<span
										class="rounded border border-border px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground uppercase"
									>
										{svc.serviceType}
									</span>
									{#if hasModified}
										<span
											class="rounded bg-amber-500/10 px-2 py-0.5 text-xs font-medium text-amber-400"
										>
											Modified from active
										</span>
									{:else}
										<span class="rounded bg-muted px-2 py-0.5 text-xs text-muted-foreground">
											Matches active
										</span>
									{/if}
								</div>
								<Card.Description class="text-xs">
									Target version: <span class="font-mono font-medium text-foreground"
										>{effectiveTarget}</span
									>
								</Card.Description>
							</div>

							<!-- Service Toolbar -->
							<div class="flex flex-wrap items-center gap-1.5">
								<!-- View Mode Switcher -->
								<div class="inline-flex rounded-lg border border-border/80 bg-muted/40 p-0.5">
									<button
										type="button"
										class={`flex items-center gap-1 rounded-md px-2.5 py-1 text-xs transition ${
											svc.viewMode === 'edit'
												? 'bg-background font-medium text-foreground shadow-xs'
												: 'text-muted-foreground hover:text-foreground'
										}`}
										onclick={() => (svc.viewMode = 'edit')}
									>
										<Pencil class="h-3 w-3" /> Edit
									</button>
									<button
										type="button"
										class={`flex items-center gap-1 rounded-md px-2.5 py-1 text-xs transition ${
											svc.viewMode === 'side-by-side'
												? 'bg-background font-medium text-foreground shadow-xs'
												: 'text-muted-foreground hover:text-foreground'
										}`}
										onclick={() => (svc.viewMode = 'side-by-side')}
									>
										<Split class="h-3 w-3" /> Side-by-Side
									</button>
									<button
										type="button"
										class={`flex items-center gap-1 rounded-md px-2.5 py-1 text-xs transition ${
											svc.viewMode === 'diff'
												? 'bg-background font-medium text-foreground shadow-xs'
												: 'text-muted-foreground hover:text-foreground'
										}`}
										onclick={() => (svc.viewMode = 'diff')}
									>
										<Eye class="h-3 w-3" /> Diff
									</button>
								</div>

								{#if svc.activeEnv && svc.candidateEnv !== svc.activeEnv}
									<Button.Root
										variant="ghost"
										size="sm"
										class="h-8 text-xs text-muted-foreground"
										onclick={() => copyActive(svc)}
										title="Copy active production variables into candidate"
									>
										<Copy class="mr-1 h-3.5 w-3.5" /> Copy Active
									</Button.Root>
								{/if}
								{#if svc.candidateEnv !== svc.originalCandidateEnv}
									<Button.Root
										variant="ghost"
										size="sm"
										class="h-8 text-xs text-muted-foreground"
										onclick={() => resetCandidate(svc)}
										title="Reset candidate content"
									>
										<RefreshCw class="mr-1 h-3.5 w-3.5" /> Reset
									</Button.Root>
								{/if}
							</div>
						</div>
					</Card.Header>

					<Card.Content class="pt-0">
						{#if svc.viewMode === 'edit'}
							<div class="space-y-1">
								<Textarea
									bind:value={svc.candidateEnv}
									class="min-h-[240px] font-mono text-xs leading-relaxed text-blue-300"
									placeholder="KEY=VALUE"
								/>
							</div>
						{:else if svc.viewMode === 'side-by-side'}
							<div class="grid gap-4 md:grid-cols-2">
								<div class="space-y-1.5">
									<div class="flex items-center justify-between">
										<span class="text-xs font-medium text-muted-foreground"
											>Active Production Environment</span
										>
										<span
											class="rounded bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground"
											>Running</span
										>
									</div>
									<Textarea
										value={svc.activeEnv}
										readonly
										class="min-h-[260px] bg-muted/20 font-mono text-xs text-muted-foreground focus-visible:ring-0"
										placeholder="Empty active environment"
									/>
								</div>
								<div class="space-y-1.5">
									<div class="flex items-center justify-between">
										<span class="text-xs font-medium text-foreground"
											>Candidate ({effectiveTarget})</span
										>
										<span
											class="rounded bg-blue-500/10 px-1.5 py-0.5 font-mono text-[10px] text-blue-400"
											>Editable</span
										>
									</div>
									<Textarea
										bind:value={svc.candidateEnv}
										class="min-h-[260px] font-mono text-xs leading-relaxed text-blue-300"
										placeholder="KEY=VALUE"
									/>
								</div>
							</div>
						{:else if svc.viewMode === 'diff'}
							<div class="rounded-lg border border-border/80 bg-zinc-950 p-4 font-mono text-xs">
								<div
									class="mb-3 flex items-center justify-between border-b border-border/50 pb-2 text-[11px] text-muted-foreground"
								>
									<span>Active (.env) vs Candidate ({effectiveTarget})</span>
									<span>{diffLines.filter((l) => l.status !== 'unchanged').length} changes</span>
								</div>
								{#if diffLines.filter((l) => l.status !== 'unchanged').length === 0}
									<p class="py-4 text-center text-muted-foreground">
										No differences between active environment and candidate configuration.
									</p>
								{:else}
									<div class="max-h-[360px] space-y-1 overflow-y-auto">
										{#each diffLines as line (line.key)}
											{#if line.status === 'added'}
												<div
													class="flex items-start gap-2 rounded bg-emerald-500/15 px-2 py-0.5 text-emerald-300"
												>
													<span class="font-bold select-none">+</span>
													<span>{line.key}={line.newValue}</span>
												</div>
											{:else if line.status === 'removed'}
												<div
													class="flex items-start gap-2 rounded bg-red-500/15 px-2 py-0.5 text-red-400"
												>
													<span class="font-bold select-none">-</span>
													<span>{line.key}={line.oldValue}</span>
												</div>
											{:else if line.status === 'modified'}
												<div
													class="flex flex-col gap-0.5 rounded bg-amber-500/15 px-2 py-1 text-amber-300"
												>
													<div class="flex items-center gap-2">
														<span class="font-bold text-amber-400 select-none">~</span>
														<span class="font-semibold">{line.key}</span>
													</div>
													<div class="pl-4 text-[11px] opacity-80">
														<span class="text-red-400 line-through">{line.oldValue}</span>
														<span class="mx-1 text-muted-foreground">→</span>
														<span class="text-emerald-300">{line.newValue}</span>
													</div>
												</div>
											{:else}
												<div class="flex items-start gap-2 px-2 py-0.5 text-zinc-500">
													<span class="opacity-40 select-none"> </span>
													<span>{line.key}={line.newValue}</span>
												</div>
											{/if}
										{/each}
									</div>
								{/if}
							</div>
						{/if}
					</Card.Content>
				</Card.Root>
			{/each}
		</div>
	{/if}
</div>
