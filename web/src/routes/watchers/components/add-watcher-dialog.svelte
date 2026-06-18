<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Button from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Select from '$lib/components/ui/select/index.js';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import WebhookEventReference from '$lib/components/webhook-event-reference.svelte';
	import { Trash2, Plus, ArrowRight, Check } from '@lucide/svelte';
	import { resolve } from '$app/paths';
	import { goto } from '$app/navigation';
	import {
		api,
		iisAppKindLabel,
		isIISService,
		serviceTypeLabel,
		type IISAppKind,
		type InspectRepoResponse,
		type InspectServiceTarget,
		type ConfigFileTarget,
		type Service
	} from '$lib/api';
	import type { WebhookSelectionState } from '$lib/webhooks';

	let {
		open = $bindable(false),
		onWatcherCreated
	}: {
		open: boolean;
		onWatcherCreated: () => Promise<void> | void;
	} = $props();

	let createStep = $state(1);
	let inspectResult = $state<InspectRepoResponse | null>(null);
	let selectedInspectService = $state('');

	// Create form fields
	let formName = $state('');
	let formServiceName = $state('');
	let formMetadataURL = $state('');
	let formInstallDir = $state('');
	let formReleaseRef = $state('latest');
	let formInterval = $state(60);
	let formHcEnabled = $state(false);
	let formHcURL = $state('');
	let formDeploymentEnvironment = $state('');
	let formGitHubToken = $state('');
	let useCustomGitHubToken = $state(false);
	let formWebhookEnabled = $state(false);
	let formWebhookURL = $state('');
	let formWebhookSigningSecret = $state('');
	let useCustomWebhookSigningSecret = $state(false);
	let webhookSelections = $state<WebhookSelectionState>({
		notify_version_found: false,
		notify_deployment_succeeded: false,
		notify_deployment_failed: false,
		notify_rollback_succeeded: false,
		notify_rollback_failed: false,
		notify_service_health_changed: false
	});
	let formServices = $state<Partial<Service>[]>([]);

	let inspecting = $state(false);
	let creating = $state(false);
	let error = $state('');

	const iisAppKinds: Array<{ value: IISAppKind; label: string; hint: string }> = [
		{ value: 'static', label: 'Static Site', hint: 'HTML/CSS/JS or frontend build output served by IIS.' },
		{ value: 'php', label: 'PHP', hint: 'PHP app hosted by IIS with FastCGI/PHP already installed.' },
		{ value: 'aspnet_classic', label: 'ASP.NET Classic', hint: 'Classic ASP.NET app using the .NET CLR app pool.' }
	];

	$effect(() => {
		if (open) {
			resetForm();
		}
	});

	function inspectServices(): InspectServiceTarget[] {
		return Object.values(inspectResult?.services || {});
	}

	function repoNameFromURL(raw: string) {
		const cleaned = raw.split('/releases')[0].replace(/\/$/, '');
		const parts = cleaned.split('/');
		return parts[parts.length - 1] || 'my-app';
	}

	function serviceTypeFromAppKind(appKind: string | undefined) {
		switch ((appKind || 'nssm').toLowerCase()) {
			case 'static':
			case 'php':
			case 'aspnet_classic':
				return 'iis';
			default:
				return 'nssm';
		}
	}

	function iisKindFromAppKind(appKind: string | undefined): IISAppKind {
		switch ((appKind || 'static').toLowerCase()) {
			case 'php':
				return 'php';
			case 'aspnet_classic':
				return 'aspnet_classic';
			default:
				return 'static';
		}
	}

	function selectInspectTarget(name: string) {
		const target = inspectResult?.services?.[name];
		if (!target) return;

		selectedInspectService = name;
		formServiceName = name;
		formName = target.windows_service_name || name;
		formInstallDir = `C:\\apps\\${name}`;
		if (inspectResult?.source === 'manifest' && inspectResult.metadata_url) {
			formMetadataURL = inspectResult.metadata_url;
		}
		if (target.health_check_url) {
			formHcURL = target.health_check_url;
			formHcEnabled = true;
		}

		const serviceType = serviceTypeFromAppKind(target.app_kind);
		formServices = [{
			service_type: serviceType,
			windows_service_name: target.windows_service_name || name,
			binary_name: serviceType === 'nssm' ? (target.binary_name || `${name}.exe`) : '',
			start_arguments: target.start_arguments || '',
			env_file: serviceType === 'nssm' ? (target.env_file || '.env') : '',
			env_content: '',
			iis_app_kind: iisKindFromAppKind(target.app_kind),
			iis_app_pool: target.iis_app_pool || target.windows_service_name || name,
			iis_site_name: target.iis_site_name || target.windows_service_name || name,
			iis_managed_runtime: target.iis_managed_runtime || '',
			public_url: target.public_url || '',
			config_files: [],
			health_check_url: target.health_check_url || formHcURL,
		}];
	}

	async function inspectRepo() {
		if (!formMetadataURL) return;
		if (useCustomGitHubToken && !formGitHubToken.trim()) {
			error = 'Custom GitHub token is enabled but empty.';
			return;
		}
		inspecting = true;
		error = '';
		try {
			const inputURL = formMetadataURL.trim();
			const releaseRef = formReleaseRef.trim() || 'latest';
			const token = useCustomGitHubToken ? formGitHubToken.trim() : '';
			inspectResult = await api.inspectRepo(inputURL, releaseRef, token);
			formMetadataURL = inspectResult.source === 'manifest' && inspectResult.metadata_url ? inspectResult.metadata_url : (inspectResult.repo_url || inputURL);

			const repoName = repoNameFromURL(inspectResult.repo_url || inputURL);
			formName = repoName;
			formServiceName = repoName;
			formInstallDir = `C:\\apps\\${repoName}`;
			selectedInspectService = '';
			formServices = [];

			const services = inspectServices();
			if (services.length === 1) {
				selectInspectTarget(services[0].name);
			}
			
			createStep = 2;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Inspect failed';
		} finally {
			inspecting = false;
		}
	}

	function jumpToNext() {
		if (inspectServices().length > 0 && !selectedInspectService) {
			error = 'Choose one deploy target before continuing.';
			return;
		}
		error = '';
		createStep = 3;
		if (formServices.length === 0) {
			formServices = [{
				service_type: 'nssm',
				windows_service_name: formServiceName,
				binary_name: formServiceName ? `${formServiceName}.exe` : 'app.exe',
				start_arguments: '',
				env_file: '.env',
				env_content: '',
				iis_app_kind: 'static',
				iis_app_pool: '',
				iis_site_name: '',
				iis_managed_runtime: '',
				public_url: '',
				config_files: [],
				health_check_url: formHcURL,
			}];
		}
	}

	async function createWatcherAndServices() {
		creating = true;
		error = '';
		try {
			const w = await api.createWatcher({
				name: formName,
				service_name: formServiceName,
				metadata_url: formMetadataURL,
				release_ref: formReleaseRef.trim() || 'latest',
				deployment_environment: formDeploymentEnvironment,
				github_token: useCustomGitHubToken ? formGitHubToken.trim() : '',
				webhook_enabled: formWebhookEnabled,
				webhook_url: formWebhookURL,
				webhook_signing_secret: useCustomWebhookSigningSecret ? formWebhookSigningSecret.trim() : '',
				notify_version_found: webhookSelections.notify_version_found,
				notify_deployment_succeeded: webhookSelections.notify_deployment_succeeded,
				notify_deployment_failed: webhookSelections.notify_deployment_failed,
				notify_rollback_succeeded: webhookSelections.notify_rollback_succeeded,
				notify_rollback_failed: webhookSelections.notify_rollback_failed,
				notify_service_health_changed: webhookSelections.notify_service_health_changed,
				install_dir: formInstallDir,
				check_interval_sec: formInterval,
				hc_enabled: formHcEnabled,
				hc_url: formHcURL
			});
			
			for (const s of formServices) {
				await api.createService(w.id, s);
			}

			try {
				await api.triggerCheck(w.id);
			} catch {
				// Let the detail page load even if the immediate trigger fails.
			}

			open = false;
			resetForm();
			if (onWatcherCreated) await onWatcherCreated();
			await goto(resolve(`/watchers/${w.id}?tab=deploys`));
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to create watcher';
		} finally {
			creating = false;
		}
	}

	function resetForm() {
		createStep = 1;
		formName = '';
		formServiceName = '';
		formMetadataURL = '';
		formInstallDir = '';
		formReleaseRef = 'latest';
		formInterval = 60;
		formHcEnabled = false;
		formHcURL = '';
		formDeploymentEnvironment = '';
		formGitHubToken = '';
		useCustomGitHubToken = false;
		formWebhookEnabled = false;
		formWebhookURL = '';
		formWebhookSigningSecret = '';
		useCustomWebhookSigningSecret = false;
		webhookSelections = {
			notify_version_found: false,
			notify_deployment_succeeded: false,
			notify_deployment_failed: false,
			notify_rollback_succeeded: false,
			notify_rollback_failed: false,
			notify_service_health_changed: false
		};
		formServices = [];
		inspectResult = null;
		selectedInspectService = '';
		error = '';
	}

	function addServiceDraft() {
		formServices = [...formServices, {
			service_type: 'nssm',
			windows_service_name: `${formServiceName}-extra`,
			binary_name: formServiceName ? `${formServiceName}-extra.exe` : 'app.exe',
			start_arguments: '',
			env_file: '.env',
			env_content: '',
			iis_app_kind: 'static',
			iis_app_pool: '',
			iis_site_name: '',
			iis_managed_runtime: '',
			public_url: '',
			config_files: [],
		}];
	}
	
	function removeServiceDraft(idx: number) {
		formServices = formServices.filter((_, i) => i !== idx);
	}

	function addConfigFileDraft(serviceIndex: number) {
		const next = [...formServices];
		const svc = next[serviceIndex];
		const configFiles = [...(svc.config_files || []), { file_path: '', target: 'app_dir' as ConfigFileTarget, content: '' }];
		next[serviceIndex] = { ...svc, config_files: configFiles };
		formServices = next;
	}

	function removeConfigFileDraft(serviceIndex: number, fileIndex: number) {
		const next = [...formServices];
		const svc = next[serviceIndex];
		const configFiles = [...(svc.config_files || [])];
		configFiles.splice(fileIndex, 1);
		next[serviceIndex] = { ...svc, config_files: configFiles };
		formServices = next;
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="max-h-[90vh] w-[min(96vw,42rem)] overflow-hidden p-0 sm:max-w-2xl">
		<form
			class="flex max-h-[calc(90vh-5.5rem)] flex-col"
			onsubmit={(e) => {
				e.preventDefault();
				if (createStep === 1) inspectRepo();
				else if (createStep === 2) jumpToNext();
				else createWatcherAndServices();
			}}
		>
			<Dialog.Header class="shrink-0 border-b border-border/70 px-6 pt-6 pb-4">
				<Dialog.Title>Add Watcher</Dialog.Title>
				<Dialog.Description>
					{#if createStep === 1}
						Step 1: Inspect GitHub Repository
					{:else if createStep === 2}
						Step 2: Configure general watcher settings
					{:else}
						Step 3: Define Services to be deployed
					{/if}
				</Dialog.Description>
			</Dialog.Header>
		
			<div class="flex-1 overflow-y-auto px-6 py-5">
				{#if error}
					<div class="mb-4 rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-400">
						{error}
					</div>
				{/if}

				{#if createStep === 1}
					<div class="space-y-4">
						<div class="grid gap-4 sm:grid-cols-[1fr_180px]">
							<div class="space-y-3">
								<Label for="metadataURL">GitHub Repository or version.json URL</Label>
								<Input
									id="metadataURL"
									placeholder="https://github.com/org/repo"
									bind:value={formMetadataURL}
									required
								/>
							</div>
							<div class="space-y-3">
								<Label for="releaseRefInitial">Release Ref</Label>
								<Input id="releaseRefInitial" placeholder="latest" bind:value={formReleaseRef} />
							</div>
						</div>
						<div class="space-y-2 rounded-md border border-border bg-muted/20 p-3">
							<div class="flex items-center gap-2 mb-2">
								<Checkbox id="useCustomGitHubToken" bind:checked={useCustomGitHubToken} />
								<Label for="useCustomGitHubToken" class="text-sm select-none">
									Use custom GitHub token for this watcher
								</Label>
							</div>
							<Input
								id="watcherGithubTokenStep1"
								type="password"
								placeholder="ghp_xxx"
								bind:value={formGitHubToken}
								disabled={!useCustomGitHubToken}
							/>
							<p class="text-xs text-muted-foreground mt-1">
								Useful for private repos or when global <code>GITHUB_TOKEN</code> cannot access this repo.
							</p>
							<p class="text-xs text-muted-foreground">
								Fine-grained PAT minimum: <code>Contents: Read</code>. If deployment status reporting is used, also add <code>Deployments: Read and write</code>.
							</p>
							<div class="space-y-1 rounded border border-border bg-background/60 p-2 text-xs text-muted-foreground mt-2">
								<p class="font-medium text-foreground/90">Org private repo checklist</p>
								<p>1. Token has access to the target org repo(s).</p>
								<p>2. SSO/SAML authorization for this token is approved.</p>
								<p>3. Token owner has repo/team access in the org.</p>
								<p>4. Org policy allows PAT type being used (fine-grained/classic).</p>
								<p>5. Repo has at least one published release (not only draft/prerelease).</p>
							</div>
						</div>
						<p class="text-xs text-muted-foreground">
							Manifest-first setup: Watcher will try <code>version.json</code> for the selected ref and use repo asset discovery only as a legacy fallback.
						</p>
					</div>
				{:else if createStep === 2}
					<div class="space-y-4">
						<div class="flex items-center justify-between rounded border bg-muted/30 p-3 text-sm">
							<div>
								<span class="font-medium">Detected:</span> {inspectResult?.latest_version || 'Unknown tag'}
							</div>
							<div class="text-muted-foreground">
								{inspectResult?.assets?.length || 0} assets attached
							</div>
						</div>

						<div class="space-y-3 rounded border border-border/70 bg-muted/20 p-3">
							<div class="flex items-center justify-between gap-3 text-sm">
								<div>
									<p class="font-medium">Deploy Target</p>
									<p class="text-xs text-muted-foreground">
										{inspectResult?.source === 'manifest' ? 'Loaded from version.json. Choose one service for this watcher.' : 'Legacy repo asset discovery. Review generated fields before saving.'}
									</p>
								</div>
								<span class="rounded border border-border px-2 py-1 text-xs text-muted-foreground">
									{inspectResult?.source === 'manifest' ? 'version.json' : 'repo assets'}
								</span>
							</div>

							{#if inspectServices().length > 0}
								<div class="grid gap-2 sm:grid-cols-2">
									{#each inspectServices() as target (target.name)}
										<button
											type="button"
											class={`rounded border p-3 text-left text-sm transition ${selectedInspectService === target.name ? 'border-primary bg-primary/10' : 'border-border bg-background/70 hover:bg-muted/40'}`}
											onclick={() => selectInspectTarget(target.name)}
										>
											<div class="flex items-start justify-between gap-2">
												<span class="font-medium">{target.name}</span>
												<span class="text-xs text-muted-foreground">{target.version}</span>
											</div>
											<p class="mt-1 truncate font-mono text-xs text-muted-foreground">{target.artifact}</p>
											<p class="mt-2 text-xs text-muted-foreground">
												{serviceTypeLabel(serviceTypeFromAppKind(target.app_kind))}
												{#if serviceTypeFromAppKind(target.app_kind) === 'nssm'}
													- {target.binary_name || `${target.name}.exe`}
												{:else}
													- {iisAppKindLabel(target.app_kind || 'static')}
												{/if}
											</p>
										</button>
									{/each}
								</div>
							{:else}
								<p class="text-sm text-muted-foreground">No deployable targets were detected.</p>
							{/if}
						</div>

						<div class="grid gap-4 sm:grid-cols-2">
							<div class="space-y-2">
								<Label for="name">Watcher Display Name</Label>
								<Input id="name" placeholder="my-app" bind:value={formName} required />
							</div>
							<div class="space-y-2">
								<Label for="serviceName">App/Service Name ID</Label>
								<Input id="serviceName" placeholder="my-app" bind:value={formServiceName} required />
							</div>
						</div>

						<div class="space-y-2">
							<Label for="installDir">Base Install Directory (auto extracts zip here)</Label>
							<Input id="installDir" placeholder="C:\apps\my-app" bind:value={formInstallDir} required />
						</div>

						<div class="grid gap-4 sm:grid-cols-2">
							<div class="space-y-2">
								<Label for="interval">Check Interval (sec)</Label>
								<Input id="interval" type="number" min="10" bind:value={formInterval} />
							</div>
							<div class="space-y-2">
								<Label for="hcURL">Global Health Check URL (optional)</Label>
								<Input id="hcURL" placeholder="http://localhost:3000/health" bind:value={formHcURL} />
							</div>
						</div>

						<div class="grid gap-4 sm:grid-cols-2">
							<div class="space-y-2">
								<Label for="deploymentEnvironment">Deployment Environment (GitHub)</Label>
								<Input id="deploymentEnvironment" placeholder="production" bind:value={formDeploymentEnvironment} />
								<p class="text-xs text-muted-foreground">Optional. Falls back to global `ENVIRONMENT` if empty.</p>
							</div>
							<div class="space-y-2">
								<Label for="releaseRef">Release Ref</Label>
								<Input id="releaseRef" placeholder="latest or v1.2.3" bind:value={formReleaseRef} />
								<p class="text-xs text-muted-foreground">Use <code>latest</code> for normal tracking, or pin this watcher to a specific release tag.</p>
							</div>
						</div>
						<div class="grid gap-4 sm:grid-cols-2">
							<div class="space-y-2 text-xs text-muted-foreground">
								<p>GitHub token mode:</p>
								<p class="font-medium">{useCustomGitHubToken && formGitHubToken.trim() ? 'Custom watcher token configured' : 'Using global GITHUB_TOKEN'}</p>
							</div>
						</div>

						<div class="rounded-lg border border-border/60 p-4 space-y-4">
							<div class="flex items-center gap-2">
								<Checkbox id="webhookEnabled" bind:checked={formWebhookEnabled} />
								<Label for="webhookEnabled" class="select-none">Enable webhook delivery for this watcher</Label>
							</div>
							<div class="grid gap-4 sm:grid-cols-2">
								<div class="space-y-2">
									<Label for="webhookURL">Webhook URL</Label>
									<Input id="webhookURL" placeholder="https://example.com/hooks/watcher" bind:value={formWebhookURL} />
									<p class="text-xs text-muted-foreground">Leave empty to inherit the global default URL.</p>
								</div>
								<div class="space-y-2">
									<Label for="webhookSigningSecret">Webhook Signing Secret Override</Label>
									<Input id="webhookSigningSecret" type="password" placeholder="whsec_..." bind:value={formWebhookSigningSecret} disabled={!useCustomWebhookSigningSecret} />
									<div class="flex items-center gap-2 mt-2">
										<Checkbox id="useCustomWebhookSigningSecret" bind:checked={useCustomWebhookSigningSecret} />
										<Label for="useCustomWebhookSigningSecret" class="text-sm select-none">Use watcher-specific webhook signing secret</Label>
									</div>
									<p class="text-xs text-muted-foreground">Standard Webhooks requires a <code>whsec_...</code> signing secret.</p>
								</div>
							</div>
							<WebhookEventReference
								title="Webhook Event Subscriptions"
								description="Choose which watcher events should be delivered. Each event entry explains when it fires and what payload fields receivers should expect."
								bind:selections={webhookSelections}
								showSelection={true}
							/>
						</div>

						<div class="flex items-center gap-2 mt-4">
							<Checkbox id="hcEnabled" bind:checked={formHcEnabled} />
							<Label for="hcEnabled" class="select-none">Enable Health Checks across services</Label>
						</div>
					</div>
				{:else if createStep === 3}
					<div class="space-y-4">
						{#each formServices as svc, i (i)}
							<div class="relative space-y-3 rounded-md border bg-card p-3">
								<Button.Root variant="ghost" size="icon" class="absolute top-2 right-2 h-6 w-6 text-red-400" type="button" onclick={() => removeServiceDraft(i)}>
									<Trash2 class="h-3 w-3" />
								</Button.Root>
								<div class="text-sm font-medium">Service #{i+1}</div>

								<div class="grid gap-3 sm:grid-cols-2">
									<div class="space-y-1">
										<Label class="text-xs">Hosting Mode</Label>
										<Select.Root type="single" bind:value={svc.service_type}>
											<Select.Trigger class="h-8 text-xs">
												{isIISService(svc.service_type || 'nssm') ? 'IIS Site' : 'NSSM Native Windows'}
											</Select.Trigger>
											<Select.Content>
												<Select.Item value="nssm" label="NSSM Native Windows">NSSM Native Windows</Select.Item>
												<Select.Item value="iis" label="IIS Site">IIS Site</Select.Item>
											</Select.Content>
										</Select.Root>
									</div>
									<div class="space-y-1">
										<Label class="text-xs">{isIISService(svc.service_type || 'nssm') ? 'Service Identifier' : 'Windows Service Name'}</Label>
										<Input class="h-8 text-xs" bind:value={svc.windows_service_name} placeholder="myapp-web" />
									</div>
									{#if !isIISService(svc.service_type || 'nssm')}
										<div class="space-y-1">
											<Label class="text-xs">Executable Name</Label>
											<Input class="h-8 text-xs" bind:value={svc.binary_name} placeholder="myapp.exe" />
										</div>
										<div class="space-y-1">
											<Label class="text-xs">Start Arguments</Label>
											<Input class="h-8 text-xs" bind:value={svc.start_arguments} placeholder="serve --port 8080" />
										</div>
										<div class="space-y-1">
											<Label class="text-xs">Env file relative path</Label>
											<Input class="h-8 text-xs" bind:value={svc.env_file} placeholder=".env.prod" />
										</div>
										<div class="space-y-1 sm:col-span-2">
											<Label class="text-xs">Env content (optional)</Label>
											<Textarea
												class="min-h-[120px] font-mono text-xs text-blue-300"
												bind:value={svc.env_content}
												placeholder="KEY=VALUE&#10;API_PORT=3000"
											/>
											<p class="text-[11px] text-muted-foreground">
												If provided, watcher writes this to <code>{svc.env_file || '.env'}</code> in install dir.
											</p>
										</div>
									{:else}
										<div class="space-y-1 sm:col-span-2">
											<Label class="text-xs">IIS App Kind</Label>
											<Select.Root type="single" bind:value={svc.iis_app_kind}>
												<Select.Trigger class="h-8 text-xs">
													{iisAppKinds.find((kind) => kind.value === (svc.iis_app_kind || 'static'))?.label || 'Select kind'}
												</Select.Trigger>
												<Select.Content>
													{#each iisAppKinds as kind (kind.value)}
														<Select.Item value={kind.value} label={kind.label}>{kind.label}</Select.Item>
													{/each}
												</Select.Content>
											</Select.Root>
											<p class="text-[11px] text-muted-foreground">
												{iisAppKinds.find((kind) => kind.value === (svc.iis_app_kind || 'static'))?.hint}
											</p>
										</div>
										<div class="space-y-1">
											<Label class="text-xs">IIS App Pool</Label>
											<Input class="h-8 text-xs" bind:value={svc.iis_app_pool} placeholder="myapp-web" />
										</div>
										<div class="space-y-1">
											<Label class="text-xs">IIS Site Name</Label>
											<Input class="h-8 text-xs" bind:value={svc.iis_site_name} placeholder="myapp-web" />
										</div>
										<div class="space-y-1">
											<Label class="text-xs">Public URL</Label>
											<Input class="h-8 text-xs" bind:value={svc.public_url} placeholder="https://app.example.com" />
										</div>
										<div class="rounded-md border border-border/70 bg-muted/20 p-3 text-[11px] text-muted-foreground sm:col-span-2">
											<span class="font-medium text-foreground/90">{serviceTypeLabel(svc.service_type || 'nssm')}:</span>
											{iisAppKindLabel(String(svc.iis_app_kind || 'static'))}. Watcher will set the IIS app pool runtime automatically for this profile.
										</div>
									{/if}
									<div class="space-y-2 sm:col-span-2">
										<div class="flex items-center justify-between">
											<Label class="text-xs">Additional managed config files</Label>
											<Button.Root variant="outline" size="sm" type="button" class="h-7 px-2 text-xs" onclick={() => addConfigFileDraft(i)}>
												<Plus class="mr-1 h-3 w-3" /> Add file
											</Button.Root>
										</div>
										{#if (svc.config_files || []).length > 0}
											<div class="space-y-3 rounded-md border border-border/70 bg-background/50 p-3">
												{#each svc.config_files || [] as file, fileIndex (fileIndex)}
													<div class="space-y-2 rounded-md border border-border/60 bg-card/60 p-3">
														<div class="flex items-center justify-between">
															<Label class="text-xs">Config file #{fileIndex + 1}</Label>
															<Button.Root
																variant="ghost"
																size="icon"
																type="button"
																class="h-7 w-7 text-red-400 hover:text-red-300"
																onclick={() => removeConfigFileDraft(i, fileIndex)}
															>
																<Trash2 class="h-3 w-3" />
															</Button.Root>
														</div>
														<div class="grid gap-2 sm:grid-cols-[1fr_150px]">
															<Input class="h-8 text-xs" bind:value={file.file_path} placeholder="web.config or config/appsettings.json" />
															<Select.Root type="single" bind:value={file.target}>
																<Select.Trigger class="h-8 text-xs">
																	{file.target === 'release_dir' ? 'Current dir' : 'Service/app dir'}
																</Select.Trigger>
																<Select.Content>
																	<Select.Item value="app_dir" label="Service/app dir">Service/app dir</Select.Item>
																	<Select.Item value="release_dir" label="Current dir">Current dir</Select.Item>
																</Select.Content>
															</Select.Root>
														</div>
														<Textarea
															class="min-h-[120px] font-mono text-xs text-blue-300"
															bind:value={file.content}
															placeholder={'{\n  "port": 3000\n}'}
														/>
													</div>
												{/each}
											</div>
										{:else}
											<p class="text-[11px] text-muted-foreground">Use <code>Current dir</code> for IIS files like <code>web.config</code> that must sit beside deployed static assets.</p>
										{/if}
									</div>
								</div>
							</div>
						{/each}

						<Button.Root variant="outline" size="sm" type="button" onclick={addServiceDraft} class="mt-2 w-full border-dashed">
							<Plus class="mr-2 h-3 w-3" /> Add Service Definition
						</Button.Root>
					</div>
				{/if}
			</div>

			<Dialog.Footer class="shrink-0 border-t border-border/70 px-6 pt-4 pb-4">
				{#if createStep === 1}
					<Button.Root variant="outline" type="button" onclick={() => { open = false; resetForm(); }}>
						Cancel
					</Button.Root>
					<Button.Root type="submit" disabled={inspecting}>
						{inspecting ? 'Inspecting...' : 'Next'} <ArrowRight class="ml-2 h-4 w-4" />
					</Button.Root>
				{:else if createStep === 2}
					<Button.Root variant="outline" type="button" onclick={() => createStep = 1}>Back</Button.Root>
					<Button.Root type="submit">Continue <ArrowRight class="ml-2 h-4 w-4" /></Button.Root>
				{:else}
					<Button.Root variant="outline" type="button" onclick={() => createStep = 2}>Back</Button.Root>
					<Button.Root type="submit" disabled={creating}>
						{#if creating}
							Creating...
						{:else}
							<Check class="mr-2 h-4 w-4" /> Save Watcher & Services
						{/if}
					</Button.Root>
				{/if}
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
