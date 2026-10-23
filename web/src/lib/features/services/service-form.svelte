<script lang="ts">
	import { untrack } from 'svelte';
	import UnsavedChanges from '$lib/components/console/unsaved-changes.svelte';
	import ConfigWorkspace from '$lib/features/configuration/config-workspace.svelte';
	import { configurationProblems, copyFiles } from '$lib/features/configuration/model';
	import WizardProgress from '$lib/components/console/wizard-progress.svelte';
	import {
		iisAppKindLabel,
		type IISAppKind,
		type ServiceConfigFile,
		type ServiceWritePayload,
		type ServiceType
	} from '$lib/api';
	import * as Button from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Select from '$lib/components/ui/select/index.js';
	import { ArrowLeft, ArrowRight, Save } from '@lucide/svelte';

	type ServiceWizardInitial = Partial<ServiceWritePayload> & {
		windows_service_name?: string;
		config_files?: ServiceConfigFile[];
	};

	let {
		title,
		description,
		showHeader = true,
		submitLabel = 'Save Service',
		layout = 'wizard',
		onSaved,
		initial = {},
		submitting = false,
		error = '',
		onSubmit
	}: {
		showHeader?: boolean;
		layout?: 'wizard' | 'single';
		onSaved?: () => void | Promise<void>;
		title: string;
		description: string;
		submitLabel?: string;
		initial?: ServiceWizardInitial;
		submitting?: boolean;
		error?: string;
		onSubmit: (payload: ServiceWritePayload) => Promise<boolean> | boolean;
	} = $props();

	const iisAppKinds: Array<{ value: IISAppKind; label: string; hint: string }> = [
		{
			value: 'static',
			label: 'Static Site',
			hint: 'Frontend build or static files served directly by IIS.'
		},
		{ value: 'php', label: 'PHP', hint: 'PHP app on IIS with FastCGI and PHP already installed.' },
		{
			value: 'aspnet_classic',
			label: 'ASP.NET Classic',
			hint: 'Classic ASP.NET app using the .NET CLR app pool.'
		}
	];

	const steps = [
		{ id: 1, label: 'Basics', description: 'Name and hosting mode' },
		{ id: 2, label: 'Runtime', description: 'Process and health checks' },
		{ id: 3, label: 'Managed files', description: 'Environment and config' }
	] as const;

	// The keyed host recreates this editor when its initial service changes.
	const defaults = untrack(() => initial);
	let currentStep = $state(1);
	let serviceType = $state<ServiceType>(
		defaults.service_type === 'iis' || defaults.service_type === 'static' ? 'iis' : 'nssm'
	);
	let serviceName = $state(defaults.windows_service_name || '');
	let binaryName = $state(defaults.binary_name || '');
	let startArguments = $state(defaults.start_arguments || '');
	let envFile = $state(defaults.env_file || '');
	let envContent = $state(defaults.env_content || '');
	let healthCheckURL = $state(defaults.health_check_url || '');
	let publicURL = $state(defaults.public_url || '');
	let iisAppKind = $state<IISAppKind>(defaults.iis_app_kind || 'static');
	let iisAppPool = $state(defaults.iis_app_pool || '');
	let iisSiteName = $state(defaults.iis_site_name || '');
	let configFiles = $state<ServiceConfigFile[]>(
		(defaults.config_files || []).map((file) => ({ ...file, target: file.target || 'app_dir' }))
	);

	function nextStep() {
		currentStep = Math.min(currentStep + 1, steps.length);
	}

	function previousStep() {
		currentStep = Math.max(currentStep - 1, 1);
	}

	function buildPayload(): ServiceWritePayload {
		return {
			service_type: serviceType,
			windows_service_name: serviceName.trim(),
			binary_name: binaryName.trim(),
			start_arguments: startArguments.trim(),
			env_file: envFile.trim(),
			env_content: envContent,
			health_check_url: healthCheckURL.trim(),
			iis_app_kind: iisAppKind,
			iis_app_pool: iisAppPool.trim(),
			iis_site_name: iisSiteName.trim(),
			public_url: publicURL.trim(),
			config_files: copyFiles(configFiles)
		};
	}

	const initialPayload = untrack(() => JSON.stringify(buildPayload()));
	let savedPayload = $state(initialPayload);
	let savedEnv = $state(defaults.env_content || '');
	let savedFiles = $state<ServiceConfigFile[]>(copyFiles(defaults.config_files || []));
	let formError = $state('');
	let success = $state('');
	const dirty = $derived(JSON.stringify(buildPayload()) !== savedPayload);
	const pathProblems = $derived(configurationProblems(envFile, configFiles));
	async function submit() {
		formError = '';
		success = '';
		if (!serviceName.trim() || (serviceType === 'nssm' && !binaryName.trim())) {
			formError = 'Enter a service name and the executable name for NSSM services.';
			if (layout === 'wizard') currentStep = !serviceName.trim() ? 1 : 2;
			return;
		}
		if (pathProblems.length) {
			formError = pathProblems.join(' ');
			if (layout === 'wizard') currentStep = 3;
			return;
		}
		const payload = buildPayload();
		if (await onSubmit(payload)) {
			savedPayload = JSON.stringify(payload);
			savedEnv = envContent;
			savedFiles = copyFiles(configFiles);
			success = 'Service configuration saved.';
			if (onSaved) await onSaved();
		}
	}
