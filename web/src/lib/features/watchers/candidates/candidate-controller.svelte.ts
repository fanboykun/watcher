import type { CandidateProps } from './candidate-model';
import {
	type EditableServiceCandidate,
	serviceChanged,
	resetCandidate,
	buildCandidatePayload,
	editableCandidate,
	markSaved
} from './candidate-model';

import { createQuery, createMutation, useQueryClient } from '@tanstack/svelte-query';
import { queries, keys } from '$lib/query/options';
import { invalidate } from '$lib/query/client';
import { createCommand } from '$lib/query/command.svelte';
import { configurationProblems } from '$lib/features/configuration/model';
import { api, type Watcher } from '$lib/api';
import { Save } from '@lucide/svelte';
import { onDestroy, onMount, tick } from 'svelte';
import { afterNavigate } from '$app/navigation';
import { page } from '$app/state';
export function createCandidate(getProps: () => CandidateProps) {
	const { watcher, busy = false, onApprove, onDiscard, onRefreshWatcher } = $derived(getProps());

	const ui = $state({
		selectedTarget: '',
		isCustom: false,
		customTarget: '',
		services: [] as EditableServiceCandidate[],
		requestTarget: '',
		requestSelection: '',
		loadedTarget: '',
		loadedSelection: '',
		saveSuccess: '',
		error: '',
		savedTargets: [] as string[],
		requestedTarget: null as { target: string; custom: boolean } | null,
		resetSelection: null as EditableServiceCandidate | null,
		approvalOpen: false,
		approvalSelection: '',
		approvalVersion: '',
		confirmDelete: null as { serviceId: number; serviceName: string; target: string } | null,
		deleteError: ''
	});

	const client = useQueryClient();

	const candidateQuery = createQuery(() => ({
		...queries.candidate(watcher.id, ui.requestTarget, ui.requestSelection),
		enabled: false
	}));
	const loading = $derived(candidateQuery.isFetching);
	const saveMutation = createMutation(() => ({
		mutationFn: (v: { target: string; services: ReturnType<typeof candidatePayload> }) =>
			api.updateWatcherCandidate(watcher.id, v.target, v.services)
	}));
	const deleteMutation = createMutation(() => ({
		mutationFn: (v: { serviceId: number; target: string }) =>
			api.deleteServiceConfigRevision(v.serviceId, v.target),
		onSuccess: () => invalidate(client, [...keys.watcher(watcher.id), 'candidate'])
	}));
	const approvalMutation = createCommand();
	const discardMutation = createCommand();

	let loadGeneration = 0;
	const saving = $derived(saveMutation.isPending);

	const approving = $derived(approvalMutation.isPending);
	const discarding = $derived(discardMutation.isPending);

	let approvalOpener: HTMLElement | null = null;
	const problems = $derived(
		ui.services.flatMap((svc) =>
			configurationProblems(
				watcher.services.find((service) => service.id === svc.serviceId)?.env_file || '',
				svc.candidateFiles
			)
		)
	);
	const deletingRevision = $derived(deleteMutation.isPending);

	const pendingSelection = $derived(
		`${watcher.id}:${watcher.pending_catalog_id}:${watcher.pending_version}`
	);
	const activeTargetChoice = $derived(ui.selectedTarget || watcher.pending_version || 'next');
	// Typing a proposed version must not retarget an already loaded draft.
	const effectiveTarget = $derived(
		ui.isCustom && ui.selectedTarget === 'custom' ? '' : activeTargetChoice
	);
	const retainedCatalog = $derived(
		watcher.pending_catalog_id > 0 &&
			watcher.status !== 'approved' &&
			watcher.status !== 'deploying'
	);

	function candidatePayload() {
		return buildCandidatePayload(ui.services);
	}

	const hasAnyChanges = $derived(ui.services.some(serviceChanged));

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
		ui.loadedTarget = '';
		ui.loadedSelection = '';
		ui.error = '';
		try {
			ui.requestTarget = target;
			ui.requestSelection = selection;
			await tick();
			const result = await candidateQuery.refetch({ throwOnError: true, cancelRefetch: false });
			const res = result.data!;
			if (generation !== loadGeneration || selection !== pendingSelection) return;
			ui.loadedTarget = target;
			ui.loadedSelection = selection;
			ui.savedTargets = res.saved_targets ?? [];
			ui.services = res.services.map((service) => editableCandidate(service, target));
			void focusRequestedService();
		} catch (e) {
			if (generation === loadGeneration)
				ui.error = e instanceof Error ? e.message : 'Failed to load candidate configuration';
		}
	}

	// Tabs keep this editor mounted. Catalog selection refreshes the watcher before
	// navigating here; routine watcher refreshes must preserve in-progress edits.
	let navigatedSelection = '';
	function syncPendingSelection() {
		if (navigatedSelection === pendingSelection) return;
		navigatedSelection = pendingSelection;
		++loadGeneration;
		ui.loadedSelection = '';
		if (hasAnyChanges) {
			ui.error =
				'The pending release changed. Your edits are preserved; save them before loading the new candidate.';
			return;
		}
		ui.isCustom = false;
		ui.selectedTarget = watcher.pending_version || 'next';
		void loadCandidateData(ui.selectedTarget);
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
			ui.requestedTarget = { target, custom };
			return;
		}
		if (saving || approving || discarding || deletingRevision) return;
		ui.isCustom = custom;
		ui.selectedTarget = target;
		if (custom) {
			ui.customTarget = target === 'custom' ? '' : target;
		}
		const toLoad = custom ? ui.customTarget.trim() : target;
		if (toLoad) {
			await loadCandidateData(toLoad);
		}
	}

	async function handleCustomTargetSubmit() {
		if (ui.customTarget.trim()) {
			await selectTarget(ui.customTarget.trim(), true);
		}
	}

	async function saveAllCandidates() {
		if (busy || saving || approving || discarding || deletingRevision) return;
		if (loading || effectiveTarget !== ui.loadedTarget) {
			ui.error = 'Load the selected target before saving';
			return;
		}
		if (!effectiveTarget) {
			ui.error = 'Please specify a target version';
			return;
		}
		if (problems.length) {
			ui.error = problems.join(' ');
			return;
		}
		ui.error = '';
		ui.saveSuccess = '';
		try {
			await saveMutation.mutateAsync({ target: effectiveTarget, services: candidatePayload() });
			markSaved(ui.services, effectiveTarget);
			if (!ui.savedTargets.includes(effectiveTarget))
				ui.savedTargets = [...ui.savedTargets, effectiveTarget].sort();
			ui.saveSuccess = `Candidate configuration saved for version "${effectiveTarget}"!`;
			setTimeout(() => {
				ui.saveSuccess = '';
			}, 4000);
			if (onRefreshWatcher) {
				await onRefreshWatcher();
			}
		} catch (e) {
			ui.error = e instanceof Error ? e.message : 'Failed to save candidate configuration';
		}
	}

	async function deleteSavedRevision() {
		const selected = ui.confirmDelete;
		if (!selected || busy || saving || approving || discarding || deletingRevision || hasAnyChanges)
			return;
		ui.deleteError = '';
		try {
			await deleteMutation.mutateAsync(selected);
			ui.confirmDelete = null;
			await loadCandidateData(effectiveTarget);
		} catch (cause) {
			ui.deleteError = cause instanceof Error ? cause.message : 'Could not delete the saved draft';
		}
	}

	async function handleApprove() {
		if (approving) return;
		await approvalMutation.mutateAsync({ name: 'approve', run: executeApproval });
	}
	async function executeApproval() {
		if (!onApprove || deletingRevision || busy || saving || problems.length) return;
		if (ui.approvalSelection !== pendingSelection) {
			ui.error = 'The pending release changed. Close this review and load the new candidate.';
			return;
		}
		if (
			loading ||
			ui.loadedTarget !== watcher.pending_version ||
			effectiveTarget !== watcher.pending_version ||
			ui.loadedSelection !== pendingSelection
		) {
			ui.error = 'Select and load the pending release before approving';
			return;
		}
		const version = watcher.pending_version;
		const selection = pendingSelection;
		const catalogId = watcher.pending_catalog_id;
		ui.error = '';
		try {
			// Save any unpersisted edits first
			if (hasAnyChanges) {
				await saveMutation.mutateAsync({ target: effectiveTarget, services: candidatePayload() });
				markSaved(ui.services, effectiveTarget);
			}
			if (selection !== pendingSelection || ui.loadedSelection !== selection) {
				ui.error = 'The pending release changed. Load and review it before approving.';
				return;
			}
			await onApprove(version, catalogId);
			ui.approvalOpen = false;
		} catch (e) {
			ui.error = e instanceof Error ? e.message : 'Approval failed';
		}
	}

	async function handleDiscard() {
		if (!onDiscard || deletingRevision) return;
		ui.error = '';
		try {
			await discardMutation.mutateAsync({ name: 'discard', run: onDiscard });
		} catch (e) {
			ui.error = e instanceof Error ? e.message : 'Discard failed';
		}
	}

	function openApproval(event: MouseEvent) {
		approvalOpener = event.currentTarget as HTMLElement;
		ui.approvalSelection = pendingSelection;
		ui.approvalVersion = ui.loadedTarget;
		ui.error = '';
		ui.approvalOpen = true;
	}
	return {
		ui,
		get hasAnyChanges() {
			return hasAnyChanges;
		},
		get busy() {
			return busy;
		},
		get saving() {
			return saving;
		},
		get approving() {
			return approving;
		},
		get discarding() {
			return discarding;
		},
		get deletingRevision() {
			return deletingRevision;
		},
		get watcher() {
			return watcher;
		},
		get retainedCatalog() {
			return retainedCatalog;
		},
		get loading() {
			return loading;
		},
		get pendingSelection() {
			return pendingSelection;
		},
		handleDiscard,
		saveAllCandidates,
		get effectiveTarget() {
			return effectiveTarget;
		},
		get activeTargetChoice() {
			return activeTargetChoice;
		},
		selectTarget,
		handleCustomTargetSubmit,
		serviceChanged,
		openApproval,
		get problems() {
			return problems;
		},
		resetCandidate,
		get approvalOpener() {
			return approvalOpener;
		},
		handleApprove,
		deleteSavedRevision
	};
}
