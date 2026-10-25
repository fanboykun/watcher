<script lang="ts">
	import ConfigWorkspace from '$lib/features/configuration/config-workspace.svelte';
	import { iisAppKindLabel, isIISService, serviceTypeLabel, type Service } from '$lib/api';
	import * as Button from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Select from '$lib/components/ui/select/index.js';
	import { Check, Plus, Trash2 } from '@lucide/svelte';
	import type { createWatcherSetup } from '$lib/features/watchers/setup/setup-controller.svelte';
	let { model }: { model: ReturnType<typeof createWatcherSetup> } = $props();
</script>

<div class="space-y-4">
	<div class="wizard-footer">
		<div>
			<p class="font-medium">Managed services</p>
			<p class="text-sm text-muted-foreground">
				Define one or more services that this watcher should manage under the same release source.
			</p>
		</div>
		<Button.Root variant="outline" size="sm" type="button" onclick={model.addServiceDraft}>
			<Plus class="mr-2 h-4 w-4" /> Add Service
		</Button.Root>
	</div>

	{#if model.ui.serviceDrafts.length === 0}
		<div
			class="rounded-lg border border-dashed border-border/70 bg-muted/20 p-8 text-center text-sm text-muted-foreground"
		>
			Add at least one service definition before creating the watcher.
		</div>
	{:else}
		{#each model.ui.serviceDrafts as svc, i (i)}
			<div class="relative space-y-3 rounded-lg border border-border/70 bg-card p-4">
				<Button.Root
					variant="ghost"
					size="icon"
					class="absolute top-3 right-3 h-8 w-8 text-red-400"
					type="button"
					onclick={() => model.removeServiceDraft(i)}
					disabled={model.ui.serviceDrafts.length === 1}
				>
					<Trash2 class="h-4 w-4" />
				</Button.Root>

				<div class="pr-10">
					<p class="font-medium">Service #{i + 1}</p>
					<p class="text-xs text-muted-foreground">Runtime definition stored under this watcher.</p>
				</div>

				<div class="grid gap-4 md:grid-cols-2">
					<div class="space-y-2">
						<Label>Hosting Mode</Label>
						<Select.Root type="single" bind:value={svc.service_type}>
							<Select.Trigger>
								{isIISService(svc.service_type || 'nssm') ? 'IIS Site' : 'NSSM Native Windows'}
							</Select.Trigger>
							<Select.Content>
								<Select.Item value="nssm" label="NSSM Native Windows"
									>NSSM Native Windows</Select.Item
								>
								<Select.Item value="iis" label="IIS Site">IIS Site</Select.Item>
							</Select.Content>
						</Select.Root>
					</div>
					<div class="space-y-2">
						<Label
							>{isIISService(svc.service_type || 'nssm')
								? 'Service Identifier'
								: 'Windows Service Name'}</Label
						>
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
					{:else}
						<div class="space-y-2 md:col-span-2">
							<Label>IIS App Kind</Label>
							<Select.Root type="single" bind:value={svc.iis_app_kind}>
								<Select.Trigger>
									{model.iisAppKinds.find((kind) => kind.value === (svc.iis_app_kind || 'static'))
										?.label || 'Select kind'}
								</Select.Trigger>
								<Select.Content>
									{#each model.iisAppKinds as kind (kind.value)}
										<Select.Item value={kind.value} label={kind.label}>{kind.label}</Select.Item>
									{/each}
								</Select.Content>
							</Select.Root>
							<p class="text-xs text-muted-foreground">
								{model.iisAppKinds.find((kind) => kind.value === (svc.iis_app_kind || 'static'))
									?.hint}
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
							<Input
								bind:value={svc.health_check_url}
								placeholder="https://app.example.com/health"
							/>
						</div>
						<div
							class="rounded-lg border border-border/70 bg-muted/20 p-3 text-xs text-muted-foreground md:col-span-2"
						>
							<span class="font-medium text-foreground/90"
								>{serviceTypeLabel(svc.service_type || 'nssm')}:</span
							>
							{iisAppKindLabel(String(svc.iis_app_kind || 'static'))}. Watcher will set the IIS app
							pool runtime automatically for this profile.
						</div>
					{/if}

					<div class="min-w-0 md:col-span-2">
						<ConfigWorkspace
							serviceName={svc.windows_service_name || `Service ${i + 1}`}
							context="Initial configuration"
							mode="form"
							envPath={svc.env_file || ''}
							bind:envContent={svc.env_content}
							bind:files={svc.config_files}
							disabled={model.creating}
						/>
					</div>
				</div>
			</div>
		{/each}
	{/if}
</div>
