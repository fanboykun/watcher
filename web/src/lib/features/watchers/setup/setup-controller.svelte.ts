import {
	draftFromInspect,
	defaultService,
	repoNameFromURL,
	serviceTypeFromAppKind,
	iisKindFromAppKind
} from './setup-model';
import { steps, iisAppKinds } from './setup-options';
import {
	feedAuthTypeFor,
	sourceLocationLabel,
	takesFeedCredentials
} from '$lib/features/watchers/presentation';
import { buildCreatePayload } from './setup-payload';
import { createMutation, useQueryClient } from '@tanstack/svelte-query';
import { keys } from '$lib/query/options';
import { invalidate, message } from '$lib/query/client';
import { configurationProblems } from '$lib/features/configuration/model';
import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import {
	api,
	isIISService,
	type InspectRepoResponse,
	type FeedAuthDraft,
	type InspectServiceTarget,
	type ReleaseSourceType,
	type Service
} from '$lib/api';
import { type WebhookSelectionState } from '$lib/webhooks';
import { Check } from '@lucide/svelte';
export function createWatcherSetup() {
	type WizardStep = 1 | 2 | 3 | 4;
	type ServiceDraft = Partial<Service>;

	const ui = $state({
		currentStep: 1 as WizardStep,
		inspectResult: null as InspectRepoResponse | null,
		selectedInspectService: '',
		error: '',
		sourceType: 'github' as ReleaseSourceType,
		feedAuth: { type: '', username: '', header: '', region: '', secret: '' } as FeedAuthDraft,
		repoURL: '',
		releaseRef: 'latest',
		inspectGitHubToken: '',
		useInspectGitHubToken: false,
		watcherName: '',
		watcherServiceName: '',
		metadataURL: '',
		installDir: '',
		checkInterval: 60,
		healthChecksEnabled: false,
		healthCheckURL: '',
		deploymentEnvironment: '',
		watcherGitHubToken: '',
		useWatcherGitHubToken: false,
		autoDeploy: true,
		serviceDrafts: [] as ServiceDraft[],
		webhookEnabled: false,
		webhookURL: '',
		webhookSigningSecret: '',
		useCustomWebhookSigningSecret: false,
		webhookSelections: {
			notify_version_found: false,
			notify_deployment_succeeded: false,
			notify_deployment_failed: false,
			notify_rollback_succeeded: false,
			notify_rollback_failed: false,
			notify_service_health_changed: false
		} as WebhookSelectionState
	});

	const client = useQueryClient();
	const inspectMutation = createMutation(() => ({
		mutationFn: (v: { url: string; ref: string; token: string; sourceType: ReleaseSourceType }) =>
			api.inspectRepo(
				v.url,
				v.ref,
				v.token,
				v.sourceType,
				takesFeedCredentials(v.sourceType)
					? { ...ui.feedAuth, type: feedAuthTypeFor(v.sourceType, ui.feedAuth.type) }
					: undefined
			)
	}));
	const createMutationState = createMutation(() => ({
		mutationFn: api.createWatcher,
		onSuccess: () => invalidate(client, keys.watchers, keys.services, keys.status)
	}));
	const triggerMutation = createMutation(() => ({ mutationFn: api.triggerCheck }));
	const inspecting = $derived(inspectMutation.isPending);
	const creating = $derived(createMutationState.isPending || triggerMutation.isPending);

	function inspectServices(): InspectServiceTarget[] {
		return Object.values(ui.inspectResult?.services || {});
	}

	function buildDraftFromInspect(name: string) {
		return draftFromInspect(name, ui.inspectResult?.services?.[name], ui.healthCheckURL);
	}

	function selectInspectTarget(name: string) {
		const target = ui.inspectResult?.services?.[name];
		if (!target) return;
		ui.selectedInspectService = name;
		ui.watcherServiceName = name;
		ui.watcherName = ui.watcherName || target.windows_service_name || name;
		ui.installDir = ui.installDir || `C:\\apps\\${name}`;
		if (ui.inspectResult?.source === 'manifest' && ui.inspectResult.metadata_url) {
			ui.metadataURL = ui.inspectResult.metadata_url;
		}
		if (target.health_check_url) {
			ui.healthCheckURL = target.health_check_url;
			ui.healthChecksEnabled = true;
		}
		const draft = buildDraftFromInspect(name);
		if (!draft) return;
		if (ui.serviceDrafts.length === 0) {
			ui.serviceDrafts = [draft];
			return;
		}
		ui.serviceDrafts = [draft, ...ui.serviceDrafts.slice(1)];
	}

	async function inspectRepo() {
		const github = ui.sourceType === 'github';
		if (!ui.repoURL.trim()) {
			ui.error = `${sourceLocationLabel(ui.sourceType)} is required.`;
			return false;
		}
		if (github && ui.useInspectGitHubToken && !ui.inspectGitHubToken.trim()) {
			ui.error = 'Inspect token override is enabled but empty.';
			return false;
		}
		ui.error = '';
		try {
			const inputURL = ui.repoURL.trim();
			const resolvedReleaseRef = ui.releaseRef.trim() || 'latest';
			const token = github && ui.useInspectGitHubToken ? ui.inspectGitHubToken.trim() : '';
			ui.inspectResult = await inspectMutation.mutateAsync({
				url: inputURL,
				ref: resolvedReleaseRef,
				token,
				sourceType: ui.sourceType
			});

			const repoName = repoNameFromURL(ui.inspectResult.repo_url || inputURL);
			ui.watcherName = ui.watcherName || repoName;
			ui.watcherServiceName = ui.watcherServiceName || repoName;
			ui.installDir = ui.installDir || `C:\\apps\\${repoName}`;
			ui.metadataURL =
				ui.inspectResult.source === 'manifest' && ui.inspectResult.metadata_url
					? ui.inspectResult.metadata_url
					: ui.inspectResult.repo_url || inputURL;

			const services = inspectServices();
			if (services.length === 1) {
				selectInspectTarget(services[0].name);
			} else if (ui.serviceDrafts.length === 0) {
				ui.serviceDrafts = [createDefaultServiceDraft()];
			}
			return true;
		} catch (e) {
			ui.error = e instanceof Error ? e.message : 'Inspect failed';
			return false;
		}
	}

	function createDefaultServiceDraft() {
		return defaultService(ui.watcherServiceName.trim() || 'app', ui.healthCheckURL);
	}

	function addServiceDraft() {
		const name = `${ui.watcherServiceName.trim() || 'app'}-${ui.serviceDrafts.length + 1}`;
		ui.serviceDrafts = [...ui.serviceDrafts, defaultService(name, ui.healthCheckURL)];
	}

	function removeServiceDraft(index: number) {
		ui.serviceDrafts = ui.serviceDrafts.filter((_, i) => i !== index);
	}

	function validateStep(step: WizardStep): boolean {
		switch (step) {
			case 1:
				if (!ui.inspectResult) {
					ui.error = 'Inspect the repository before continuing.';
					return false;
				}
				return true;
			case 2:
				if (inspectServices().length > 0 && !ui.selectedInspectService) {
					ui.error = 'Choose one detected deploy target before continuing.';
					return false;
				}
				if (!ui.watcherName.trim()) {
					ui.error = 'Watcher name is required.';
					return false;
				}
				if (!ui.watcherServiceName.trim()) {
					ui.error = 'App/service name ID is required.';
					return false;
				}
				if (!ui.metadataURL.trim()) {
					ui.error = 'Metadata URL is required.';
					return false;
				}
				if (!ui.installDir.trim()) {
					ui.error = 'Install directory is required.';
					return false;
				}
				if (ui.checkInterval < 10) {
					ui.error = 'Check interval must be at least 10 seconds.';
					return false;
				}
				return true;
			case 3:
				if (ui.serviceDrafts.length === 0) {
					ui.error = 'At least one service definition is required.';
					return false;
				}
				for (const [index, svc] of ui.serviceDrafts.entries()) {
					if (!(svc.windows_service_name || '').trim()) {
						ui.error = `Service #${index + 1} is missing its Windows service name.`;
						return false;
					}
					if (!isIISService(svc.service_type || 'nssm') && !(svc.binary_name || '').trim()) {
						ui.error = `Service #${index + 1} is missing its executable name.`;
						return false;
					}
					const problems = configurationProblems(svc.env_file || '', svc.config_files || []);
					if (problems.length) {
						ui.error = `Service #${index + 1}: ${problems.join(' ')}`;
						return false;
					}
				}
				return true;
			case 4:
				return true;
		}
	}

	async function goNext() {
		ui.error = '';
		if (ui.currentStep === 1) {
			const ok = await inspectRepo();
			if (ok) ui.currentStep = 2;
			return;
		}
		if (!validateStep(ui.currentStep)) {
			return;
		}
		ui.currentStep = (ui.currentStep + 1) as WizardStep;
	}

	function goBack() {
		ui.error = '';
		ui.currentStep = (ui.currentStep - 1) as WizardStep;
	}

	function goToStep(target: WizardStep) {
		if (target <= ui.currentStep) {
			ui.error = '';
			ui.currentStep = target;
			return;
		}
		for (let step = ui.currentStep; step < target; step += 1) {
			if (!validateStep(step as WizardStep)) {
				return;
			}
		}
		ui.currentStep = target;
	}

	async function createWatcher() {
		ui.error = '';
		if (!validateStep(2) || !validateStep(3) || !validateStep(4)) {
			return;
		}
		try {
			const watcher = await createMutationState.mutateAsync(buildCreatePayload(ui));
			try {
				await triggerMutation.mutateAsync(watcher.id);
			} catch {
				// Ignore trigger failure so the user can still land on the watcher detail page.
			}
			await goto(resolve(`/watchers/${watcher.id}?tab=services`));
		} catch (e) {
			ui.error = e instanceof Error ? e.message : 'Failed to create watcher';
		}
	}
	return {
		ui,
		get steps() {
			return steps;
		},
		goToStep,
		get inspecting() {
			return inspecting;
		},
		get creating() {
			return creating;
		},
		inspectServices,
		selectInspectTarget,
		addServiceDraft,
		removeServiceDraft,
		get iisAppKinds() {
			return iisAppKinds;
		},
		goBack,
		goNext,
		createWatcher
	};
}