</script>

<UnsavedChanges {dirty} label="service settings" />
<div class="space-y-6">
	{#if showHeader}<div>
			<h2 class="text-xl font-semibold">{title}</h2>
			<p class="mt-1 text-sm text-muted-foreground">{description}</p>
		</div>{/if}
	{#if layout === 'wizard'}<WizardProgress
			{steps}
			current={currentStep}
			onselect={(step) => (currentStep = step)}
			disabled={submitting}
		/>{:else}<div class="review-step">
			<p class="text-sm font-medium">Service settings</p>
			<p class="mt-2 text-xs leading-relaxed text-muted-foreground">
				Edit identity, runtime, health checks, and files together. Saving updates the active service
				configuration; runtime changes take effect when the service is restarted or deployed.
			</p>
		</div>{/if}
	{#if error || formError}<p
			role="alert"
			class="rounded-md border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-300"
		>
			{error || formError}
		</p>{/if}
	{#if success}<p
			role="status"
			class="rounded-md border border-emerald-500/30 bg-emerald-500/10 p-3 text-sm text-emerald-300"
		>
			{success}
		</p>{/if}
	<fieldset disabled={submitting} class="min-w-0 space-y-6">
		{#if layout === 'single' || currentStep === 1}<section
				class="console-panel"
				aria-label="Identity and health"
			>
				<div class="panel-heading">
					<div>
						<p class="eyebrow">01 / Identity and health</p>
						<h3 class="mt-1">How this service is identified and monitored</h3>
					</div>
				</div>
				<div class="p-5">
					<div class="grid gap-4 md:grid-cols-2">
						<div class="space-y-2">
							<Label for="serviceType">Hosting Mode</Label>
							<Select.Root type="single" bind:value={serviceType}>
								<Select.Trigger id="serviceType">
									{serviceType === 'iis' ? 'IIS Site' : 'Binary (NSSM)'}
								</Select.Trigger>
								<Select.Content>
									<Select.Item value="nssm" label="Binary (NSSM)">Binary (NSSM)</Select.Item>
									<Select.Item value="iis" label="IIS Site">IIS Site</Select.Item>
								</Select.Content>
							</Select.Root>
						</div>
						<div class="space-y-2">
							<Label for="serviceName"
								>{serviceType === 'iis' ? 'Service Identifier' : 'Windows Service Name'}</Label
							>
							<Input
								id="serviceName"
								bind:value={serviceName}
								placeholder={serviceType === 'iis' ? 'marketing-site' : 'my-app-web'}
								aria-describedby="serviceName-help"
							/>
							<p id="serviceName-help" class="text-xs text-muted-foreground">
								{serviceType === 'nssm'
									? 'Windows service name registered with NSSM.'
									: 'Identifier for this IIS service in Watcher.'}
							</p>
						</div>
						<div class="space-y-2">
							<Label for="healthCheckURL">Health Check URL</Label>
							<Input
								id="healthCheckURL"
								bind:value={healthCheckURL}
								placeholder="http://localhost:3000/health"
								aria-describedby="healthCheckURL-help"
							/>
							<p id="healthCheckURL-help" class="text-xs text-muted-foreground">
								Health endpoint reachable from the Watcher machine. Leave blank to use the watcher
								health URL.
							</p>
						</div>
						<div class="space-y-2">
							<Label for="publicURL">Public URL</Label>
							<Input
								id="publicURL"
								bind:value={publicURL}
								placeholder="https://app.example.com"
								aria-describedby="publicURL-help"
							/>
							<p id="publicURL-help" class="text-xs text-muted-foreground">
								Optional link for opening this application from the dashboard.
							</p>
						</div>
					</div>
				</div>
			</section>{/if}
		{#if layout === 'single' || currentStep === 2}<section
				class="console-panel"
				aria-label="Runtime settings"
			>
				<div class="panel-heading">
					<div>
						<p class="eyebrow">02 / Runtime</p>
						<h3 class="mt-1">
							{serviceType === 'nssm' ? 'Executable and startup' : 'IIS hosting'}
						</h3>
					</div>
				</div>
				<div class="p-5">
					{#if serviceType === 'nssm'}
						<div class="grid gap-4 md:grid-cols-2">
							<div class="space-y-2">
								<Label for="binaryName">Binary Name</Label>
								<Input
									id="binaryName"
									bind:value={binaryName}
									placeholder="my-app.exe"
									aria-describedby="binaryName-help"
								/>
								<p id="binaryName-help" class="text-xs text-muted-foreground">
									Executable path relative to the extracted release. It must match the artifact
									filename.
								</p>
							</div>
							<div class="space-y-2">
								<Label for="startArguments">Start Arguments</Label>
								<Input
									id="startArguments"
									bind:value={startArguments}
									placeholder="serve --port 8080"
									aria-describedby="startArguments-help"
								/>
								<p id="startArguments-help" class="text-xs text-muted-foreground">
									Command-line arguments passed to the executable when the service starts.
								</p>
							</div>
							<div class="space-y-2 md:col-span-2">
								<Label for="envFile">Env File</Label>
								<Input
									id="envFile"
									bind:value={envFile}
									placeholder=".env.production"
									aria-describedby="envFile-help"
								/>
								<p id="envFile-help" class="text-xs text-muted-foreground">
									Environment file path relative to the watcher installation directory. Leave blank
									if unused.
								</p>
							</div>
						</div>
					{:else}
						<div class="grid gap-4 md:grid-cols-2">
							<div class="space-y-2 md:col-span-2">
								<Label for="iisAppKind">IIS App Kind</Label>
								<Select.Root type="single" bind:value={iisAppKind}>
									<Select.Trigger id="iisAppKind">
										{iisAppKinds.find((kind) => kind.value === iisAppKind)?.label || 'Select kind'}
									</Select.Trigger>
									<Select.Content>
										{#each iisAppKinds as kind (kind.value)}
											<Select.Item value={kind.value} label={kind.label}>{kind.label}</Select.Item>
										{/each}
									</Select.Content>
								</Select.Root>
								<p class="text-xs text-muted-foreground">
									{iisAppKinds.find((kind) => kind.value === iisAppKind)?.hint}
								</p>
							</div>
							<div class="space-y-2">
								<Label for="iisAppPool">IIS App Pool</Label>
								<Input
									id="iisAppPool"
									bind:value={iisAppPool}
									placeholder="my-app"
									aria-describedby="iisAppPool-help"
								/>
								<p id="iisAppPool-help" class="text-xs text-muted-foreground">
									IIS application pool to configure or recycle for this application.
								</p>
							</div>
							<div class="space-y-2">
								<Label for="iisSiteName">IIS Site Name</Label>
								<Input
									id="iisSiteName"
									bind:value={iisSiteName}
									placeholder="my-app"
									aria-describedby="iisSiteName-help"
								/>
								<p id="iisSiteName-help" class="text-xs text-muted-foreground">
									IIS website name on the Watcher machine.
								</p>
							</div>
							<div
								class="rounded-lg border border-border/70 bg-muted/20 p-4 text-sm text-muted-foreground md:col-span-2"
							>
								<span class="font-medium text-foreground">Bootstrap profile:</span>
								{iisAppKindLabel(iisAppKind)}. Watcher will set the IIS managed runtime
								automatically for this app kind.
							</div>
						</div>
					{/if}
				</div>
			</section>{/if}
		{#if layout === 'single' || currentStep === 3}
			{#if serviceType === 'iis'}<label class="block space-y-2 text-sm"
					><span>Environment file path (optional)</span><Input
						bind:value={envFile}
						placeholder=".env"
						aria-label="Environment file path"
					/></label
				>{/if}
			<ConfigWorkspace
				serviceName={serviceName || 'New service'}
				context={layout === 'single' ? 'Service settings' : 'Initial configuration'}
				mode="form"
				envPath={envFile}
				bind:envContent
				bind:files={configFiles}
				baselineEnv={savedEnv}
				baselineFiles={savedFiles}
				{savedEnv}
				{savedFiles}
				disabled={submitting}
			/>
		{/if}
	</fieldset>
	<div class="wizard-footer">
		<span class="flex items-center text-xs text-muted-foreground" aria-live="polite"
			>{dirty ? 'Unsaved changes' : 'All changes saved'}</span
		>
		<div class="flex gap-2">
			{#if layout === 'wizard' && currentStep > 1}<Button.Root
					variant="outline"
					onclick={previousStep}
					disabled={submitting}><ArrowLeft class="size-3.5" />Back</Button.Root
				>{/if}{#if layout === 'wizard' && currentStep < steps.length}<Button.Root
					onclick={nextStep}
					disabled={submitting}>Next<ArrowRight class="size-3.5" /></Button.Root
				>{:else}<Button.Root
					onclick={submit}
					loading={submitting}
					disabled={submitting || (layout === 'single' && !dirty)}
					><Save class="size-3.5" />{submitLabel}</Button.Root
				>{/if}
		</div>
	</div>
</div>
