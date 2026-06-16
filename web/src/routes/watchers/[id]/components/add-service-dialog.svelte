<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Button from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Select from '$lib/components/ui/select/index.js';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Plus, Trash2 } from '@lucide/svelte';
	import { iisAppKindLabel, type ServiceConfigFile, type IISAppKind, type ServiceWritePayload } from '$lib/api';

	interface Props {
		open: boolean;
		onServiceAdded: (data: ServiceWritePayload) => Promise<void> | void;
	}

	let {
		open = $bindable(false),
		onServiceAdded
	}: Props = $props();

	const iisAppKinds: Array<{ value: IISAppKind; label: string; hint: string }> = [
		{
			value: 'static',
			label: 'Static Site',
			hint: 'Frontend build or static files served directly by IIS.'
		},
		{
			value: 'php',
			label: 'PHP',
			hint: 'PHP app on IIS with FastCGI and PHP already installed.'
		},
		{
			value: 'aspnet_classic',
			label: 'ASP.NET Classic',
			hint: 'Classic ASP.NET app using the managed CLR app pool.'
		}
	];

	let svcType = $state<'nssm' | 'iis'>('nssm');
	let svcName = $state('');
	let svcBinary = $state('');
	let svcStartArguments = $state('');
	let svcEnvFile = $state('');
	let svcEnvContent = $state('');
	let svcConfigFiles = $state<ServiceConfigFile[]>([]);
	let svcHealthURL = $state('');
	let svcIISAppKind = $state<IISAppKind>('static');
	let svcIISAppPool = $state('');
	let svcIISSiteName = $state('');
	let svcIISManagedRuntime = $state('');
	let svcPublicURL = $state('');

	let submitting = $state(false);
	let error = $state('');

	$effect(() => {
		if (open) {
			// Reset on open
			svcType = 'nssm';
			svcName = '';
			svcBinary = '';
			svcStartArguments = '';
			svcEnvFile = '';
			svcEnvContent = '';
			svcConfigFiles = [];
			svcHealthURL = '';
			svcIISAppKind = 'static';
			svcIISAppPool = '';
			svcIISSiteName = '';
			svcIISManagedRuntime = '';
			svcPublicURL = '';
			error = '';
		}
	});

	function addSvcConfigFile() {
		svcConfigFiles = [...svcConfigFiles, { file_path: '', target: 'app_dir' as const, content: '' }];
	}

	function removeSvcConfigFile(index: number) {
		svcConfigFiles = svcConfigFiles.filter((_, i) => i !== index);
	}

	async function handleSubmit() {
		submitting = true;
		error = '';
		try {
			await onServiceAdded({
				service_type: svcType,
				windows_service_name: svcName,
				binary_name: svcBinary,
				start_arguments: svcStartArguments,
				env_file: svcEnvFile,
				env_content: svcEnvContent,
				config_files: svcConfigFiles.filter((file) => file.file_path.trim() !== ''),
				health_check_url: svcHealthURL,
				iis_app_kind: svcIISAppKind,
				iis_app_pool: svcIISAppPool,
				iis_site_name: svcIISSiteName,
				iis_managed_runtime: svcIISManagedRuntime,
				public_url: svcPublicURL
			});
			open = false;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to add service';
		} finally {
			submitting = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="max-h-[90vh] w-[min(96vw,56rem)] overflow-hidden p-0 sm:max-w-3xl">
		<form
			class="flex max-h-[calc(90vh-5.5rem)] flex-col"
			onsubmit={(e) => {
				e.preventDefault();
				handleSubmit();
			}}
		>
			<Dialog.Header class="shrink-0 border-b border-border/70 px-6 pt-6 pb-4">
				<Dialog.Title>Add Service</Dialog.Title>
				<Dialog.Description>Register a service for this watcher to manage</Dialog.Description>
			</Dialog.Header>
			<div class="flex-1 space-y-5 overflow-y-auto px-6 py-5">
				{#if error}
					<div class="rounded border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-400">
						{error}
					</div>
				{/if}

				<div class="grid gap-4 md:grid-cols-2">
					<div class="space-y-2">
						<Label for="svcType">Hosting Mode</Label>
						<Select.Root type="single" bind:value={svcType}>
							<Select.Trigger id="svcType">
								{svcType === 'iis' ? 'IIS Site' : 'Binary (NSSM)'}
							</Select.Trigger>
							<Select.Content>
								<Select.Item value="nssm" label="Binary (NSSM)">Binary (NSSM)</Select.Item>
								<Select.Item value="iis" label="IIS Site">IIS Site</Select.Item>
							</Select.Content>
						</Select.Root>
					</div>
					<div class="space-y-2">
						<Label for="svcName">
							{svcType === 'iis' ? 'Service Identifier' : 'Windows Service Name'}
						</Label>
						<Input
							id="svcName"
							placeholder={svcType === 'iis' ? 'marketing-site' : 'my-app-web-1'}
							bind:value={svcName}
							required
						/>
					</div>

					{#if svcType === 'nssm'}
						<div class="space-y-2">
							<Label for="svcBinary">Binary Name</Label>
							<Input id="svcBinary" placeholder="my-app.exe" bind:value={svcBinary} required />
						</div>
						<div class="space-y-2">
							<Label for="svcStartArguments">Start Arguments (optional)</Label>
							<Input id="svcStartArguments" placeholder="serve --port 8080" bind:value={svcStartArguments} />
						</div>
						<div class="space-y-2 md:col-span-2">
							<Label for="svcEnvFile">Env File (optional)</Label>
							<Input id="svcEnvFile" placeholder="C:\apps\my-app\.env.web.1" bind:value={svcEnvFile} />
						</div>
						<div class="space-y-2 md:col-span-2">
							<Label for="svcEnvContent">Env Content (optional)</Label>
							<Textarea
								id="svcEnvContent"
								class="min-h-45 font-mono text-xs text-blue-300"
								bind:value={svcEnvContent}
								placeholder="KEY=VALUE&#10;API_URL=https://example.com"
							/>
							<p class="text-xs text-muted-foreground">
								If set, watcher writes this content into <code>{svcEnvFile || '.env'}</code> during service sync/deploy.
							</p>
						</div>
					{:else}
						<div class="space-y-2 md:col-span-2">
							<Label for="svcIISAppKind">IIS App Kind</Label>
							<Select.Root type="single" bind:value={svcIISAppKind}>
								<Select.Trigger id="svcIISAppKind">
									{iisAppKinds.find((kind) => kind.value === svcIISAppKind)?.label || 'Select kind'}
								</Select.Trigger>
								<Select.Content>
									{#each iisAppKinds as kind (kind.value)}
										<Select.Item value={kind.value} label={kind.label}>{kind.label}</Select.Item>
									{/each}
								</Select.Content>
							</Select.Root>
							<p class="text-xs text-muted-foreground">
								{iisAppKinds.find((kind) => kind.value === svcIISAppKind)?.hint}
							</p>
						</div>
						<div class="space-y-2">
							<Label for="svcIISAppPool">IIS App Pool Name</Label>
							<Input id="svcIISAppPool" placeholder="my-frontend" bind:value={svcIISAppPool} />
						</div>
						<div class="space-y-2">
							<Label for="svcIISSiteName">IIS Site Name</Label>
							<Input id="svcIISSiteName" placeholder="my-frontend" bind:value={svcIISSiteName} />
						</div>
						<div class="rounded-md border border-border/70 bg-muted/20 p-3 text-xs text-muted-foreground md:col-span-2">
							<span class="font-medium text-foreground/90">Bootstrap profile:</span>
							{iisAppKindLabel(svcIISAppKind)}. Watcher will set the IIS managed runtime automatically for this app kind.
						</div>
					{/if}

					<div class="space-y-2">
						<Label for="svcHealthURL">Health Check URL (optional)</Label>
						<Input
							id="svcHealthURL"
							placeholder="http://localhost:3000/health"
							bind:value={svcHealthURL}
						/>
					</div>
					<div class="space-y-2">
						<Label for="svcPublicURL">Public URL</Label>
						<Input
							id="svcPublicURL"
							placeholder="https://my-app.example.com"
							bind:value={svcPublicURL}
						/>
						{#if svcType === 'iis'}
							<p class="text-xs text-muted-foreground">
								Needed when Watcher must create the IIS site and binding automatically.
							</p>
						{/if}
					</div>
				</div>

				<div class="space-y-3">
					<div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
						<div>
							<Label>Additional managed config files</Label>
							<p class="text-xs text-muted-foreground">Store runtime-generated config alongside this service. Use <code>Current dir</code> for IIS files like <code>web.config</code>.</p>
						</div>
						<Button.Root variant="outline" size="sm" type="button" class="h-8 shrink-0" onclick={addSvcConfigFile}>
							<Plus class="mr-1.5 h-3 w-3" /> Add file
						</Button.Root>
					</div>
					{#if svcConfigFiles.length > 0}
						<div class="space-y-3 rounded-md border border-border/70 bg-background/50 p-3">
							{#each svcConfigFiles as file, fileIndex (fileIndex)}
								<div class="space-y-2 rounded-md border border-border/60 bg-card/60 p-3">
									<div class="flex items-center justify-between">
										<Label>Config file #{fileIndex + 1}</Label>
										<Button.Root
											variant="ghost"
											size="icon"
											type="button"
											class="h-7 w-7 text-red-400 hover:text-red-300"
											onclick={() => removeSvcConfigFile(fileIndex)}
										>
											<Trash2 class="h-3 w-3" />
										</Button.Root>
									</div>
									<div class="grid gap-2 sm:grid-cols-[1fr_160px]">
										<Input bind:value={file.file_path} placeholder="web.config or settings/appsettings.json" />
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
										class="min-h-35 font-mono text-xs text-blue-300"
										bind:value={file.content}
										placeholder={'{\n  "featureFlag": true\n}'}
									/>
								</div>
							{/each}
						</div>
					{:else}
						<p class="text-xs text-muted-foreground">
							Use this for runtime files like <code>config.json</code>, <code>appsettings.json</code>, or other generated config.
						</p>
					{/if}
				</div>
			</div>
			<Dialog.Footer class="shrink-0 border-t border-border/70 px-6 py-4">
				<Button.Root variant="outline" type="button" onclick={() => (open = false)}>Cancel</Button.Root>
				<Button.Root type="submit" disabled={submitting}>
					{submitting ? 'Adding...' : 'Add Service'}
				</Button.Root>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
