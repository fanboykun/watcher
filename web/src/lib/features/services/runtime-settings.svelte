<script lang="ts">
	import * as Select from '$lib/components/ui/select';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { iisAppKindLabel, type ServiceType, type IISAppKind } from '$lib/api';
	let {
		serviceType,
		iisAppKinds,
		binaryName = $bindable(''),
		startArguments = $bindable(''),
		envFile = $bindable(''),
		iisAppKind = $bindable<IISAppKind>('static'),
		iisAppPool = $bindable(''),
		iisSiteName = $bindable('')
	}: {
		serviceType: ServiceType;
		iisAppKinds: Array<{ value: IISAppKind; label: string; hint: string }>;
		binaryName?: string;
		startArguments?: string;
		envFile?: string;
		iisAppKind?: IISAppKind;
		iisAppPool?: string;
		iisSiteName?: string;
	} = $props();
</script>

<section class="console-panel" aria-label="Runtime settings">
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
						Executable path relative to the extracted release. It must match the artifact filename.
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
						Environment file path relative to the watcher installation directory. Leave blank if
						unused.
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
					{iisAppKindLabel(iisAppKind)}. Watcher will set the IIS managed runtime automatically for
					this app kind.
				</div>
			</div>
		{/if}
	</div>
</section>
