<script lang="ts">
	import type { Service, ServiceConfigFile } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as Button from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Textarea } from '$lib/components/ui/textarea';
	import * as Select from '$lib/components/ui/select';
	import { RefreshCw, Save, XCircle } from '@lucide/svelte';

	let {
		service,
		envContent = $bindable(''),
		configFiles = $bindable([]),
		savingEnv = $bindable(false),
		onSaveEnv,
		onSaveAndRestart
	}: {
		service: Service;
		envContent: string;
		configFiles: ServiceConfigFile[];
		savingEnv: boolean;
		onSaveEnv: () => Promise<void>;
		onSaveAndRestart: () => Promise<void>;
	} = $props();

	function addConfigFile() {
		configFiles = [...configFiles, { file_path: '', target: 'app_dir', content: '' }];
	}

	function removeConfigFile(index: number) {
		configFiles = configFiles.filter((_, i) => i !== index);
	}
</script>

<Card.Root class="border-border bg-card">
	<Card.Header class="pb-3">
		<div class="flex items-center justify-between">
			<div class="space-y-1">
				<Card.Title class="text-lg">Service Files</Card.Title>
				<Card.Description
					>Manage <code>{service.env_file || '.env'}</code> and any additional runtime config files
					for this service.</Card.Description
				>
			</div>
			<div class="flex items-center gap-2">
				<Button.Root variant="outline" size="sm" onclick={onSaveEnv} disabled={savingEnv}>
					{#if savingEnv}<RefreshCw class="mr-2 h-4 w-4 animate-spin" />{:else}<Save
							class="mr-2 h-4 w-4"
						/>{/if}
					Save
				</Button.Root>
				<Button.Root
					variant="default"
					size="sm"
					onclick={onSaveAndRestart}
					disabled={savingEnv}
					class="bg-amber-600 text-white hover:bg-amber-700"
				>
					<RefreshCw class="mr-2 h-4 w-4" /> Save & Restart
				</Button.Root>
			</div>
		</div>
	</Card.Header>
	<Card.Content class="space-y-4">
		<div class="space-y-2">
			<p class="text-sm text-muted-foreground">Primary env file</p>
			<Input value={service.env_file || '.env'} disabled />
		</div>
		<Textarea
			bind:value={envContent}
			class="min-h-[280px] font-mono text-sm text-blue-300"
			placeholder="KEY=VALUE"
		/>
		<p class="mt-2 text-xs italic text-muted-foreground">
			Note: Environment variables are written to <code>{service.env_file}</code> in the service's
			installation directory.
		</p>
		<div class="space-y-3 border-t border-border pt-4">
			<div class="flex items-center justify-between">
				<div>
					<h3 class="text-sm font-medium">Additional managed config files</h3>
					<p class="text-xs text-muted-foreground">
						Use <code>Current dir</code> for IIS files like <code>web.config</code> that must sit beside
						deployed static assets.
					</p>
				</div>
				<Button.Root variant="outline" size="sm" onclick={addConfigFile}> Add file </Button.Root>
			</div>
			{#if configFiles.length > 0}
				<div class="space-y-3">
					{#each configFiles as file, index (index)}
						<Card.Root class="border-border/70 bg-background/60">
							<Card.Content class="space-y-3 p-4">
								<div class="flex items-center justify-between">
									<p class="text-sm font-medium">Config file #{index + 1}</p>
									<Button.Root
										variant="ghost"
										size="icon"
										class="h-8 w-8 text-red-400 hover:text-red-300"
										onclick={() => removeConfigFile(index)}
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
										<Select.Trigger />
										<Select.Content>
											<Select.Item value="app_dir">Service/app dir</Select.Item>
											<Select.Item value="release_dir">Current dir</Select.Item>
										</Select.Content>
									</Select.Root>
								</div>
								<Textarea
									class="min-h-[180px] font-mono text-sm text-blue-300"
									bind:value={file.content}
									placeholder={'{\n  "featureFlag": true\n}'}
								/>
							</Card.Content>
						</Card.Root>
					{/each}
				</div>
			{:else}
				<div
					class="rounded-md border border-dashed border-border bg-muted/20 p-4 text-sm text-muted-foreground"
				>
					No extra config files yet.
				</div>
			{/if}
		</div>
	</Card.Content>
</Card.Root>
