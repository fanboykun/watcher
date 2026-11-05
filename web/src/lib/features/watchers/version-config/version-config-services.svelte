<script lang="ts">
	import ConfigWorkspace from '$lib/features/configuration/config-workspace.svelte';
	import type { createVersionConfig } from './version-config-controller.svelte';

	let {
		model,
		version,
		installDir
	}: {
		model: ReturnType<typeof createVersionConfig>;
		version: string;
		installDir: string;
	} = $props();
</script>

{#each model.ui.services as service (service.serviceId)}
	<section aria-label={`${service.serviceName} configuration for ${version}`} class="space-y-2">
		<p class="text-xs text-muted-foreground">
			{service.hasDraft
				? 'Saved edits differ from the configuration this version last ran with.'
				: 'Matches the configuration this version last ran with.'}
		</p>
		<ConfigWorkspace
			serviceName={service.serviceName}
			context={version}
			mode="rollback"
			baselineLabel="saved"
			directory={installDir}
			envPath={service.envPath}
			bind:envContent={service.env}
			bind:files={service.files}
			baselineEnv={service.snapshotEnv}
			baselineFiles={service.snapshotFiles}
			savedEnv={service.savedEnv}
			savedFiles={service.savedFiles}
			explicitFiles={true}
			savedExplicitFiles={true}
			disabled={model.readonly || model.saving || model.rollingBack}
		/>
	</section>
{:else}
	<p class="text-sm text-muted-foreground">This watcher has no services to configure.</p>
{/each}
