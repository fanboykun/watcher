<script lang="ts">
	import ConfigWorkspace from '$lib/features/configuration/config-workspace.svelte';
	import * as Button from '$lib/components/ui/button';
	import type { createCandidate } from '$lib/features/watchers/candidates/candidate-controller.svelte';
	let { model }: { model: ReturnType<typeof createCandidate> } = $props();
</script>

{#each model.ui.services as svc (svc.serviceId)}
	<section
		id={`candidate-service-${svc.serviceId}`}
		aria-label={`${svc.serviceName} candidate`}
		class="scroll-mt-6 space-y-2"
	>
		<div class="flex flex-wrap items-center justify-between gap-3">
			<span class="text-xs text-muted-foreground"
				>{svc.hasCandidateEnv
					? `Saved draft: ${svc.revisionTarget}`
					: 'Using active configuration as the starting point'}</span
			>
			<div class="flex gap-2">
				<Button.Root
					variant="ghost"
					size="xs"
					disabled={model.busy || model.loading || !model.serviceChanged(svc)}
					onclick={() => (model.ui.resetSelection = svc)}>Reset to saved</Button.Root
				>{#if svc.hasCandidateEnv}<Button.Root
						variant="ghost"
						size="xs"
						disabled={model.busy || model.loading || model.hasAnyChanges}
						onclick={() => {
							model.ui.deleteError = '';
							model.ui.confirmDelete = {
								serviceId: svc.serviceId,
								serviceName: svc.serviceName,
								target: svc.revisionTarget || model.effectiveTarget
							};
						}}>Delete saved draft</Button.Root
					>{/if}
			</div>
		</div>
		<ConfigWorkspace
			serviceName={svc.serviceName}
			context={model.ui.loadedTarget || model.effectiveTarget}
			mode="candidate"
			directory={model.watcher.install_dir}
			envPath={model.watcher.services.find((service) => service.id === svc.serviceId)?.env_file ||
				''}
			bind:envContent={svc.candidateEnv}
			bind:files={svc.candidateFiles}
			baselineEnv={svc.activeEnv}
			baselineFiles={svc.activeFiles}
			savedEnv={svc.originalCandidateEnv}
			savedFiles={svc.originalCandidateFiles}
			explicitFiles={svc.filesExplicit}
			savedExplicitFiles={svc.originalFilesExplicit}
			disabled={model.busy ||
				model.saving ||
				model.approving ||
				model.discarding ||
				model.loading ||
				model.effectiveTarget !== model.ui.loadedTarget}
			onfileschange={() => (svc.filesExplicit = true)}
		/>
	</section>
{/each}
