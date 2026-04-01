<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type Watcher, type InspectRepoResponse, type Service } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Button from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Select } from '$lib/components/ui/select';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Eye, Plus, Trash2, Zap, Clock, AlertCircle, ArrowRight, Check } from '@lucide/svelte';
	import { resolve } from '$app/paths';
	import { goto } from '$app/navigation';

	let watchers = $state<Watcher[]>([]);
	let error = $state('');
	let triggerMsg = $state('');
	let showCreate = $state(false);
	let creating = $state(false);
	let inspecting = $state(false);
	let showDeleteDialog = $state(false);
	let deleting = $state(false);
	let deleteWatcherID = $state<number | null>(null);
	let deleteWatcherName = $state('');

	let createStep = $state(1);
	let inspectResult = $state<InspectRepoResponse | null>(null);

	// Create form fields
	let formName = $state('');
	let formServiceName = $state('');
	let formMetadataURL = $state('');
	let formInstallDir = $state('');
	let formInterval = $state(60);
	let formHcEnabled = $state(false);
	let formHcURL = $state('');
	let formDeploymentEnvironment = $state('');
	let formGitHubToken = $state('');
	let useCustomGitHubToken = $state(false);
	let formServices = $state<Partial<Service>[]>([]);

	onMount(load);

	async function load() {
		try {
			watchers = await api.listWatchers();
			error = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load watchers';
		}
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
			// Trim to proper repo URL if accidentally copied trailing parts
			let cleaned = formMetadataURL.split('/releases')[0];
			const token = useCustomGitHubToken ? formGitHubToken.trim() : '';
			inspectResult = await api.inspectRepo(cleaned, token);
			formMetadataURL = cleaned;
			
			const parts = cleaned.split('/');
			const repoName = parts[parts.length - 1] || 'my-app';
			formName = repoName;
			formServiceName = repoName;
			formInstallDir = `C:\\apps\\${repoName}`;
			
			createStep = 2;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Inspect failed';
		} finally {
			inspecting = false;
		}
	}

	function jumpToNext() {
		createStep = 3;
		if (formServices.length === 0) {
			formServices = [{
				service_type: 'nssm',
				windows_service_name: formServiceName,
				binary_name: formServiceName ? `${formServiceName}.exe` : 'app.exe',
				start_arguments: '',
				env_file: '.env',
				env_content: '',
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
				deployment_environment: formDeploymentEnvironment,
				github_token: useCustomGitHubToken ? formGitHubToken.trim() : '',
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

			showCreate = false;
			resetForm();
			await load();
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
		formInterval = 60;
		formHcEnabled = false;
		formHcURL = '';
		formDeploymentEnvironment = '';
		formGitHubToken = '';
		useCustomGitHubToken = false;
		formServices = [];
		inspectResult = null;
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
			config_files: [],
		}];
	}
	
	function removeServiceDraft(idx: number) {
		formServices = formServices.filter((_, i) => i !== idx);
	}

	function addConfigFileDraft(serviceIndex: number) {
		const next = [...formServices];
		const svc = next[serviceIndex];
		const configFiles = [...(svc.config_files || []), { file_path: '', content: '' }];
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

	async function triggerCheck(id: number) {
		try {
			const res = await api.triggerCheck(id);
			triggerMsg = res.message;
			setTimeout(() => (triggerMsg = ''), 3000);
		} catch (e) {
			triggerMsg = e instanceof Error ? e.message : 'Trigger failed';
		}
	}

	function openDeleteWatcherDialog(id: number, name: string) {
		deleteWatcherID = id;
		deleteWatcherName = name;
		showDeleteDialog = true;
	}

	async function confirmDeleteWatcher() {
		if (!deleteWatcherID) return;
		deleting = true;
		try {
			await api.deleteWatcher(deleteWatcherID);
			await load();
			showDeleteDialog = false;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Delete failed';
		} finally {
			deleting = false;
		}
	}

	function statusColor(s: string) {
		switch (s) {
			case 'healthy':
				return 'bg-emerald-500/15 text-emerald-400 border-emerald-500/30';
			case 'deploying':
				return 'bg-blue-500/15 text-blue-400 border-blue-500/30';
			case 'failed':
				return 'bg-red-500/15 text-red-400 border-red-500/30';
			case 'rollback':
				return 'bg-amber-500/15 text-amber-400 border-amber-500/30';
			default:
				return 'bg-muted text-muted-foreground border-border';
		}
	}

	function timeAgo(ts: string | null): string {
		if (!ts) return '—';
		const diff = Date.now() - new Date(ts).getTime();
		const mins = Math.floor(diff / 60000);
		if (mins < 1) return 'just now';
		if (mins < 60) return `${mins}m ago`;
		const hrs = Math.floor(mins / 60);
		if (hrs < 24) return `${hrs}h ago`;
		return `${Math.floor(hrs / 24)}d ago`;
	}
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold tracking-tight">Watchers</h1>
			<p class="text-sm text-muted-foreground">Repository poll loops</p>
		</div>
		<Button.Root onclick={() => {resetForm(); showCreate = true;}}>
			<Plus class="mr-2 h-4 w-4" /> Add Watcher
		</Button.Root>
	</div>

	{#if error && !showCreate}
		<div class="rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-400">
			<AlertCircle class="mr-2 inline h-4 w-4" />
			{error}
		</div>
	{/if}

	{#if triggerMsg}
		<div class="rounded-lg border border-blue-500/30 bg-blue-500/10 p-4 text-sm text-blue-400">
			<Zap class="mr-2 inline h-4 w-4" />
			{triggerMsg}
		</div>
	{/if}

	{#if watchers.length > 0}
		<Card.Root class="border-border bg-card">
			<Table.Root>
				<Table.Header>
					<Table.Row class="border-border hover:bg-transparent">
						<Table.Head>Name</Table.Head>
						<Table.Head>Status</Table.Head>
						<Table.Head>Version</Table.Head>
						<Table.Head>Last Checked</Table.Head>
						<Table.Head>Services</Table.Head>
						<Table.Head class="text-right">Actions</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each watchers as w (w.id)}
						<Table.Row class="border-border">
							<Table.Cell>
								<a href={resolve(`/watchers/${w.id}`)} class="font-medium hover:underline"
									>{w.name}</a
								>
								<p class="font-mono text-xs text-muted-foreground">{w.service_name}</p>
							</Table.Cell>
							<Table.Cell>
								<span
									class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium capitalize {statusColor(
										w.status
									)}"
								>
									{w.status}
								</span>
							</Table.Cell>
							<Table.Cell>
								<span class="font-mono text-sm">{w.current_version || '—'}</span>
							</Table.Cell>
							<Table.Cell class="text-muted-foreground">
								{timeAgo(w.last_checked)}
							</Table.Cell>
							<Table.Cell class="text-muted-foreground">
								{w.services.length}
							</Table.Cell>
							<Table.Cell class="text-right">
								<div class="flex items-center justify-end gap-1">
									<Button.Root
										variant="ghost"
										size="icon"
										class="h-8 w-8"
										onclick={() => triggerCheck(w.id)}
										title="Trigger check"
									>
										<Zap class="h-4 w-4" />
									</Button.Root>
									<Button.Root
										variant="ghost"
										size="icon"
										class="h-8 w-8 text-red-400 hover:text-red-300"
										onclick={() => openDeleteWatcherDialog(w.id, w.name)}
										title="Delete"
									>
										<Trash2 class="h-4 w-4" />
									</Button.Root>
								</div>
							</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		</Card.Root>
	{:else if !error || showCreate}
		<Card.Root class="border-dashed border-border bg-card">
			<Card.Content class="flex flex-col items-center justify-center py-16 text-center">
				<Eye class="mb-3 h-10 w-10 text-muted-foreground/40" />
				<h3 class="text-sm font-medium text-muted-foreground">No watchers yet</h3>
				<p class="mt-1 text-xs text-muted-foreground/60">Click "Add Watcher" to get started</p>
			</Card.Content>
		</Card.Root>
	{/if}
</div>

<!-- Create Watcher Dialog Multi-step -->
<Dialog.Root bind:open={showCreate}>
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
						<div class="space-y-3">
							<Label for="metadataURL">GitHub Repository URL</Label>
							<Input
								id="metadataURL"
								placeholder="https://github.com/org/repo"
								bind:value={formMetadataURL}
								required
							/>
						</div>
						<div class="space-y-2 rounded-md border border-border bg-muted/20 p-3">
							<label class="inline-flex items-center gap-2 text-sm">
								<input type="checkbox" bind:checked={useCustomGitHubToken} class="rounded border-border" />
								Use custom GitHub token for this watcher
							</label>
							<Input
								id="watcherGithubTokenStep1"
								type="password"
								placeholder="ghp_xxx"
								bind:value={formGitHubToken}
								disabled={!useCustomGitHubToken}
							/>
							<p class="text-xs text-muted-foreground">
								Useful for private repos or when global <code>GITHUB_TOKEN</code> cannot access this repo.
							</p>
							<p class="text-xs text-muted-foreground">
								Fine-grained PAT minimum: <code>Contents: Read</code>. If deployment status reporting is used, also add <code>Deployments: Read and write</code>.
							</p>
							<div class="space-y-1 rounded border border-border bg-background/60 p-2 text-xs text-muted-foreground">
								<p class="font-medium text-foreground/90">Org private repo checklist</p>
								<p>1. Token has access to the target org repo(s).</p>
								<p>2. SSO/SAML authorization for this token is approved.</p>
								<p>3. Token owner has repo/team access in the org.</p>
								<p>4. Org policy allows PAT type being used (fine-grained/classic).</p>
								<p>5. Repo has at least one published release (not only draft/prerelease).</p>
							</div>
						</div>
						<p class="text-xs text-muted-foreground">
							Supported: Public & Private Repositories (if token configured).
							We will fetch the latest release and find the corresponding assets.
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
							<div class="space-y-2 text-xs text-muted-foreground">
								<p>GitHub token mode:</p>
								<p class="font-medium">{useCustomGitHubToken && formGitHubToken.trim() ? 'Custom watcher token configured' : 'Using global GITHUB_TOKEN'}</p>
							</div>
						</div>

						<div class="flex items-center gap-2">
							<input
								type="checkbox"
								id="hcEnabled"
								bind:checked={formHcEnabled}
								class="rounded border-border"
							/>
							<Label for="hcEnabled">Enable Health Checks across services</Label>
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
										<Label class="text-xs">Type</Label>
										<Select bind:value={svc.service_type} class="h-8 text-xs">
											<option value="nssm">NSSM Native Windows</option>
											<option value="static">Static IIS App</option>
										</Select>
									</div>
									<div class="space-y-1">
										<Label class="text-xs">Window Service Name</Label>
										<Input class="h-8 text-xs" bind:value={svc.windows_service_name} placeholder="myapp-web" />
									</div>
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
														<Input class="h-8 text-xs" bind:value={file.file_path} placeholder="config.json or config/appsettings.json" />
														<Textarea
															class="min-h-[120px] font-mono text-xs text-blue-300"
															bind:value={file.content}
															placeholder={'{\n  "port": 3000\n}'}
														/>
													</div>
												{/each}
											</div>
										{:else}
											<p class="text-[11px] text-muted-foreground">Use this for files like <code>config.json</code>, <code>appsettings.json</code>, or other runtime config.</p>
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
					<Button.Root variant="outline" type="button" onclick={() => { showCreate = false; resetForm(); }}>
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

<Dialog.Root bind:open={showDeleteDialog}>
	<Dialog.Content class="sm:max-w-[420px]">
		<Dialog.Header>
			<Dialog.Title>Delete Watcher</Dialog.Title>
			<Dialog.Description>
				This will delete watcher <span class="font-medium">{deleteWatcherName}</span> and all linked services.
			</Dialog.Description>
		</Dialog.Header>
		<Dialog.Footer>
			<Button.Root variant="outline" type="button" onclick={() => (showDeleteDialog = false)} disabled={deleting}>
				Cancel
			</Button.Root>
			<Button.Root type="button" class="bg-red-600 text-white hover:bg-red-700" onclick={confirmDeleteWatcher} disabled={deleting}>
				{deleting ? 'Deleting...' : 'Delete'}
			</Button.Root>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
