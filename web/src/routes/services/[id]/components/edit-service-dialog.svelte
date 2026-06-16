<script lang="ts">
	import { isIISService, iisAppKindLabel, type Service, type ServiceConfigFile, type IISAppKind, type ServiceWritePayload } from '$lib/api';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Button from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Textarea } from '$lib/components/ui/textarea';
	import * as Select from '$lib/components/ui/select/index.js';
	import { Label } from '$lib/components/ui/label';
	import { Plus, XCircle } from '@lucide/svelte';

	let {
		open = $bindable(false),
		service,
		onSave
	}: {
		open: boolean;
		service: Service | null;
		onSave: (data: ServiceWritePayload) => Promise<void> | void;
	} = $props();

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

	let editSvcType = $state<'nssm' | 'iis'>('nssm');
	let editSvcName = $state('');
	let editSvcBinary = $state('');
	let editSvcStartArguments = $state('');
	let editSvcEnvFile = $state('');
	let editSvcEnvContent = $state('');
	let editSvcConfigFiles = $state<ServiceConfigFile[]>([]);
	let editSvcHealthURL = $state('');
	let editSvcIISAppKind = $state<IISAppKind>('static');
	let editSvcIISAppPool = $state('');
	let editSvcIISSiteName = $state('');
	let editSvcIISManagedRuntime = $state('');
	let editSvcPublicURL = $state('');

	let submitting = $state(false);
	let error = $state('');

	$effect(() => {
		if (open && service) {
			editSvcType = isIISService(service.service_type) ? 'iis' : 'nssm';
			editSvcName = service.windows_service_name;
			editSvcBinary = service.binary_name || '';
			editSvcStartArguments = service.start_arguments || '';
			editSvcEnvFile = service.env_file || '';
			editSvcEnvContent = service.env_content || '';
			editSvcConfigFiles = (service.config_files || []).map((file) => ({
				id: file.id,
				service_id: file.service_id,
				file_path: file.file_path,
				target: file.target || 'app_dir',
				content: file.content
			}));
			editSvcHealthURL = service.health_check_url || '';
			editSvcIISAppKind = service.iis_app_kind || 'static';
			editSvcIISAppPool = service.iis_app_pool || '';
			editSvcIISSiteName = service.iis_site_name || '';
			editSvcIISManagedRuntime = service.iis_managed_runtime || '';
			editSvcPublicURL = service.public_url || '';
			error = '';
		}
	});

	function addEditSvcConfigFile() {
		editSvcConfigFiles = [...editSvcConfigFiles, { file_path: '', target: 'app_dir', content: '' }];
	}

	function removeEditSvcConfigFile(index: number) {
		editSvcConfigFiles = editSvcConfigFiles.filter((_, i) => i !== index);
	}

	async function handleSubmit() {
		if (!service) return;
		submitting = true;
		error = '';
		try {
			await onSave({
				service_type: editSvcType,
				windows_service_name: editSvcName,
				binary_name: editSvcBinary,
				start_arguments: editSvcStartArguments,
				env_file: editSvcEnvFile,
				env_content: editSvcEnvContent,
				config_files: editSvcConfigFiles.filter((file) => file.file_path.trim() !== ''),
				health_check_url: editSvcHealthURL,
				iis_app_kind: editSvcIISAppKind,
				iis_app_pool: editSvcIISAppPool,
				iis_site_name: editSvcIISSiteName,
				iis_managed_runtime: editSvcIISManagedRuntime,
				public_url: editSvcPublicURL
			});
			open = false;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save service changes';
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
			<Dialog.Header class="shrink-0 border-b border-border/70 px-6 pb-4 pt-6">
				<Dialog.Title>Edit Service</Dialog.Title>
				<Dialog.Description>Update how this watcher manages this service</Dialog.Description>
			</Dialog.Header>
			<div class="flex-1 space-y-5 overflow-y-auto px-6 py-5">
				{#if error}
					<div class="rounded border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-400">
						{error}
					</div>
				{/if}

				<div class="grid gap-4 md:grid-cols-2">
					<div class="space-y-2">
						<Label for="editSvcType">Hosting Mode</Label>
						<Select.Root type="single" bind:value={editSvcType}>
							<Select.Trigger id="editSvcType">
								{editSvcType === 'iis' ? 'IIS Site' : 'Binary (NSSM)'}
							</Select.Trigger>
							<Select.Content>
								<Select.Item value="nssm" label="Binary (NSSM)">Binary (NSSM)</Select.Item>
								<Select.Item value="iis" label="IIS Site">IIS Site</Select.Item>
							</Select.Content>
						</Select.Root>
					</div>
					<div class="space-y-2">
						<Label for="editSvcName"
							>{editSvcType === 'iis' ? 'Service Identifier' : 'Windows Service Name'}</Label
						>
						<Input
							id="editSvcName"
							placeholder={editSvcType === 'iis' ? 'marketing-site' : 'my-app-web-1'}
							bind:value={editSvcName}
							required
						/>
					</div>

					{#if editSvcType === 'nssm'}
						<div class="space-y-2">
							<Label for="editSvcBinary">Binary Name</Label>
							<Input
								id="editSvcBinary"
								placeholder="my-app.exe"
								bind:value={editSvcBinary}
								required
							/>
						</div>
						<div class="space-y-2">
							<Label for="editSvcStartArguments">Start Arguments (optional)</Label>
							<Input
								id="editSvcStartArguments"
								placeholder="serve --port 8080"
								bind:value={editSvcStartArguments}
							/>
						</div>
						<div class="space-y-2 md:col-span-2">
							<Label for="editSvcEnvFile">Env File (optional)</Label>
							<Input
								id="editSvcEnvFile"
								placeholder="C:\apps\my-app\.env.web.1"
								bind:value={editSvcEnvFile}
							/>
						</div>
						<div class="space-y-2 md:col-span-2">
							<Label for="editSvcEnvContent">Env Content (optional)</Label>
							<Textarea
								id="editSvcEnvContent"
								class="min-h-[180px] font-mono text-xs text-blue-300"
								bind:value={editSvcEnvContent}
								placeholder="KEY=VALUE&#10;API_URL=https://example.com"
							/>
							<p class="text-xs text-muted-foreground">
								If set, watcher writes this content into <code>{editSvcEnvFile || '.env'}</code> during
								service sync/deploy.
							</p>
						</div>
					{:else}
						<div class="space-y-2 md:col-span-2">
							<Label for="editSvcIISAppKind">IIS App Kind</Label>
							<Select.Root type="single" bind:value={editSvcIISAppKind}>
								<Select.Trigger id="editSvcIISAppKind">
									{iisAppKinds.find((k) => k.value === editSvcIISAppKind)?.label || 'Select kind'}
								</Select.Trigger>
								<Select.Content>
									{#each iisAppKinds as kind (kind.value)}
										<Select.Item value={kind.value} label={kind.label}>{kind.label}</Select.Item>
									{/each}
								</Select.Content>
							</Select.Root>
							<p class="text-xs text-muted-foreground">
								{iisAppKinds.find((kind) => kind.value === editSvcIISAppKind)?.hint}
							</p>
						</div>
						<div class="space-y-2">
							<Label for="editSvcIISAppPool">IIS App Pool Name</Label>
							<Input id="editSvcIISAppPool" placeholder="my-frontend" bind:value={editSvcIISAppPool} />
						</div>
						<div class="space-y-2">
							<Label for="editSvcIISSiteName">IIS Site Name</Label>
							<Input
								id="editSvcIISSiteName"
								placeholder="my-frontend"
								bind:value={editSvcIISSiteName}
							/>
						</div>
						<div
							class="rounded-md border border-border/70 bg-muted/20 p-3 text-xs text-muted-foreground md:col-span-2"
						>
							<span class="font-medium text-foreground/90">Bootstrap profile:</span>
							{' '}{iisAppKindLabel(editSvcIISAppKind)}. Watcher will set the IIS managed runtime automatically
							for this app kind.
						</div>
					{/if}

					<div class="space-y-2">
						<Label for="editSvcHealthURL">Health Check URL (optional)</Label>
						<Input
							id="editSvcHealthURL"
							placeholder="http://localhost:3000/health"
							bind:value={editSvcHealthURL}
						/>
					</div>
					<div class="space-y-2">
						<Label for="editSvcPublicURL">Public URL</Label>
						<Input
							id="editSvcPublicURL"
							placeholder="https://my-app.example.com"
							bind:value={editSvcPublicURL}
						/>
						{#if editSvcType === 'iis'}
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
							<p class="text-xs text-muted-foreground">
								Store runtime-generated config alongside this service. Use <code>Current dir</code> for
								IIS files like <code>web.config</code>.
							</p>
						</div>
						<Button.Root
							variant="outline"
							size="sm"
							type="button"
							class="h-8 shrink-0"
							onclick={addEditSvcConfigFile}
						>
							<Plus class="mr-1.5 h-3 w-3" /> Add file
						</Button.Root>
					</div>
					{#if editSvcConfigFiles.length > 0}
						<div class="space-y-3 rounded-md border border-border/70 bg-background/50 p-3">
							{#each editSvcConfigFiles as file, fileIndex (fileIndex)}
								<div class="space-y-2 rounded-md border border-border/60 bg-card/60 p-3">
									<div class="flex items-center justify-between">
										<Label>Config file #{fileIndex + 1}</Label>
										<Button.Root
											variant="ghost"
											size="icon"
											type="button"
											class="h-7 w-7 text-red-400 hover:text-red-300"
											onclick={() => removeEditSvcConfigFile(fileIndex)}
										>
											<XCircle class="h-4 w-4" />
										</Button.Root>
									</div>
									<div class="grid gap-2 sm:grid-cols-[1fr_160px]">
										<Input
											bind:value={file.file_path}
											placeholder="web.config or settings/appsettings.json"
										/>
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
										class="min-h-[140px] font-mono text-xs text-blue-300"
										bind:value={file.content}
										placeholder={'{\n  "featureFlag": true\n}'}
									/>
								</div>
							{/each}
						</div>
					{:else}
						<p class="text-xs text-muted-foreground">
							Use this for runtime files like <code>config.json</code>, <code>appsettings.json</code>,
							or other generated config.
						</p>
					{/if}
				</div>
			</div>
			<Dialog.Footer class="shrink-0 border-t border-border/70 px-6 pb-4 pt-4">
				<Button.Root variant="outline" type="button" onclick={() => (open = false)}>
					Cancel
				</Button.Root>
				<Button.Root type="submit" disabled={submitting}>
					{submitting ? 'Saving...' : 'Save Service'}
				</Button.Root>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
