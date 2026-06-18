<script lang="ts">
	import { iisAppKindLabel, type IISAppKind, type Service, type ServiceConfigFile, type ServiceWritePayload, type ServiceType } from '$lib/api';
	import * as Button from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Select from '$lib/components/ui/select/index.js';
	import { Textarea } from '$lib/components/ui/textarea';
	import { ArrowLeft, ArrowRight, Plus, Save, Trash2 } from '@lucide/svelte';

	type ServiceWizardInitial = Partial<ServiceWritePayload> & {
		windows_service_name?: string;
		config_files?: ServiceConfigFile[];
	};

	let {
		title,
		description,
		submitLabel = 'Save Service',
		initial = {},
		submitting = false,
		error = '',
		onSubmit
	}: {
		title: string;
		description: string;
		submitLabel?: string;
		initial?: ServiceWizardInitial;
		submitting?: boolean;
		error?: string;
		onSubmit: (payload: ServiceWritePayload) => Promise<void> | void;
	} = $props();

	const iisAppKinds: Array<{ value: IISAppKind; label: string; hint: string }> = [
		{ value: 'static', label: 'Static Site', hint: 'Frontend build or static files served directly by IIS.' },
		{ value: 'php', label: 'PHP', hint: 'PHP app on IIS with FastCGI and PHP already installed.' },
		{ value: 'aspnet_classic', label: 'ASP.NET Classic', hint: 'Classic ASP.NET app using the .NET CLR app pool.' }
	];

	const steps = [
		{ id: 1, label: 'Basics' },
		{ id: 2, label: 'Runtime' },
		{ id: 3, label: 'Managed Files' }
	] as const;

	let currentStep = $state(1);
	let serviceType = $state<ServiceType>('nssm');
	let serviceName = $state('');
	let binaryName = $state('');
	let startArguments = $state('');
	let envFile = $state('');
	let envContent = $state('');
	let healthCheckURL = $state('');
	let publicURL = $state('');
	let iisAppKind = $state<IISAppKind>('static');
	let iisAppPool = $state('');
	let iisSiteName = $state('');
	let configFiles = $state<ServiceConfigFile[]>([]);

	$effect(() => {
		serviceType = initial.service_type === 'iis' || initial.service_type === 'static' ? 'iis' : 'nssm';
		serviceName = initial.windows_service_name || '';
		binaryName = initial.binary_name || '';
		startArguments = initial.start_arguments || '';
		envFile = initial.env_file || '';
		envContent = initial.env_content || '';
		healthCheckURL = initial.health_check_url || '';
		publicURL = initial.public_url || '';
		iisAppKind = initial.iis_app_kind || 'static';
		iisAppPool = initial.iis_app_pool || '';
		iisSiteName = initial.iis_site_name || '';
		configFiles = (initial.config_files || []).map((file) => ({
			id: file.id,
			service_id: file.service_id,
			file_path: file.file_path,
			target: file.target || 'app_dir',
			content: file.content
		}));
		currentStep = 1;
	});

	function addConfigFile() {
		configFiles = [...configFiles, { file_path: '', target: 'app_dir', content: '' }];
	}

	function removeConfigFile(index: number) {
		configFiles = configFiles.filter((_, i) => i !== index);
	}

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
			config_files: configFiles.filter((file) => file.file_path.trim() !== '')
		};
	}

	async function submit() {
		await onSubmit(buildPayload());
	}
</script>

