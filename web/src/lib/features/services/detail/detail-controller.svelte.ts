import {
	copyFiles,
	configFingerprint,
	configurationProblems
} from '$lib/features/configuration/model';
import { untrack } from 'svelte';
import { createQuery, createMutation, useQueryClient } from '@tanstack/svelte-query';
import { queries, keys } from '$lib/query/options';
import { forget, invalidate, message, serviceScope, watcherScope } from '$lib/query/client';
import { createCommand } from '$lib/query/command.svelte';
import { page } from '$app/state';
import { api, type Service, type ServiceConfigFile } from '$lib/api';
import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
export function createServiceDetail() {
	const client = useQueryClient();
	const id = $derived(Number(page.params.id));
	const detail = createQuery(() => queries.service(id));
	const statusQuery = createQuery(() => ({ ...queries.serviceStatus(id), enabled: !!detail.data }));
	const service = $derived.by(() => {
		const base = detail.data?.service;
		if (!base) return null;
		const status = statusQuery.data;
		const runtime = Object.fromEntries(
			Object.entries(status ?? {}).filter(
				([key]) => key.startsWith('last_service_') || key.startsWith('last_health_')
			)
		);
		return { ...base, ...runtime };
	});
	const watcher = $derived(detail.data?.watcher ?? null);
	const ui = $state({
		deployPage: 1,
		deployPageSize: 10,
		logType: 'out' as 'out' | 'err',
		logCount: 100,
		error: '',
		actionMsg: '',
		envContent: '',
		baselineEnv: '',
		baselineFiles: [] as ServiceConfigFile[],
		configConflict: false,
		configFiles: [] as ServiceConfigFile[],
		showDeleteDialog: false,
		confirmServiceAction: null as 'stop' | 'restart' | null,
		activeTab: page.url.searchParams.get('tab') || 'health'
	});

	const healthQuery = createQuery(() => ({ ...queries.health(id), enabled: !!detail.data }));
	const deployQuery = createQuery(() => ({
		...queries.serviceDeploys(id, ui.deployPage, ui.deployPageSize),
		enabled: !!detail.data
	}));
	const logsQuery = createQuery(() => ({
		...queries.serviceLogs(id, ui.logCount, ui.logType),
		enabled: !!detail.data
	}));
	const healthHistory = $derived(healthQuery.data ?? []);
	const deploys = $derived(deployQuery.data?.data ?? []);
	const deployTotal = $derived(deployQuery.data?.total ?? 0);
	const logLines = $derived(logsQuery.data?.lines ?? []);

	const loading = $derived(detail.isPending);
	const logError = $derived(message(logsQuery.error));
	const statusError = $derived(message(statusQuery.error));
	const checkingStatus = $derived(statusQuery.isFetching);
	const logsLoading = $derived(logsQuery.isFetching);
	const healthLoading = $derived(healthQuery.isFetching);
	const healthError = $derived(message(healthQuery.error));
	// Paging keeps the previous rows as a placeholder; only a first load replaces the list.
	const deploysLoading = $derived(deployQuery.isPending);
	const deploysPaging = $derived(deployQuery.isPlaceholderData);
	const deploysError = $derived(message(deployQuery.error));
	// Runtime actions change status, health, logs and the owning watcher's service view.
	const actionMutation = createCommand<{ message: string }>(() =>
		watcher ? serviceScope(id, watcher.id) : [keys.service(id), keys.services]
	);
	const saveMutation = createMutation(() => ({
		mutationFn: (v: import('$lib/api').ServiceWritePayload) =>
			api.updateService(watcher!.id, id, v),
		onSuccess: (value) => {
			const target = queries.service(value.id).queryKey;
			const owner = client.getQueryData(target)?.watcher;
			if (owner) client.setQueryData(target, { service: value, watcher: owner });
			return invalidate(client, ...watcherScope(value.watcher_id));
		}
	}));
	const deleteMutation = createMutation(() => ({
		mutationFn: () => api.deleteService(watcher!.id, id),
		onSuccess: () => invalidate(client, ...watcherScope(watcher!.id))
	}));
	const restartMutation = createCommand<boolean>();

	const configDirty = $derived(
		configFingerprint(ui.envContent, ui.configFiles) !==
			configFingerprint(ui.baselineEnv, ui.baselineFiles)
	);
	const savingEnv = $derived(saveMutation.isPending);

	const deleting = $derived(deleteMutation.isPending);
	const pendingAction = $derived(
		actionMutation.isPending ? (actionMutation.variables?.name ?? '') : ''
	);
	const restartingEnv = $derived(restartMutation.isPending);

	const actionBusy = $derived(
		Boolean(pendingAction) ||
			savingEnv ||
			restartingEnv ||
			deleting ||
			watcher?.status === 'deploying'
	);

	const candidateURL = $derived(
		watcher
			? resolve('/watchers/[id]', { id: String(watcher.id) }) +
					`?tab=candidates&service=${id}#candidate-service-${id}`
			: ''
	);
	let draftServiceID = 0;
	$effect(() => {
		const value = detail.data;
		if (value)
			untrack(() => {
				if (draftServiceID !== value.service.id) {
					draftServiceID = value.service.id;
					resetConfiguration(value.service);
				} else if (configDirty)
					ui.configConflict =
						configFingerprint(value.service.env_content || '', value.service.config_files || []) !==
						configFingerprint(ui.baselineEnv, ui.baselineFiles);
				else resetConfiguration(value.service);
			});
	});
	$effect(() => {
		if (detail.data && page.url.searchParams.get('tab') === 'candidates')
			void untrack(() => openCandidates(true));
	});

	async function openCandidates(replaceState = false) {
		if (!candidateURL) return;
		try {
			await goto(
				resolve(`/watchers/[id]?tab=candidates&service=${id}#candidate-service-${id}`, {
					id: String(watcher!.id)
				}),
				{ replaceState }
			);
		} catch (cause) {
			ui.error = cause instanceof Error ? cause.message : 'Could not open deployment candidates';
		}
	}

	async function loadDetail() {
		await detail.refetch();
	}
	async function loadHealthHistory() {
		await healthQuery.refetch();
	}
	// Page, size, stream and line-count changes fetch through their keys.
	async function loadDeploys() {
		await deployQuery.refetch();
	}
	async function loadLogs() {
		await logsQuery.refetch();
	}

	function resetConfiguration(value = service) {
		ui.baselineEnv = value?.env_content || '';
		ui.baselineFiles = copyFiles(value?.config_files || []);
		ui.envContent = ui.baselineEnv;
		ui.configFiles = copyFiles(ui.baselineFiles);
		ui.configConflict = false;
	}

	async function runAction(name: string, fn: () => Promise<{ message: string }>) {
		if (pendingAction) return;
		ui.error = '';
		try {
			const res = await actionMutation.mutateAsync({ name, run: fn });
			ui.actionMsg = res.message;
			setTimeout(() => (ui.actionMsg = ''), 4000);
		} catch (e) {
			ui.error = e instanceof Error ? e.message : 'Action failed';
		}
	}

	async function saveEnv() {
		if (!service || !watcher || savingEnv) return false;
		const problems = configurationProblems(service.env_file, ui.configFiles);
		if (problems.length) {
			ui.error = problems.join(' ');
			return false;
		}
		ui.error = '';
		try {
			const saved = await saveMutation.mutateAsync({
				env_content: ui.envContent,
				config_files: copyFiles(ui.configFiles)
			});
			if (saved.id !== id) return true;
			resetConfiguration(saved);
			ui.actionMsg = 'Service files saved';
			setTimeout(() => (ui.actionMsg = ''), 4000);
			return true;
		} catch (e) {
			ui.error = e instanceof Error ? e.message : 'Failed to save env';
			return false;
		}
	}

	async function deleteService() {
		if (!service || !watcher || deleting) return;
		ui.error = '';
		try {
			await deleteMutation.mutateAsync();
			ui.showDeleteDialog = false;
			await goto(resolve(`/watchers/${watcher.id}/edit#services`));
			forget(client, keys.service(id));
		} catch (e) {
			ui.error = e instanceof Error ? e.message : 'Failed to delete service';
			setTimeout(() => (ui.actionMsg = ''), 5000);
		}
	}

	async function refreshStatus(_force = false) {
		await statusQuery.refetch();
		await healthQuery.refetch();
	}
	return {
		ui,
		get configDirty() {
			return configDirty;
		},
		get service() {
			return service;
		},
		get watcher() {
			return watcher;
		},
		get id() {
			return id;
		},
		get actionBusy() {
			return actionBusy;
		},
		get pendingAction() {
			return pendingAction;
		},
		runAction,
		get detail() {
			return detail;
		},
		loadDetail,
		get loading() {
			return loading;
		},
		get checkingStatus() {
			return checkingStatus;
		},
		refreshStatus,
		get statusError() {
			return statusError;
		},
		openCandidates,
		get deployTotal() {
			return deployTotal;
		},
		get healthError() {
			return healthError;
		},
		loadHealthHistory,
		get healthLoading() {
			return healthLoading;
		},
		get healthHistory() {
			return healthHistory;
		},
		get logsLoading() {
			return logsLoading;
		},
		get logLines() {
			return logLines;
		},
		get logError() {
			return logError;
		},
		loadLogs,
		resetConfiguration,
		get savingEnv() {
			return savingEnv;
		},
		get restartingEnv() {
			return restartingEnv;
		},
		saveEnv,
		get restartMutation() {
			return restartMutation;
		},
		get deploysError() {
			return deploysError;
		},
		loadDeploys,
		get deploysLoading() {
			return deploysLoading;
		},
		get deploysPaging() {
			return deploysPaging;
		},
		get deploys() {
			return deploys;
		},
		get candidateURL() {
			return candidateURL;
		},
		get deleting() {
			return deleting;
		},
		deleteService
	};
}
