<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import {
		api,
		iisAppKindLabel,
		isIISService,
		serviceTypeLabel,
		type ConfigFileTarget,
		type IISAppKind,
		type InspectRepoResponse,
		type InspectServiceTarget,
		type Service,
		type WatcherWritePayload
	} from '$lib/api';
	import * as Button from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Select from '$lib/components/ui/select/index.js';
	import { Textarea } from '$lib/components/ui/textarea';
	import { webhookDocsHref, type WebhookSelectionState } from '$lib/webhooks';
	import { AlertCircle, ArrowLeft, ArrowRight, BookOpenText, Check, ExternalLink, Plus, Trash2 } from '@lucide/svelte';

	type WizardStep = 1 | 2 | 3 | 4;
	type ServiceDraft = Partial<Service>;

	const steps: Array<{ id: WizardStep; label: string }> = [
		{ id: 1, label: 'Repository' },
		{ id: 2, label: 'Watcher' },
		{ id: 3, label: 'Services' },
		{ id: 4, label: 'Webhook' }
	];

	const iisAppKinds: Array<{ value: IISAppKind; label: string; hint: string }> = [
		{ value: 'static', label: 'Static Site', hint: 'HTML/CSS/JS or frontend build output served by IIS.' },
		{ value: 'php', label: 'PHP', hint: 'PHP app hosted by IIS with FastCGI/PHP already installed.' },
		{ value: 'aspnet_classic', label: 'ASP.NET Classic', hint: 'Classic ASP.NET app using the .NET CLR app pool.' }
	];

	let currentStep = $state<WizardStep>(1);
	let inspectResult = $state<InspectRepoResponse | null>(null);
	let selectedInspectService = $state('');
	let inspecting = $state(false);
	let creating = $state(false);
	let error = $state('');

	let repoURL = $state('');
	let releaseRef = $state('latest');
	let inspectGitHubToken = $state('');
	let useInspectGitHubToken = $state(false);

	let watcherName = $state('');
	let watcherServiceName = $state('');
	let metadataURL = $state('');
	let installDir = $state('');
	let checkInterval = $state(60);
	let healthChecksEnabled = $state(false);
	let healthCheckURL = $state('');
	let deploymentEnvironment = $state('');
	let watcherGitHubToken = $state('');
	let useWatcherGitHubToken = $state(false);

	let serviceDrafts = $state<ServiceDraft[]>([]);

	let webhookEnabled = $state(false);
	let webhookURL = $state('');
	let webhookBearerToken = $state('');
	let useCustomWebhookBearerToken = $state(false);
	let webhookSelections = $state<WebhookSelectionState>({
		notify_version_found: false,
		notify_deployment_succeeded: false,
		notify_deployment_failed: false,
		notify_rollback_succeeded: false,
		notify_rollback_failed: false,
		notify_service_health_changed: false
	});

	function repoNameFromURL(raw: string) {
		const cleaned = raw.split('/releases')[0].replace(/\/$/, '');
		const parts = cleaned.split('/');
		return parts[parts.length - 1] || 'my-app';
	}

	function inspectServices(): InspectServiceTarget[] {
		return Object.values(inspectResult?.services || {});
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

	function buildDraftFromInspect(name: string): ServiceDraft | null {
		const target = inspectResult?.services?.[name];
		if (!target) return null;
		const serviceType = serviceTypeFromAppKind(target.app_kind);
		return {
			service_type: serviceType,
			windows_service_name: target.windows_service_name || name,
			binary_name: serviceType === 'nssm' ? target.binary_name || `${name}.exe` : '',
			start_arguments: target.start_arguments || '',
			env_file: serviceType === 'nssm' ? target.env_file || '.env' : '',
			env_content: '',
			iis_app_kind: iisKindFromAppKind(target.app_kind),
			iis_app_pool: target.iis_app_pool || target.windows_service_name || name,
			iis_site_name: target.iis_site_name || target.windows_service_name || name,
			iis_managed_runtime: target.iis_managed_runtime || '',
			public_url: target.public_url || '',
			config_files: [],
			health_check_url: target.health_check_url || healthCheckURL
		};
	}

	function selectInspectTarget(name: string) {
		const target = inspectResult?.services?.[name];
		if (!target) return;
		selectedInspectService = name;
		watcherServiceName = name;
		watcherName = watcherName || target.windows_service_name || name;
		installDir = installDir || `C:\\apps\\${name}`;
		if (inspectResult?.source === 'manifest' && inspectResult.metadata_url) {
			metadataURL = inspectResult.metadata_url;
		}
		if (target.health_check_url) {
			healthCheckURL = target.health_check_url;
			healthChecksEnabled = true;
		}
		const draft = buildDraftFromInspect(name);
		if (!draft) return;
		if (serviceDrafts.length === 0) {
			serviceDrafts = [draft];
			return;
		}
		serviceDrafts = [draft, ...serviceDrafts.slice(1)];
	}

	async function inspectRepo() {
		if (!repoURL.trim()) {
			error = 'Repository URL or metadata URL is required.';
			return false;
		}
		if (useInspectGitHubToken && !inspectGitHubToken.trim()) {
			error = 'Inspect token override is enabled but empty.';
			return false;
		}
		inspecting = true;
		error = '';
		try {
			const inputURL = repoURL.trim();
			const resolvedReleaseRef = releaseRef.trim() || 'latest';
			const token = useInspectGitHubToken ? inspectGitHubToken.trim() : '';
			inspectResult = await api.inspectRepo(inputURL, resolvedReleaseRef, token);

			const repoName = repoNameFromURL(inspectResult.repo_url || inputURL);
			watcherName = watcherName || repoName;
			watcherServiceName = watcherServiceName || repoName;
			installDir = installDir || `C:\\apps\\${repoName}`;
			metadataURL =
				inspectResult.source === 'manifest' && inspectResult.metadata_url
					? inspectResult.metadata_url
					: (inspectResult.repo_url || inputURL);

			const services = inspectServices();
			if (services.length === 1) {
				selectInspectTarget(services[0].name);
			} else if (serviceDrafts.length === 0) {
				serviceDrafts = [createDefaultServiceDraft()];
			}
			return true;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Inspect failed';
			return false;
		} finally {
			inspecting = false;
		}
	}

	function createDefaultServiceDraft(): ServiceDraft {
		const baseName = watcherServiceName.trim() || 'app';
		return {
			service_type: 'nssm',
			windows_service_name: baseName,
			binary_name: `${baseName}.exe`,
			start_arguments: '',
			env_file: '.env',
			env_content: '',
			iis_app_kind: 'static',
			iis_app_pool: '',
			iis_site_name: '',
			iis_managed_runtime: '',
			public_url: '',
			config_files: [],
			health_check_url: healthCheckURL
		};
	}

	function addServiceDraft() {
		const baseName = watcherServiceName.trim() || 'app';
		serviceDrafts = [
			...serviceDrafts,
			{
				service_type: 'nssm',
				windows_service_name: `${baseName}-${serviceDrafts.length + 1}`,
				binary_name: `${baseName}-${serviceDrafts.length + 1}.exe`,
				start_arguments: '',
				env_file: '.env',
				env_content: '',
				iis_app_kind: 'static',
				iis_app_pool: '',
				iis_site_name: '',
				iis_managed_runtime: '',
				public_url: '',
				config_files: [],
				health_check_url: healthCheckURL
			}
		];
	}

	function removeServiceDraft(index: number) {
		serviceDrafts = serviceDrafts.filter((_, i) => i !== index);
	}

	function addConfigFileDraft(serviceIndex: number) {
		const next = [...serviceDrafts];
		const svc = next[serviceIndex];
		const configFiles = [...(svc.config_files || []), { file_path: '', target: 'app_dir' as ConfigFileTarget, content: '' }];
		next[serviceIndex] = { ...svc, config_files: configFiles };
		serviceDrafts = next;
	}

	function removeConfigFileDraft(serviceIndex: number, fileIndex: number) {
		const next = [...serviceDrafts];
		const svc = next[serviceIndex];
		const configFiles = [...(svc.config_files || [])];
		configFiles.splice(fileIndex, 1);
		next[serviceIndex] = { ...svc, config_files: configFiles };
		serviceDrafts = next;
	}

	function validateStep(step: WizardStep): boolean {
		switch (step) {
			case 1:
				if (!inspectResult) {
					error = 'Inspect the repository before continuing.';
					return false;
				}
				return true;
			case 2:
				if (inspectServices().length > 0 && !selectedInspectService) {
					error = 'Choose one detected deploy target before continuing.';
					return false;
				}
				if (!watcherName.trim()) {
					error = 'Watcher name is required.';
					return false;
				}
				if (!watcherServiceName.trim()) {
					error = 'App/service name ID is required.';
					return false;
				}
				if (!metadataURL.trim()) {
					error = 'Metadata URL is required.';
					return false;
				}
				if (!installDir.trim()) {
					error = 'Install directory is required.';
					return false;
				}
				if (checkInterval < 10) {
					error = 'Check interval must be at least 10 seconds.';
					return false;
				}
				return true;
			case 3:
				if (serviceDrafts.length === 0) {
					error = 'At least one service definition is required.';
					return false;
				}
				for (const [index, svc] of serviceDrafts.entries()) {
					if (!(svc.windows_service_name || '').trim()) {
						error = `Service #${index + 1} is missing its Windows service name.`;
						return false;
					}
					if (!isIISService(svc.service_type || 'nssm') && !(svc.binary_name || '').trim()) {
						error = `Service #${index + 1} is missing its executable name.`;
						return false;
					}
					for (const [fileIndex, file] of (svc.config_files || []).entries()) {
						if (!(file.file_path || '').trim()) {
							error = `Service #${index + 1} config file #${fileIndex + 1} is missing its path.`;
							return false;
						}
					}
				}
				return true;
			case 4:
				return true;
		}
	}

	async function goNext() {
		error = '';
		if (currentStep === 1) {
			const ok = await inspectRepo();
			if (ok) currentStep = 2;
			return;
		}
		if (!validateStep(currentStep)) {
			return;
		}
		currentStep = (currentStep + 1) as WizardStep;
	}

	function goBack() {
		error = '';
		currentStep = (currentStep - 1) as WizardStep;
	}

	function goToStep(target: WizardStep) {
		if (target <= currentStep) {
			error = '';
			currentStep = target;
			return;
		}
		for (let step = currentStep; step < target; step += 1) {
			if (!validateStep(step as WizardStep)) {
				return;
			}
		}
		currentStep = target;
	}

	function buildCreatePayload(): WatcherWritePayload {
		return {
			name: watcherName.trim(),
			service_name: watcherServiceName.trim(),
			metadata_url: metadataURL.trim(),
			release_ref: releaseRef.trim() || 'latest',
			deployment_environment: deploymentEnvironment.trim(),
			github_token: useWatcherGitHubToken ? watcherGitHubToken.trim() : '',
			check_interval_sec: checkInterval,
			install_dir: installDir.trim(),
			hc_enabled: healthChecksEnabled,
			hc_url: healthCheckURL.trim(),
			webhook_enabled: webhookEnabled,
			webhook_url: webhookURL.trim(),
			webhook_bearer_token: useCustomWebhookBearerToken ? webhookBearerToken.trim() : '',
			notify_version_found: webhookSelections.notify_version_found,
			notify_deployment_succeeded: webhookSelections.notify_deployment_succeeded,
			notify_deployment_failed: webhookSelections.notify_deployment_failed,
			notify_rollback_succeeded: webhookSelections.notify_rollback_succeeded,
			notify_rollback_failed: webhookSelections.notify_rollback_failed,
			notify_service_health_changed: webhookSelections.notify_service_health_changed,
			services: serviceDrafts.map((svc) => ({
				service_type: isIISService(svc.service_type || 'nssm') ? 'iis' : 'nssm',
				windows_service_name: (svc.windows_service_name || '').trim(),
				binary_name: (svc.binary_name || '').trim(),
				start_arguments: (svc.start_arguments || '').trim(),
				env_file: (svc.env_file || '').trim(),
				health_check_url: (svc.health_check_url || '').trim(),
				iis_app_kind: (svc.iis_app_kind || 'static') as IISAppKind,
				iis_app_pool: (svc.iis_app_pool || '').trim(),
				iis_site_name: (svc.iis_site_name || '').trim(),
				iis_managed_runtime: (svc.iis_managed_runtime || '').trim(),
				public_url: (svc.public_url || '').trim(),
				env_content: svc.env_content || '',
				config_files: (svc.config_files || []).map((file) => ({
					file_path: file.file_path.trim(),
					target: file.target || 'app_dir',
					content: file.content || ''
				}))
			}))
		};
	}

	async function createWatcher() {
		error = '';
		if (!validateStep(2) || !validateStep(3) || !validateStep(4)) {
			return;
		}
		creating = true;
		try {
			const watcher = await api.createWatcher(buildCreatePayload());
			try {
				await api.triggerCheck(watcher.id);
			} catch {
				// Ignore trigger failure so the user can still land on the watcher detail page.
			}
			await goto(resolve(`/watchers/${watcher.id}?tab=services`));
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to create watcher';
		} finally {
			creating = false;
		}
	}
</script>

<svelte:head>
	<title>Add Watcher | Watcher</title>
</svelte:head>

<div class="space-y-6">
	<div class="flex items-center gap-4">
		<a href={resolve('/watchers')}>
			<Button.Root variant="ghost" size="icon" class="h-8 w-8">
				<ArrowLeft class="h-4 w-4" />
			</Button.Root>
		</a>
		<div>
			<h1 class="text-2xl font-bold tracking-tight">Add Watcher</h1>
			<p class="text-sm text-muted-foreground">
				Inspect the source, configure watcher behavior, define one or more services, then enable webhook delivery if needed.
			</p>
		</div>
	</div>

	<div class="flex flex-wrap gap-2">
		{#each steps as step (step.id)}
			<button
				type="button"
				class={`rounded-full border px-3 py-1 text-sm transition ${
					step.id === currentStep
						? 'border-primary bg-primary/10 text-primary'
						: step.id < currentStep
							? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-400'
							: 'border-border/70 text-muted-foreground'
				}`}
				onclick={() => goToStep(step.id)}
			>
				{step.id}. {step.label}
			</button>
		{/each}
	</div>

	{#if error}
		<div class="flex items-center rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400">
			<AlertCircle class="mr-2 h-4 w-4 shrink-0" />
			<span>{error}</span>
		</div>
	{/if}

	<Card.Root class="border-border bg-card">
		<Card.Content class="space-y-5 p-6">
			{#if currentStep === 1}
				<div class="space-y-4">
					<div class="grid gap-4 md:grid-cols-2">
						<div class="space-y-2 md:col-span-2">
							<Label for="repoURL">GitHub Repository or metadata URL</Label>
							<Input id="repoURL" bind:value={repoURL} placeholder="https://github.com/org/repo" />
						</div>
						<div class="space-y-2">
							<Label for="releaseRef">Release Ref</Label>
							<Input id="releaseRef" bind:value={releaseRef} placeholder="latest or v1.2.3" />
						</div>
						<div class="space-y-2">
							<Label for="inspectGitHubToken">Inspect Token Override</Label>
							<Input
								id="inspectGitHubToken"
								type="password"
								bind:value={inspectGitHubToken}
								disabled={!useInspectGitHubToken}
								placeholder="Use only for inspect"
							/>
							<div class="mt-2 flex items-center gap-2">
								<Checkbox id="useInspectGitHubToken" bind:checked={useInspectGitHubToken} />
								<Label for="useInspectGitHubToken">Use custom token for inspect</Label>
							</div>
						</div>
					</div>

					<div class="rounded-lg border border-border/70 bg-muted/20 p-4 text-sm text-muted-foreground">
						Watcher inspects `version.json` first for the chosen ref and falls back to legacy repo asset discovery when needed.
					</div>

					{#if inspectResult}
						<div class="rounded-lg border border-border/70 bg-muted/20 p-4">
							<div class="grid gap-3 md:grid-cols-3">
								<div>
									<p class="text-xs text-muted-foreground">Detected version</p>
									<p class="mt-1 font-medium">{inspectResult.latest_version || 'Unknown'}</p>
								</div>
								<div>
									<p class="text-xs text-muted-foreground">Source</p>
									<p class="mt-1 font-medium">{inspectResult.source}</p>
								</div>
								<div>
									<p class="text-xs text-muted-foreground">Assets</p>
									<p class="mt-1 font-medium">{inspectResult.assets.length}</p>
								</div>
							</div>
						</div>
					{/if}
				</div>
			{:else if currentStep === 2}
				<div class="space-y-4">
					{#if inspectServices().length > 0}
						<div class="space-y-3 rounded-lg border border-border/70 bg-muted/20 p-4">
							<div class="flex items-center justify-between gap-3 text-sm">
								<div>
									<p class="font-medium">Detected deploy target</p>
									<p class="text-xs text-muted-foreground">
										Choose the release target that should drive this watcher's `service_name`.
									</p>
								</div>
								<span class="rounded border border-border px-2 py-1 text-xs text-muted-foreground">
									{inspectResult?.source === 'manifest' ? 'version.json' : 'repo assets'}
								</span>
							</div>
							<div class="grid gap-2 lg:grid-cols-2">
								{#each inspectServices() as target (target.name)}
									<button
										type="button"
										class={`rounded-lg border p-3 text-left text-sm transition ${
											selectedInspectService === target.name
												? 'border-primary bg-primary/10'
												: 'border-border/70 bg-background/60 hover:bg-muted/40'
										}`}
										onclick={() => selectInspectTarget(target.name)}
									>
										<div class="flex items-start justify-between gap-2">
											<span class="font-medium">{target.name}</span>
											<span class="text-xs text-muted-foreground">{target.version}</span>
										</div>
										<p class="mt-1 truncate font-mono text-xs text-muted-foreground">{target.artifact}</p>
									</button>
								{/each}
							</div>
						</div>
					{/if}

					<div class="grid gap-4 md:grid-cols-2">
						<div class="space-y-2">
							<Label for="watcherName">Watcher Display Name</Label>
							<Input id="watcherName" bind:value={watcherName} placeholder="my-app" />
						</div>
						<div class="space-y-2">
							<Label for="watcherServiceName">App/Service Name ID</Label>
							<Input id="watcherServiceName" bind:value={watcherServiceName} placeholder="my-app" />
						</div>
						<div class="space-y-2 md:col-span-2">
							<Label for="metadataURL">Metadata URL</Label>
							<Input id="metadataURL" bind:value={metadataURL} />
						</div>
						<div class="space-y-2 md:col-span-2">
							<Label for="installDir">Base Install Directory</Label>
							<Input id="installDir" bind:value={installDir} placeholder="C:\apps\my-app" />
						</div>
						<div class="space-y-2">
							<Label for="checkInterval">Check Interval (sec)</Label>
							<Input id="checkInterval" type="number" min="10" bind:value={checkInterval} />
						</div>
						<div class="space-y-2">
							<Label for="deploymentEnvironment">Deployment Environment</Label>
							<Input id="deploymentEnvironment" bind:value={deploymentEnvironment} placeholder="production" />
						</div>
						<div class="space-y-2 md:col-span-2">
							<Label for="watcherGitHubToken">Watcher GitHub Token Override</Label>
							<Input
								id="watcherGitHubToken"
								type="password"
								bind:value={watcherGitHubToken}
								disabled={!useWatcherGitHubToken}
								placeholder="Optional watcher-specific token"
							/>
							<div class="mt-2 flex items-center gap-2">
								<Checkbox id="useWatcherGitHubToken" bind:checked={useWatcherGitHubToken} />
								<Label for="useWatcherGitHubToken">Use watcher-specific GitHub token</Label>
							</div>
						</div>
						<div class="flex items-center gap-2 md:col-span-2">
							<Checkbox id="healthChecksEnabled" bind:checked={healthChecksEnabled} />
							<Label for="healthChecksEnabled">Enable watcher-level health checks</Label>
						</div>
						<div class="space-y-2 md:col-span-2">
							<Label for="healthCheckURL">Global Health Check URL</Label>
							<Input id="healthCheckURL" bind:value={healthCheckURL} placeholder="http://localhost:3000/health" />
						</div>
					</div>
				</div>
			{:else if currentStep === 3}
				<div class="space-y-4">
					<div class="flex items-center justify-between gap-3">
						<div>
							<p class="font-medium">Managed services</p>
							<p class="text-sm text-muted-foreground">
								Define one or more services that this watcher should manage under the same release source.
							</p>
						</div>
						<Button.Root variant="outline" size="sm" type="button" onclick={addServiceDraft}>
							<Plus class="mr-2 h-4 w-4" /> Add Service
						</Button.Root>
					</div>

					{#if serviceDrafts.length === 0}
						<div class="rounded-lg border border-dashed border-border/70 bg-muted/20 p-8 text-center text-sm text-muted-foreground">
							Add at least one service definition before creating the watcher.
						</div>
					{:else}
						{#each serviceDrafts as svc, i (i)}
							<div class="relative space-y-3 rounded-lg border border-border/70 bg-card p-4">
								<Button.Root
									variant="ghost"
									size="icon"
									class="absolute right-3 top-3 h-8 w-8 text-red-400"
									type="button"
									onclick={() => removeServiceDraft(i)}
									disabled={serviceDrafts.length === 1}
								>
									<Trash2 class="h-4 w-4" />
								</Button.Root>

								<div class="pr-10">
									<p class="font-medium">Service #{i + 1}</p>
									<p class="text-xs text-muted-foreground">
										Runtime definition stored under this watcher.
									</p>
								</div>

								<div class="grid gap-4 md:grid-cols-2">
									<div class="space-y-2">
										<Label>Hosting Mode</Label>
										<Select.Root type="single" bind:value={svc.service_type}>
											<Select.Trigger>
												{isIISService(svc.service_type || 'nssm') ? 'IIS Site' : 'NSSM Native Windows'}
											</Select.Trigger>
											<Select.Content>
												<Select.Item value="nssm" label="NSSM Native Windows">NSSM Native Windows</Select.Item>
												<Select.Item value="iis" label="IIS Site">IIS Site</Select.Item>
											</Select.Content>
										</Select.Root>
									</div>
									<div class="space-y-2">
										<Label>{isIISService(svc.service_type || 'nssm') ? 'Service Identifier' : 'Windows Service Name'}</Label>
										<Input bind:value={svc.windows_service_name} placeholder="myapp-web" />
									</div>

									{#if !isIISService(svc.service_type || 'nssm')}
										<div class="space-y-2">
											<Label>Executable Name</Label>
											<Input bind:value={svc.binary_name} placeholder="myapp.exe" />
										</div>
										<div class="space-y-2">
											<Label>Start Arguments</Label>
											<Input bind:value={svc.start_arguments} placeholder="serve --port 8080" />
										</div>
										<div class="space-y-2">
											<Label>Env File</Label>
											<Input bind:value={svc.env_file} placeholder=".env.prod" />
										</div>
										<div class="space-y-2">
											<Label>Service Health Check URL</Label>
											<Input bind:value={svc.health_check_url} placeholder="http://localhost:3000/health" />
										</div>
										<div class="space-y-2 md:col-span-2">
											<Label>Env Content</Label>
											<Textarea
												class="min-h-[140px] font-mono text-xs"
												bind:value={svc.env_content}
												placeholder="KEY=VALUE&#10;API_PORT=3000"
											/>
										</div>
									{:else}
										<div class="space-y-2 md:col-span-2">
											<Label>IIS App Kind</Label>
											<Select.Root type="single" bind:value={svc.iis_app_kind}>
												<Select.Trigger>
													{iisAppKinds.find((kind) => kind.value === (svc.iis_app_kind || 'static'))?.label || 'Select kind'}
												</Select.Trigger>
												<Select.Content>
													{#each iisAppKinds as kind (kind.value)}
														<Select.Item value={kind.value} label={kind.label}>{kind.label}</Select.Item>
													{/each}
												</Select.Content>
											</Select.Root>
											<p class="text-xs text-muted-foreground">
												{iisAppKinds.find((kind) => kind.value === (svc.iis_app_kind || 'static'))?.hint}
											</p>
										</div>
										<div class="space-y-2">
											<Label>IIS App Pool</Label>
											<Input bind:value={svc.iis_app_pool} placeholder="myapp-web" />
										</div>
										<div class="space-y-2">
											<Label>IIS Site Name</Label>
											<Input bind:value={svc.iis_site_name} placeholder="myapp-web" />
										</div>
										<div class="space-y-2">
											<Label>Public URL</Label>
											<Input bind:value={svc.public_url} placeholder="https://app.example.com" />
										</div>
										<div class="space-y-2">
											<Label>Service Health Check URL</Label>
											<Input bind:value={svc.health_check_url} placeholder="https://app.example.com/health" />
										</div>
										<div class="rounded-lg border border-border/70 bg-muted/20 p-3 text-xs text-muted-foreground md:col-span-2">
											<span class="font-medium text-foreground/90">{serviceTypeLabel(svc.service_type || 'nssm')}:</span>
											{' '}{iisAppKindLabel(String(svc.iis_app_kind || 'static'))}. Watcher will set the IIS app pool runtime automatically for this profile.
										</div>
									{/if}

									<div class="space-y-2 md:col-span-2">
										<div class="flex items-center justify-between gap-3">
											<Label>Additional managed config files</Label>
											<Button.Root variant="outline" size="sm" type="button" onclick={() => addConfigFileDraft(i)}>
												<Plus class="mr-2 h-3 w-3" /> Add file
											</Button.Root>
										</div>
										{#if (svc.config_files || []).length > 0}
											<div class="space-y-3 rounded-lg border border-border/70 bg-background/50 p-3">
												{#each svc.config_files || [] as file, fileIndex (fileIndex)}
													<div class="space-y-2 rounded-lg border border-border/60 bg-card/60 p-3">
														<div class="flex items-center justify-between gap-3">
															<Label>Config file #{fileIndex + 1}</Label>
															<Button.Root
																variant="ghost"
																size="icon"
																type="button"
																class="h-8 w-8 text-red-400"
																onclick={() => removeConfigFileDraft(i, fileIndex)}
															>
																<Trash2 class="h-4 w-4" />
															</Button.Root>
														</div>
														<div class="grid gap-2 md:grid-cols-[1fr_180px]">
															<Input bind:value={file.file_path} placeholder="web.config or config/appsettings.json" />
															<Select.Root type="single" bind:value={file.target}>
																<Select.Trigger>
																	{file.target === 'release_dir' ? 'Current dir' : 'Service/app dir'}
																</Select.Trigger>
																<Select.Content>
																	<Select.Item value="app_dir" label="Service/app dir">Service/app dir</Select.Item>
																	<Select.Item value="release_dir" label="Current dir">Current dir</Select.Item>
																</Select.Content>
															</Select.Root>
														</div>
														<Textarea
															class="min-h-[120px] font-mono text-xs"
															bind:value={file.content}
															placeholder={'{\n  "port": 3000\n}'}
														/>
													</div>
												{/each}
											</div>
										{:else}
											<p class="text-xs text-muted-foreground">
												Use <code>Current dir</code> for IIS files like <code>web.config</code> that must sit beside deployed assets.
											</p>
										{/if}
									</div>
								</div>
							</div>
						{/each}
					{/if}
				</div>
			{:else}
				<div class="space-y-4">
					<div class="rounded-lg border border-border/70 bg-muted/20 p-4 text-sm text-muted-foreground">
						Webhook delivery is optional. This step only controls watcher-specific endpoint, token override, and event subscriptions.
					</div>

					<div class="rounded-lg border border-border/70 p-4">
						<div class="flex items-center gap-2">
							<Checkbox id="webhookEnabled" bind:checked={webhookEnabled} />
							<Label for="webhookEnabled">Enable webhook delivery for this watcher</Label>
						</div>
						<div class="mt-4 grid gap-4 md:grid-cols-2">
							<div class="space-y-2">
								<Label for="webhookURL">Webhook URL</Label>
								<Input id="webhookURL" bind:value={webhookURL} placeholder="https://example.com/hooks/watcher" />
								<p class="text-xs text-muted-foreground">Leave empty to inherit the global default URL.</p>
							</div>
							<div class="space-y-2">
								<Label for="webhookBearerToken">Webhook Bearer Token Override</Label>
								<Input
									id="webhookBearerToken"
									type="password"
									bind:value={webhookBearerToken}
									disabled={!useCustomWebhookBearerToken}
									placeholder="Bearer token override"
								/>
								<div class="mt-2 flex items-center gap-2">
									<Checkbox id="useCustomWebhookBearerToken" bind:checked={useCustomWebhookBearerToken} />
									<Label for="useCustomWebhookBearerToken">Use watcher-specific webhook bearer token</Label>
								</div>
							</div>
						</div>
					</div>

					<div class="space-y-3 rounded-lg border border-border/70 p-4">
						<div class="flex items-start justify-between gap-3">
							<div>
								<h4 class="font-medium">Event Subscriptions</h4>
								<p class="text-sm text-muted-foreground">
									Choose which business events this watcher should emit.
								</p>
							</div>
							<div class="flex flex-wrap gap-2">
								<a href={resolve('/docs/webhooks')}>
									<Button.Root type="button" variant="outline" size="sm">
										<BookOpenText class="mr-2 h-4 w-4" />
										Integration Guide
									</Button.Root>
								</a>
								<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
								<a href={webhookDocsHref} target="_blank" rel="noopener noreferrer">
									<Button.Root type="button" variant="outline" size="sm">
										<ExternalLink class="mr-2 h-4 w-4" />
										Repo Docs
									</Button.Root>
								</a>
							</div>
						</div>
						<div class="grid gap-3 md:grid-cols-2">
							<label class="flex items-center gap-2 text-sm">
								<Checkbox bind:checked={webhookSelections.notify_version_found} />
								<span>Version Found</span>
							</label>
							<label class="flex items-center gap-2 text-sm">
								<Checkbox bind:checked={webhookSelections.notify_deployment_succeeded} />
								<span>Deployment Succeeded</span>
							</label>
							<label class="flex items-center gap-2 text-sm">
								<Checkbox bind:checked={webhookSelections.notify_deployment_failed} />
								<span>Deployment Failed</span>
							</label>
							<label class="flex items-center gap-2 text-sm">
								<Checkbox bind:checked={webhookSelections.notify_rollback_succeeded} />
								<span>Rollback Succeeded</span>
							</label>
							<label class="flex items-center gap-2 text-sm">
								<Checkbox bind:checked={webhookSelections.notify_rollback_failed} />
								<span>Rollback Failed</span>
							</label>
							<label class="flex items-center gap-2 text-sm">
								<Checkbox bind:checked={webhookSelections.notify_service_health_changed} />
								<span>Service Health Changed</span>
							</label>
						</div>
					</div>
				</div>
			{/if}
		</Card.Content>
	</Card.Root>

	<div class="flex items-center justify-between gap-3">
		{#if currentStep === 1}
			<a href={resolve('/watchers')}>
				<Button.Root variant="outline" type="button">Cancel</Button.Root>
			</a>
		{:else}
			<Button.Root variant="outline" type="button" onclick={goBack}>Back</Button.Root>
		{/if}

		{#if currentStep < 4}
			<Button.Root type="button" onclick={goNext} disabled={inspecting}>
				{#if currentStep === 1 && inspecting}
					Inspecting...
				{:else}
					Next <ArrowRight class="ml-2 h-4 w-4" />
				{/if}
			</Button.Root>
		{:else}
			<Button.Root type="button" onclick={createWatcher} disabled={creating}>
				{#if creating}
					Creating...
				{:else}
					<Check class="mr-2 h-4 w-4" /> Save Watcher & Services
				{/if}
			</Button.Root>
		{/if}
	</div>
</div>