<div class="space-y-6">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">{title}</h1>
		<p class="mt-1 text-sm text-muted-foreground">{description}</p>
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
				onclick={() => (currentStep = step.id)}
			>
				{step.id}. {step.label}
			</button>
		{/each}
	</div>

	{#if error}
		<div class="rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400">
			{error}
		</div>
	{/if}

	<Card.Root class="border-border bg-card">
		<Card.Content class="space-y-5 p-6">
			{#if currentStep === 1}
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
						<Label for="serviceName">{serviceType === 'iis' ? 'Service Identifier' : 'Windows Service Name'}</Label>
						<Input id="serviceName" bind:value={serviceName} placeholder={serviceType === 'iis' ? 'marketing-site' : 'my-app-web'} />
					</div>
					<div class="space-y-2">
						<Label for="healthCheckURL">Health Check URL</Label>
						<Input id="healthCheckURL" bind:value={healthCheckURL} placeholder="http://localhost:3000/health" />
					</div>
					<div class="space-y-2">
						<Label for="publicURL">Public URL</Label>
						<Input id="publicURL" bind:value={publicURL} placeholder="https://app.example.com" />
					</div>
				</div>
			{:else if currentStep === 2}
				{#if serviceType === 'nssm'}
					<div class="grid gap-4 md:grid-cols-2">
						<div class="space-y-2">
							<Label for="binaryName">Binary Name</Label>
							<Input id="binaryName" bind:value={binaryName} placeholder="my-app.exe" />
						</div>
						<div class="space-y-2">
							<Label for="startArguments">Start Arguments</Label>
							<Input id="startArguments" bind:value={startArguments} placeholder="serve --port 8080" />
						</div>
						<div class="space-y-2 md:col-span-2">
							<Label for="envFile">Env File</Label>
							<Input id="envFile" bind:value={envFile} placeholder=".env.production" />
						</div>
						<div class="space-y-2 md:col-span-2">
							<Label for="envContent">Env Content</Label>
							<Textarea id="envContent" class="min-h-[220px] font-mono text-xs" bind:value={envContent} placeholder="KEY=VALUE&#10;API_URL=https://example.com" />
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
							<p class="text-xs text-muted-foreground">{iisAppKinds.find((kind) => kind.value === iisAppKind)?.hint}</p>
						</div>
						<div class="space-y-2">
							<Label for="iisAppPool">IIS App Pool</Label>
							<Input id="iisAppPool" bind:value={iisAppPool} placeholder="my-app" />
						</div>
						<div class="space-y-2">
							<Label for="iisSiteName">IIS Site Name</Label>
							<Input id="iisSiteName" bind:value={iisSiteName} placeholder="my-app" />
						</div>
						<div class="rounded-lg border border-border/70 bg-muted/20 p-4 text-sm text-muted-foreground md:col-span-2">
							<span class="font-medium text-foreground">Bootstrap profile:</span>
							{' '}{iisAppKindLabel(iisAppKind)}. Watcher will set the IIS managed runtime automatically for this app kind.
						</div>
					</div>
				{/if}
			{:else}
				<div class="space-y-4">
					<div class="flex items-center justify-between gap-3">
						<div>
							<p class="font-medium">Managed config files</p>
							<p class="text-sm text-muted-foreground">
								Optional files that Watcher should write during sync or deploy.
							</p>
						</div>
						<Button.Root type="button" size="sm" variant="outline" onclick={addConfigFile}>
							<Plus class="mr-2 h-4 w-4" />
							Add file
						</Button.Root>
					</div>

					{#if configFiles.length === 0}
						<div class="rounded-lg border border-dashed border-border/70 bg-muted/20 p-6 text-sm text-muted-foreground">
							No managed files configured.
						</div>
					{:else}
						<div class="space-y-3">
							{#each configFiles as file, index (index)}
								<div class="rounded-lg border border-border/70 bg-muted/20 p-4">
									<div class="mb-3 flex items-center justify-between gap-3">
										<p class="font-medium">Managed File #{index + 1}</p>
										<Button.Root type="button" size="icon" variant="ghost" class="h-8 w-8 text-red-400" onclick={() => removeConfigFile(index)}>
											<Trash2 class="h-4 w-4" />
										</Button.Root>
									</div>
									<div class="grid gap-4 md:grid-cols-2">
										<div class="space-y-2">
											<Label for={`config-path-${index}`}>Path</Label>
											<Input id={`config-path-${index}`} bind:value={file.file_path} placeholder="web.config" />
										</div>
										<div class="space-y-2">
											<Label for={`config-target-${index}`}>Target</Label>
											<Select.Root type="single" bind:value={file.target}>
												<Select.Trigger id={`config-target-${index}`}>
													{file.target === 'release_dir' ? 'Release dir' : 'App dir'}
												</Select.Trigger>
												<Select.Content>
													<Select.Item value="app_dir" label="App dir">App dir</Select.Item>
													<Select.Item value="release_dir" label="Release dir">Release dir</Select.Item>
												</Select.Content>
											</Select.Root>
										</div>
										<div class="space-y-2 md:col-span-2">
											<Label for={`config-content-${index}`}>Content</Label>
											<Textarea id={`config-content-${index}`} class="min-h-[180px] font-mono text-xs" bind:value={file.content} />
										</div>
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</div>
			{/if}
		</Card.Content>
	</Card.Root>

	<div class="flex items-center justify-between gap-3">
		<Button.Root type="button" variant="outline" onclick={previousStep} disabled={currentStep === 1}>
			<ArrowLeft class="mr-2 h-4 w-4" />
			Back
		</Button.Root>

		{#if currentStep < steps.length}
			<Button.Root type="button" onclick={nextStep}>
				Next
				<ArrowRight class="ml-2 h-4 w-4" />
			</Button.Root>
		{:else}
			<Button.Root type="button" onclick={submit} disabled={submitting}>
				<Save class="mr-2 h-4 w-4" />
				{submitting ? 'Saving...' : submitLabel}
			</Button.Root>
		{/if}
	</div>
</div>
