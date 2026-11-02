import { createMutation, createQuery, useQueryClient } from '@tanstack/svelte-query';
import { api } from '$lib/api';
import { queries } from '$lib/query/options';
import { invalidate, message, watcherScope } from '$lib/query/client';
import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { configurationProblems } from '$lib/features/configuration/model';
import { markFor } from '$lib/features/watchers/releases/release-marks';
import {
	draftChanges,
	editableVersionService,
	isDirty,
	markSaved,
	type EditableVersionService
} from './version-config-model';

/** Edits a retained version's configuration as drafts that a manual rollback can apply. */
export function createVersionConfig(params: () => { watcherId: number; version: string }) {
	const client = useQueryClient();
	const watcherId = $derived(params().watcherId);
	const version = $derived(params().version);
	const configQuery = createQuery(() => queries.rollbackConfig(watcherId, version));
	const versionsQuery = createQuery(() => queries.versions(watcherId));
	const watcherQuery = createQuery(() => queries.watcher(watcherId));

	const ui = $state({
		services: [] as EditableVersionService[],
		loadedKey: '',
		error: '',
		notice: '',
		rollbackOpen: false,
		reportGitHub: true,
		useDraft: true
	});
	// Editable drafts are a separate copy: background refreshes must not overwrite edits.
	$effect(() => {
		const data = configQuery.data;
		const key = `${watcherId}:${version}`;
		if (data && ui.loadedKey !== key) {
			ui.services = data.services.map(editableVersionService);
			ui.loadedKey = key;
		}
	});

	const retained = $derived(versionsQuery.data?.versions.find((v) => v.version === version));
	const mark = $derived(markFor(versionsQuery.data?.marks, version));
	const current = $derived(
		Boolean(retained?.is_current) || watcherQuery.data?.current_version === version
	);
	const blocked = $derived(Boolean(mark?.blocked_at));
	const readonly = $derived(current || blocked);
	const dirty = $derived(ui.services.some(isDirty));
	const hasDraft = $derived(ui.services.some((service) => service.hasDraft));
	const problems = $derived(
		ui.services.flatMap((service) =>
			configurationProblems(service.envPath, service.files).map(
				(problem) => `${service.serviceName}: ${problem}`
			)
		)
	);

	const save = createMutation(() => ({
		mutationFn: async () => {
			const { save: drafts, discard } = draftChanges(ui.services);
			if (drafts.length) await api.updateWatcherCandidate(watcherId, version, drafts);
			for (const serviceId of discard) await api.deleteServiceConfigRevision(serviceId, version);
		},
		onSuccess: () => invalidate(client, ...watcherScope(watcherId))
	}));
	const rollback = createMutation(() => ({
		mutationFn: () =>
			api.rollbackWatcher(watcherId, version, ui.reportGitHub, ui.useDraft && hasDraft),
		onSuccess: () => invalidate(client, ...watcherScope(watcherId))
	}));

	async function saveDrafts() {
		if (save.isPending || problems.length) return;
		ui.error = '';
		try {
			await save.mutateAsync();
			markSaved(ui.services);
			ui.notice = hasDraft
				? 'Edits saved. They apply when you roll back to this version.'
				: 'No edits remain; rollback restores the saved configuration.';
		} catch (cause) {
			ui.error = message(cause) || 'Could not save the edits.';
		}
	}
	/** Puts the editor back to what the version last ran with; saving then removes the drafts. */
	function resetToSnapshot() {
		for (const service of ui.services) {
			service.env = service.snapshotEnv;
			service.files = service.snapshotFiles.map((file) => ({ ...file }));
		}
		ui.notice = '';
	}
	async function startRollback(_version: string, reportGitHub = true) {
		ui.reportGitHub = reportGitHub;
		ui.error = '';
		try {
			const res = await rollback.mutateAsync();
			ui.rollbackOpen = false;
			await goto(
				resolve('/watchers/[id]/logs/[logId]', {
					id: String(watcherId),
					logId: String(res.deploy_log_id)
				})
			);
		} catch (cause) {
			ui.error = message(cause) || `Rollback to ${version} could not start.`;
		}
	}

	return {
		ui,
		configQuery,
		saveDrafts,
		resetToSnapshot,
		startRollback,
		get mark() {
			return mark;
		},
		get current() {
			return current;
		},
		get blocked() {
			return blocked;
		},
		get readonly() {
			return readonly;
		},
		get dirty() {
			return dirty;
		},
		get hasDraft() {
			return hasDraft;
		},
		get problems() {
			return problems;
		},
		get saving() {
			return save.isPending;
		},
		get rollingBack() {
			return rollback.isPending;
		}
	};
}
